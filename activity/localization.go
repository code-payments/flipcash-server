package activity

import (
	"context"
	"errors"

	activitypb "github.com/code-payments/flipcash-protobuf-api/generated/go/activity/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
)

func InjectLocalizedText(ctx context.Context, codeData codedata.Provider, userOwnerAccount *codecommon.Account, notification *activitypb.Notification) error {
	var localizedText string
	switch typed := notification.AdditionalMetadata.(type) {
	case *activitypb.Notification_WelcomeBonus:
		localizedText = "Welcome Bonus"

	case *activitypb.Notification_GaveCrypto:
		localizedText = "Gave"

	case *activitypb.Notification_ReceivedCrypto:
		localizedText = "Received"

	case *activitypb.Notification_WithdrewCrypto:
		localizedText = "Withdrew"

	case *activitypb.Notification_DepositedCrypto:
		localizedText = "Added"

	case *activitypb.Notification_PaidCrypto:
		switch typed.PaidCrypto.PaymentMetadata.(type) {
		case *activitypb.PaidCryptoNotificationMetadata_Pool:
			localizedText = "Paid into pool"
		default:
			return errors.New("unsupported paid usdc payment metadata type")
		}

	case *activitypb.Notification_DistributedCrypto:
		switch typed2 := typed.DistributedCrypto.DistributionMetadata.(type) {
		case *activitypb.DistributedCryptoNotificationMetadata_Pool:
			switch typed2.Pool.Outcome {
			case poolpb.UserOutcome_WIN_OUTCOME, poolpb.UserOutcome_REFUND_OUTCOME:
				localizedText = "Received from pool"
			default:
				return errors.New("unsupported distributed usdc pool outcome")
			}
		default:
			return errors.New("unsupported distributed usdc distribution metadata type")
		}

	case *activitypb.Notification_SentCrypto:
		if typed.SentCrypto.CanInitiateCancelAction {
			localizedText = "Sending"
		} else {
			localizedText = "Sent"

			giftCardVaultAccount, err := codecommon.NewAccountFromPublicKeyBytes(typed.SentCrypto.Vault.Value)
			if err != nil {
				return err
			}

			intentRecord, err := codeData.GetGiftCardClaimedIntent(ctx, giftCardVaultAccount.PublicKey().ToBase58())
			if err != nil {
				return err
			}

			if intentRecord.InitiatorOwnerAccount == userOwnerAccount.PublicKey().ToBase58() {
				if intentRecord.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard {
					localizedText = "Cancelled"
				}
				if intentRecord.ReceivePaymentsPubliclyMetadata.IsReturned {
					localizedText = "Returned"
				}
			}
		}

	default:
		return errors.New("unsupported notification type")
	}

	notification.LocalizedText = localizedText

	return nil
}
