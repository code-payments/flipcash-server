package airdrop

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codetransaction "github.com/code-payments/code-server/pkg/code/server/transaction"
	codecurrency "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/flipcash-server/account"
	"github.com/code-payments/flipcash-server/iap"
)

type Integration struct {
	accounts account.Store
	iaps     iap.Store
}

func NewIntegration(accounts account.Store, iaps iap.Store) codetransaction.AirdropIntegration {
	return &Integration{
		accounts: accounts,
		iaps:     iaps,
	}
}

func (i *Integration) GetWelcomeBonusAmount(ctx context.Context, owner *codecommon.Account) (float64, codecurrency.Code, error) {
	userID, err := i.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
	if err == account.ErrNotFound {
		return 0, "", nil
	} else if err != nil {
		return 0, "", err
	}

	purchases, err := i.iaps.GetPurchasesByUserAndProduct(ctx, userID, iap.ProductCreateAccountWithWelcomeBonus)
	if err == iap.ErrNotFound {
		return 0, "", nil
	} else if err != nil {
		return 0, "", err
	}

	if len(purchases) == 0 {
		return 0, "", nil
	}
	if len(purchases) > 1 {
		return 0, "", errors.New("user has multiple account creation with welcome bonus purchases")
	}
	purchase := purchases[0]

	return purchase.PaymentAmount, codecurrency.Code(purchase.PaymentCurrency), nil
}
