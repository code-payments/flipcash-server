package pool

import (
	"bytes"
	"context"
	"errors"
	"math"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
)

// todo: optimize when we don't require the entire bet list
func GetBetSummary(ctx context.Context, pools Store, codeData codedata.Provider, pool *Pool) (*poolpb.BetSummary, []*Bet, error) {
	bets, err := pools.GetBetsByPool(ctx, pool.ID)
	if err != nil && err != ErrBetNotFound {
		return nil, nil, err
	}

	var numYes, numNo int
	for _, bet := range bets {
		isPaid, err := bet.IsPaid(ctx, pools, codeData, pool)
		if err != nil {
			return nil, nil, err
		}

		if !isPaid {
			continue
		}

		if bet.SelectedOutcome {
			numYes++
		} else {
			numNo++
		}
	}

	return &poolpb.BetSummary{
		Kind: &poolpb.BetSummary_BooleanSummary{
			BooleanSummary: &poolpb.BetSummary_BooleanBetSummary{
				NumYes: uint32(numYes),
				NumNo:  uint32(numNo),
			},
		},
		TotalAmountBet: &commonpb.FiatPaymentAmount{
			Currency:     pool.BuyInCurrency,
			NativeAmount: float64(numYes+numNo) * pool.BuyInAmount,
		},
	}, bets, nil
}

func GetUserSummary(ctx context.Context, pools Store, codeData codedata.Provider, userID *commonpb.UserId, pool *Pool) (*poolpb.UserPoolSummary, error) {
	res := &poolpb.UserPoolSummary{
		Outcome: &poolpb.UserPoolSummary_None{},
	}

	if !pool.HasResolution() {
		return res, nil
	}

	betSummary, bets, err := GetBetSummary(ctx, pools, codeData, pool)
	if err != nil {
		return nil, err
	}

	return getUserSummaryWithCachedBetMetadata(userID, pool, betSummary, bets)
}

// Use this method when GetBetSummary has already been called to avoid recalculation
//
// todo: Export this utility?
func getUserSummaryWithCachedBetMetadata(userID *commonpb.UserId, pool *Pool, betSummary *poolpb.BetSummary, bets []*Bet) (*poolpb.UserPoolSummary, error) {
	res := &poolpb.UserPoolSummary{
		Outcome: &poolpb.UserPoolSummary_None{},
	}

	if !pool.HasResolution() {
		return res, nil
	}

	var userBet *Bet
	for _, bet := range bets {
		if bytes.Equal(bet.UserID.Value, userID.Value) {
			userBet = bet
			break
		}
	}
	if userBet == nil {
		return res, nil
	}

	if !userBet.IsIntentSubmitted {
		return res, nil
	}

	var isUserWinner bool
	var isUserRefunded bool
	var numWinners, numLosers int
	switch pool.Resolution {
	case ResolutionRefunded:
		isUserWinner = false
		isUserRefunded = true
	case ResolutionYes:
		isUserWinner = userBet.SelectedOutcome
		isUserRefunded = betSummary.GetBooleanSummary().NumYes == 0
		numWinners = int(betSummary.GetBooleanSummary().NumYes)
		numLosers = int(betSummary.GetBooleanSummary().NumNo)
	case ResolutionNo:
		isUserWinner = !userBet.SelectedOutcome
		isUserRefunded = betSummary.GetBooleanSummary().NumNo == 0
		numWinners = int(betSummary.GetBooleanSummary().NumNo)
		numLosers = int(betSummary.GetBooleanSummary().NumYes)
	default:
		return nil, errors.New("unsupported resolution")
	}

	if isUserRefunded {
		res = &poolpb.UserPoolSummary{
			Outcome: &poolpb.UserPoolSummary_Refund{
				Refund: &poolpb.UserPoolSummary_RefundOutcome{
					AmountRefunded: &commonpb.FiatPaymentAmount{
						Currency:     pool.BuyInCurrency,
						NativeAmount: pool.BuyInAmount,
					},
				},
			},
		}
	} else if isUserWinner {
		totalAmountReceived := (float64(numWinners+numLosers) * pool.BuyInAmount) / float64(numWinners)
		amountWon := math.Max(totalAmountReceived-pool.BuyInAmount, 0)
		res = &poolpb.UserPoolSummary{
			Outcome: &poolpb.UserPoolSummary_Win{
				Win: &poolpb.UserPoolSummary_WinOutcome{
					AmountWon: &commonpb.FiatPaymentAmount{
						Currency:     pool.BuyInCurrency,
						NativeAmount: amountWon,
					},
					TotalAmountReceived: &commonpb.FiatPaymentAmount{
						Currency:     pool.BuyInCurrency,
						NativeAmount: totalAmountReceived,
					},
				},
			},
		}
	} else {
		res = &poolpb.UserPoolSummary{
			Outcome: &poolpb.UserPoolSummary_Lose{
				Lose: &poolpb.UserPoolSummary_LoseOutcome{
					AmountLost: &commonpb.FiatPaymentAmount{
						Currency:     pool.BuyInCurrency,
						NativeAmount: pool.BuyInAmount,
					},
				},
			},
		}
	}
	return res, nil
}
