package pool

import (
	"bytes"
	"context"
	"errors"

	codetransactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codebalance "github.com/code-payments/code-server/pkg/code/balance"
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codetransaction "github.com/code-payments/code-server/pkg/code/server/transaction"
	"github.com/code-payments/flipcash-server/event"
)

// todo: add tests
type IntentHandler struct {
	pools Store

	codeData codedata.Provider

	eventForwarder event.Forwarder
}

func NewIntentHandler(pools Store, codeData codedata.Provider, eventForwarder event.Forwarder) *IntentHandler {
	return &IntentHandler{
		pools: pools,

		codeData: codeData,

		eventForwarder: eventForwarder,
	}
}

func (h *IntentHandler) ValidateBetPayment(ctx context.Context, intentRecord *codeintent.Record) error {
	if intentRecord.IntentType != codeintent.SendPublicPayment {
		return errors.New("unexpected intent type")
	}

	return codetransaction.NewIntentDeniedError("bet payments are disabled")

	intentID, err := codecommon.NewAccountFromPublicKeyString(intentRecord.IntentId)
	if err != nil {
		return err
	}

	destinationTokenAccount, err := codecommon.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
	if err != nil {
		return err
	}

	// The intent ID must match the bet ID
	bet, err := h.pools.GetBetByID(ctx, &poolpb.BetId{Value: intentID.PublicKey().ToBytes()})
	if err == ErrBetNotFound {
		return codetransaction.NewIntentValidationErrorf("bet with id %s does not exist", intentID.PublicKey().ToBase58())
	} else if err != nil {
		return err
	}

	// The bet payment must be made to a betting pool
	bettingPool, err := h.pools.GetPoolByFundingDestination(ctx, &commonpb.PublicKey{Value: destinationTokenAccount.PublicKey().ToBytes()})
	if err == ErrPoolNotFound {
		return codetransaction.NewIntentValidationErrorf("betting pool with funding destination %s does not exist", destinationTokenAccount.PublicKey().ToBase58())
	} else if err != nil {
		return err
	}

	// The bet must be associated to the pool it was made against
	if !bytes.Equal(bettingPool.ID.Value, bet.PoolID.Value) {
		return codetransaction.NewIntentValidationError("bet payment sent to wrong pool")
	}

	// Bet payment amount must be exactly the buy in
	if string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency) != bettingPool.BuyInCurrency {
		return codetransaction.NewIntentValidationErrorf("betting pool buy in currency must be %s", bettingPool.BuyInCurrency)
	}
	if intentRecord.SendPublicPaymentMetadata.NativeAmount != bettingPool.BuyInAmount {
		return codetransaction.NewIntentValidationErrorf("betting pool buy in amount must be %.6f", bettingPool.BuyInAmount)
	}

	return nil
}

