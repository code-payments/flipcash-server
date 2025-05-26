package activity

import (
	"context"

	codebalance "github.com/code-payments/code-server/pkg/code/balance"
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
)

func isGiftCardClaimed(ctx context.Context, codeData codedata.Provider, giftCardVaultAccount *codecommon.Account) (bool, error) {
	balance, err := codebalance.CalculateFromCache(ctx, codeData, giftCardVaultAccount)
	if err == codebalance.ErrNotManagedByCode {
		return true, nil
	} else if err != nil {
		return false, err
	}
	return balance == 0, nil
}
