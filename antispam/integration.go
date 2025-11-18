package antispam

import (
	"context"

	codetransactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	codeantispam "github.com/code-payments/code-server/pkg/code/antispam"
	"github.com/code-payments/code-server/pkg/code/common"
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/flipcash-server/account"
)

type Integration struct {
	accounts account.Store
}

func NewIntegration(accounts account.Store) codeantispam.Integration {
	return &Integration{
		accounts: accounts,
	}
}

func (i *Integration) AllowOpenAccounts(ctx context.Context, owner *codecommon.Account, accountSet codetransactionpb.OpenAccountsMetadata_AccountSet) (bool, string, error) {
	switch accountSet {
	case codetransactionpb.OpenAccountsMetadata_USER:
		userID, err := i.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
		if err == account.ErrNotFound {
			return false, "public key not associated with a flipcash user", nil
		} else if err != nil {
			return false, "", err
		}

		isRegistered, err := i.accounts.IsRegistered(ctx, userID)
		if err != nil {
			return false, "", err
		}

		if !isRegistered {
			return false, "flipcash user has not completed iap for account creation", nil
		}
		return true, "", nil
	case codetransactionpb.OpenAccountsMetadata_POOL:
		return true, "", nil
	default:
		return false, "unsupported account set", nil
	}
}

func (i *Integration) AllowWelcomeBonus(ctx context.Context, owner *codecommon.Account) (bool, string, error) {
	// Always allow since we properly gate everything required in AllowOpenAccounts
	return true, "", nil
}

func (i *Integration) AllowSendPayment(_ context.Context, _, _ *codecommon.Account, isPublic bool) (bool, string, error) {
	if !isPublic {
		return false, "flipcash payments must be public", nil
	}
	return true, "", nil
}

func (i *Integration) AllowReceivePayments(ctx context.Context, owner *codecommon.Account, isPublic bool) (bool, string, error) {
	if !isPublic {
		return false, "flipcash payments must be public", nil
	}
	return true, "", nil
}

func (i *Integration) AllowDistribution(ctx context.Context, owner *codecommon.Account, isPublic bool) (bool, string, error) {
	if !isPublic {
		return false, "flipcash distributions must be public", nil
	}
	return true, "", nil
}

func (i *Integration) AllowSwap(_ context.Context, _, _, _ *common.Account) (bool, string, error) {
	return true, "", nil
}
