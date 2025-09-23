package intent

import (
	"context"

	codecommonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	codetransactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeaccount "github.com/code-payments/code-server/pkg/code/data/account"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codetransaction "github.com/code-payments/code-server/pkg/code/server/transaction"
	"github.com/code-payments/flipcash-server/event"
	"github.com/code-payments/flipcash-server/pool"
)

type Integration struct {
	codeData codedata.Provider

	bettingPoolHandler *pool.IntentHandler
}

func NewIntegration(
	pools pool.Store,
	codeData codedata.Provider,
	eventForwarder event.Forwarder,
) codetransaction.SubmitIntentIntegration {
	return &Integration{
		codeData: codeData,

		bettingPoolHandler: pool.NewIntentHandler(pools, codeData, eventForwarder),
	}
}

func (i *Integration) AllowCreation(ctx context.Context, intentRecord *codeintent.Record, metadata *codetransactionpb.Metadata, actions []*codetransactionpb.Action) error {
	switch intentRecord.IntentType {
	case codeintent.OpenAccounts:
		if metadata.GetOpenAccounts().AccountSet != codetransactionpb.OpenAccountsMetadata_USER {
			return codetransaction.NewIntentDeniedError("only user account set opening is currently enabled")
		}
		return nil

	case codeintent.SendPublicPayment:
		isPoolAccount, err := i.isPoolAccount(ctx, intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
		if err != nil {
			return err
		} else if !isPoolAccount {
			return nil
		}

		// Destination account is a pool, enforce betting logic
		return i.bettingPoolHandler.ValidateBetPayment(ctx, intentRecord)

	case codeintent.PublicDistribution:
		return i.bettingPoolHandler.ValidateDistribution(ctx, intentRecord, actions)

	case codeintent.ReceivePaymentsPublicly:
		return nil

	default:
		return codetransaction.NewIntentDeniedError("flipcash does not support the intent type")
	}
}

func (i *Integration) OnSuccess(ctx context.Context, intentRecord *codeintent.Record) error {
	switch intentRecord.IntentType {
	case codeintent.SendPublicPayment:
		isPoolAccount, err := i.isPoolAccount(ctx, intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
		if err != nil {
			return err
		} else if !isPoolAccount {
			return nil
		}

		// Destination account is a pool, call bet payment success handler
		return i.bettingPoolHandler.OnSuccessfulBetPayment(ctx, intentRecord)
	}
	return nil
}

func (i *Integration) isPoolAccount(ctx context.Context, address string) (bool, error) {
	tokenAccount, err := codecommon.NewAccountFromPublicKeyString(address)
	if err != nil {
		return false, err
	}

	destinationAccountInfoRecord, err := i.codeData.GetAccountInfoByTokenAddress(ctx, tokenAccount.PublicKey().ToBase58())
	if err == codeaccount.ErrAccountInfoNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return destinationAccountInfoRecord.AccountType == codecommonpb.AccountType_POOL, nil
}
