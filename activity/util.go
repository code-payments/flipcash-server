package activity

import (
	"context"

	codebalance "github.com/code-payments/code-server/pkg/code/balance"
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeaction "github.com/code-payments/code-server/pkg/code/data/action"
)

func isGiftCardIssuedByUser(ctx context.Context, codeData codedata.Provider, userOwnerAccount, giftCardVaultAccount *codecommon.Account) (bool, error) {
	issuedIntentRecord, err := codeData.GetOriginalGiftCardIssuedIntent(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err != nil {
		return false, err
	}

	return issuedIntentRecord.InitiatorOwnerAccount == userOwnerAccount.PublicKey().ToBase58(), nil
}

func isGiftCardClaimed(ctx context.Context, codeData codedata.Provider, giftCardVaultAccount *codecommon.Account) (bool, error) {
	balance, err := codebalance.CalculateFromCache(ctx, codeData, giftCardVaultAccount)
	if err == codebalance.ErrNotManagedByCode {
		return true, nil
	} else if err != nil {
		return false, err
	}
	return balance == 0, nil
}

func isClaimedGiftCardAccountReturnedToSender(ctx context.Context, codeData codedata.Provider, userOwnerAccount, giftCardVaultAccount *codecommon.Account) (bool, error) {
	claimedActionRecord, err := codeData.GetGiftCardClaimedAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	switch err {
	case nil:

		userTimelockAccounts, err := userOwnerAccount.GetTimelockAccounts(codecommon.CodeVmAccount, codecommon.CoreMintAccount)
		if err != nil {
			return false, err
		}

		return *claimedActionRecord.Destination == userTimelockAccounts.Vault.PublicKey().ToBase58(), nil
	case codeaction.ErrActionNotFound:
		// There is no explicit claim action, the gift card must've been auto-returned
		return true, nil
	default:
		return false, err
	}
}
