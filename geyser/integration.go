package geyser

import (
	"context"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	codegeyser "github.com/code-payments/code-server/pkg/code/async/geyser"
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/flipcash-server/account"
	"github.com/code-payments/flipcash-server/push"
)

type Integration struct {
	accounts account.Store
	pusher   push.Pusher
}

func NewIntegration(accounts account.Store, pusher push.Pusher) codegeyser.Integration {
	return &Integration{
		accounts: accounts,
		pusher:   pusher,
	}
}

func (i *Integration) OnDepositReceived(ctx context.Context, owner, mint *codecommon.Account, currencyName string, usdMarketValue float64) error {
	// Hide small, potentially spam deposits
	if usdMarketValue < 0.01 {
		return nil
	}

	userID, err := i.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
	if err != nil {
		return err
	}

	if codecommon.IsCoreMint(mint) {
		return push.SendUsdcReceivedFromDepositPush(ctx, i.pusher, userID, usdMarketValue)
	}
	return push.SendFlipcashCurrencyReceivedFromDepositPush(ctx, i.pusher, userID, currencyName, usdMarketValue)
}
