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

func (i *Integration) OnDepositReceived(ctx context.Context, owner *codecommon.Account, quarksReceived uint64) error {
	userID, err := i.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
	if err != nil {
		return err
	}
	return push.SendDepositReceivedPush(ctx, i.pusher, userID, quarksReceived)
}