func (h *IntentHandler) ValidateDistribution(ctx context.Context, intentRecord *codeintent.Record, actions []*codetransactionpb.Action) error {
	if intentRecord.IntentType != codeintent.PublicDistribution {
		return errors.New("unexpected intent type")
	}

	poolAccount, err := codecommon.NewAccountFromPublicKeyString(intentRecord.PublicDistributionMetadata.Source)
	if err != nil {
		return err
	}

	bettingPool, err := h.pools.GetPoolByFundingDestination(ctx, &commonpb.PublicKey{Value: poolAccount.PublicKey().ToBytes()})
	if err == ErrPoolNotFound {
		return codetransaction.NewIntentValidationError("source is not a betting pool")
	} else if err != nil {
		return err
	}

	// Betting pool must be closed and have a resolution in order for payout to occur
	if bettingPool.IsOpen {
		return codetransaction.NewIntentValidationError("betting pool is open")
	}
	if !bettingPool.HasResolution() {
		return codetransaction.NewIntentValidationError("betting pool is not resolved")
	}

	bets, err := h.pools.GetBetsByPool(ctx, bettingPool.ID)
	if err == ErrBetNotFound {
		return codetransaction.NewIntentValidationError("no bets made against betting pool")
	} else if err != nil {
		return err
	}

	var paidBets []*Bet
	for _, bet := range bets {
		isPaid, err := bet.IsPaid(ctx, h.pools, h.codeData, bettingPool)
		if err != nil {
			return err
		}

		if isPaid {
			paidBets = append(paidBets, bet)
		}
	}

	var betsToPayout []*Bet
	for _, bet := range paidBets {
		switch bettingPool.Resolution {
		case ResolutionRefunded:
			betsToPayout = append(betsToPayout, bet)
		case ResolutionYes:
			if bet.SelectedOutcome {
				betsToPayout = append(betsToPayout, bet)
			}
		case ResolutionNo:
			if !bet.SelectedOutcome {
				betsToPayout = append(betsToPayout, bet)
			}
		default:
			return errors.New("unsupported resolution")
		}
	}
	if len(betsToPayout) == 0 {
		betsToPayout = paidBets
	}
	if len(betsToPayout) == 0 {
		return codetransaction.NewIntentDeniedError("no bets to pay out for pool")
	}

	bettingPoolBalance, err := codebalance.CalculateFromCache(ctx, h.codeData, poolAccount)
	if err != nil {
		return err
	}
	minPayoutAmount := bettingPoolBalance / uint64(len(betsToPayout))

	remainingPoolBalance := int64(bettingPoolBalance)
	seenPayoutDestinations := make(map[string]any)
	for _, action := range actions {
		var payoutAmount uint64
		var payoutDestinationAccount *codecommon.Account
		switch typed := action.Type.(type) {
		case *codetransactionpb.Action_NoPrivacyTransfer:
			payoutAmount = typed.NoPrivacyTransfer.Amount
			payoutDestinationAccount, err = codecommon.NewAccountFromProto(typed.NoPrivacyTransfer.Destination)
			if err != nil {
				return err
			}
		case *codetransactionpb.Action_NoPrivacyWithdraw:
			payoutAmount = typed.NoPrivacyWithdraw.Amount
			payoutDestinationAccount, err = codecommon.NewAccountFromProto(typed.NoPrivacyWithdraw.Destination)
			if err != nil {
				return err
			}
		default:
			return codetransaction.NewActionValidationError(action, "expected a no privacy transfer or withdraw")
		}

		// Each winning bet should be paid an equal amount
		//
		// todo: Enforce maximum when client-side fix is deployed to evenly distribute remainder
		if payoutAmount < minPayoutAmount {
			return codetransaction.NewActionValidationErrorf(action, "bet payout amount minimum is %d", minPayoutAmount)
		}
		remainingPoolBalance -= int64(payoutAmount)

		// Each winning bet should be paid at most once
		_, ok := seenPayoutDestinations[payoutDestinationAccount.PublicKey().ToBase58()]
		if ok {
			return codetransaction.NewActionValidationError(action, "duplicate bet payout destination")
		}
		seenPayoutDestinations[payoutDestinationAccount.PublicKey().ToBase58()] = true
	}

	// Exact pool balance should be distributed
	if remainingPoolBalance != 0 {
		return codetransaction.NewIntentValidationErrorf("betting pool has a remaining balance of %d quarks", remainingPoolBalance)
	}

	// Ensure all winning bets are paid
	if len(actions) != len(betsToPayout) {
		return codetransaction.NewIntentValidationErrorf("expected %d actions", len(betsToPayout))
	}
	for _, bet := range betsToPayout {
		payoutDestinationAccount, err := codecommon.NewAccountFromPublicKeyBytes(bet.PayoutDestination.Value)
		if err != nil {
			return err
		}

		if _, ok := seenPayoutDestinations[payoutDestinationAccount.PublicKey().ToBase58()]; !ok {
			return codetransaction.NewIntentValidationErrorf("bet payout to %s is missing", payoutDestinationAccount.PublicKey().ToBase58())
		}
	}

	return nil
}
