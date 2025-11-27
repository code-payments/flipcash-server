package swap

import (
	"context"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	codeswap "github.com/code-payments/code-server/pkg/code/async/swap"
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codecurrency "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/flipcash-server/account"
	"github.com/code-payments/flipcash-server/push"
)

type Integration struct {
	accounts account.Store
	pusher   push.Pusher
}

func NewIntegration(accounts account.Store, pusher push.Pusher) codeswap.Integration {
	return &Integration{
		accounts: accounts,
		pusher:   pusher,
	}
}

func (i *Integration) OnSwapFinalized(ctx context.Context, owner, mint *codecommon.Account, currencyName string, region codecurrency.Code, nativeAmount float64) error {
	userID, err := i.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
	if err != nil {
		return err
	}

	if codecommon.IsCoreMint(mint) {
		return push.SendUsdcReceivedFromSwapPush(ctx, i.pusher, userID, region, nativeAmount)
	}
	return push.SendFlipcashCurrencyReceivedFromSwapPush(ctx, i.pusher, userID, currencyName, region, nativeAmount)
}
