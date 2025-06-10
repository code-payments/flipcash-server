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

	var allAirdropPurchases []*iap.Purchase

	purchases, err := i.iaps.GetPurchasesByUserAndProduct(ctx, userID, iap.ProductCreateAccountWithWelcomeBonus)
	switch err {
	case nil:
		for _, purchase := range purchases {
			if purchase.Platform == commonpb.Platform_GOOGLE {
				allAirdropPurchases = append(allAirdropPurchases, purchase)
			}
		}
	case iap.ErrNotFound:
	default:
		return 0, "", err
	}

	purchases, err = i.iaps.GetPurchasesByUserAndProduct(ctx, userID, iap.ProductCreateAccount)
	switch err {
	case nil:
		for _, purchase := range purchases {
			if purchase.Platform == commonpb.Platform_APPLE {
				allAirdropPurchases = append(allAirdropPurchases, purchase)
			}
		}
	case iap.ErrNotFound:
	default:
		return 0, "", err
	}

	if len(allAirdropPurchases) == 0 {
		return 0, "", nil
	}
	purchase := allAirdropPurchases[0]
	for _, otherPurchase := range allAirdropPurchases {
		if otherPurchase.PaymentCurrency != purchase.PaymentCurrency || otherPurchase.PaymentAmount != purchase.PaymentAmount {
			return 0, "", errors.New("user has multiple conflicting airdrop iaps")
		}
	}
	return purchase.PaymentAmount, codecurrency.Code(purchase.PaymentCurrency), nil
}
