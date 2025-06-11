package airdrop

import (
	"context"

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

// Welcome bonuses have been disabled
func (i *Integration) GetWelcomeBonusAmount(ctx context.Context, owner *codecommon.Account) (float64, codecurrency.Code, error) {
	return 0, "", nil
}
