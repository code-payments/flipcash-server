package intent

import (
	"bytes"
	"context"
	"errors"

	codecommonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	codetransactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codebalance "github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codetransaction "github.com/code-payments/code-server/pkg/code/server/transaction"
	"github.com/code-payments/flipcash-server/pool"
)

type Integration struct {
	pools    pool.Store
	codeData codedata.Provider
}

func NewIntegration(pools pool.Store, codeData codedata.Provider) codetransaction.SubmitIntentIntegration {
	return &Integration{
		pools:    pools,
		codeData: codeData,
	}
}

func (i *Integration) AllowCreation(ctx context.Context, intentRecord *codeintent.Record, _s *codetransactionpb.Metadata, actions []*codetransactionpb.Action) error {
	switch intentRecord.IntentType {
	case codeintent.SendPublicPayment:
		return i.validatePotentialBetPayment(ctx, intentRecord)
	case codeintent.PublicDistribution:
		return i.validateBettingPoolDistribution(ctx, intentRecord, actions)
	case codeintent.OpenAccounts, codeintent.ReceivePaymentsPublicly:
		return nil
	default:
		return codetransaction.NewIntentDeniedError("flipcash does not support the intent type")
	}
}

func (i *Integration) validatePotentialBetPayment(ctx context.Context, intentRecord *codeintent.Record) error {
	if intentRecord.IntentType != codeintent.SendPublicPayment {
		return errors.New("unexpected intent type")
	}

	intentID, err := common.NewAccountFromPublicKeyString(intentRecord.IntentId)
	if err != nil {
		return err
	}

	destinationTokenAccount, err := common.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
	if err != nil {
		return err
	}

	destinationAccountInfoRecord, err := i.codeData.GetAccountInfoByTokenAddress(ctx, destinationTokenAccount.PublicKey().ToBase58())
	if err != nil {
		return err
	}

	if destinationAccountInfoRecord.AccountType != codecommonpb.AccountType_POOL {
		return nil
	}

	// Destination account is a pool, enforce betting logic

	// The intent ID must match the bet ID
	bet, err := i.pools.GetBetByID(ctx, &poolpb.BetId{Value: intentID.PublicKey().ToBytes()})
	if err == pool.ErrBetNotFound {
		return codetransaction.NewIntentValidationErrorf("bet with id %s does not exist", intentID.PublicKey().ToBase58())
	} else if err != nil {
		return err
	}

	// The bet payment must be made to a betting pool
	bettingPool, err := i.pools.GetPoolByFundingDestination(ctx, &commonpb.PublicKey{Value: destinationTokenAccount.PublicKey().ToBytes()})
	if err == pool.ErrPoolNotFound {
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

func (i *Integration) validateBettingPoolDistribution(ctx context.Context, intentRecord *codeintent.Record, actions []*codetransactionpb.Action) error {
	if intentRecord.IntentType != codeintent.PublicDistribution {
		return errors.New("unexpected intent type")
	}

	poolAccount, err := common.NewAccountFromPublicKeyString(intentRecord.PublicDistributionMetadata.Source)
	if err != nil {
		return err
	}

	bettingPool, err := i.pools.GetPoolByFundingDestination(ctx, &commonpb.PublicKey{Value: poolAccount.PublicKey().ToBytes()})
	if err == pool.ErrPoolNotFound {
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

	bets, err := i.pools.GetBetsByPool(ctx, bettingPool.ID)
	if err == pool.ErrBetNotFound {
		return codetransaction.NewIntentValidationError("no bets made against betting pool")
	} else if err != nil {
		return err
	}

	var winningBets []*pool.Bet
	for _, bet := range bets {
		isPaid, err := bet.IsPaid(ctx, i.codeData, i.pools, bettingPool)
		if err != nil {
			return err
		}

		if !isPaid {
			continue
		}

		switch bettingPool.Resolution {
		case pool.ResolutionRefunded:
			winningBets = append(winningBets, bet)
		case pool.ResolutionYes:
			if bet.SelectedOutcome {
				winningBets = append(winningBets, bet)
			}
		case pool.ResolutionNo:
			if !bet.SelectedOutcome {
				winningBets = append(winningBets, bet)
			}
		default:
			return errors.New("unsupported resolution")
		}

	}

	bettingPoolBalance, err := codebalance.CalculateFromCache(ctx, i.codeData, poolAccount)
	if err != nil {
		return err
	}
	minPayoutAmount := bettingPoolBalance / uint64(len(winningBets))
	maxPayoutAmount := minPayoutAmount + 1

	remainingPoolBalance := int64(bettingPoolBalance)
	seenPayoutDestinations := make(map[string]any)
	for _, action := range actions {
		var payoutAmount uint64
		var payoutDestinationAccount *common.Account
		switch typed := action.Type.(type) {
		case *codetransactionpb.Action_NoPrivacyTransfer:
			payoutAmount = typed.NoPrivacyTransfer.Amount
			payoutDestinationAccount, err = common.NewAccountFromProto(typed.NoPrivacyTransfer.Destination)
			if err != nil {
				return err
			}
		case *codetransactionpb.Action_NoPrivacyWithdraw:
			payoutAmount = typed.NoPrivacyWithdraw.Amount
			payoutDestinationAccount, err = common.NewAccountFromProto(typed.NoPrivacyWithdraw.Destination)
			if err != nil {
				return err
			}
		default:
			return codetransaction.NewActionValidationError(action, "expected a no privacy transfer or withdraw")
		}

		// Each winning bet should be paid an equal amount
		if payoutAmount < minPayoutAmount || payoutAmount > maxPayoutAmount {
			return codetransaction.NewActionValidationErrorf(action, "bet payout amount must be in [%d, %d]", minPayoutAmount, maxPayoutAmount)
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
	if len(actions) != len(winningBets) {
		return codetransaction.NewIntentValidationErrorf("expected %d actions", len(winningBets))
	}
	for _, bet := range winningBets {
		payoutDestinationAccount, err := common.NewAccountFromPublicKeyBytes(bet.PayoutDestination.Value)
		if err != nil {
			return err
		}

		if _, ok := seenPayoutDestinations[payoutDestinationAccount.PublicKey().ToBase58()]; !ok {
			return codetransaction.NewIntentValidationErrorf("bet payout to %s is missing", payoutDestinationAccount.PublicKey().ToBase58())
		}
	}

	return nil
}
