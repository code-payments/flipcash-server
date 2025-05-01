package activity

import (
	"context"
	"errors"

	activitypb "github.com/code-payments/flipcash-protobuf-api/generated/go/activity/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
)

func InjectLocalizedText(ctx context.Context, codeData codedata.Provider, userOwnerAccount *codecommon.Account, notification *activitypb.Notification) error {
	var localizedText string
	switch typed := notification.AdditionalMetadata.(type) {
	case *activitypb.Notification_WelcomeBonus:
		localizedText = "Welcome Bonus"
	case *activitypb.Notification_GaveUsdc:
		localizedText = "Gave"
	case *activitypb.Notification_ReceivedUsdc:
		localizedText = "Received"
	case *activitypb.Notification_WithdrewUsdc:
		localizedText = "Withdrew"
	case *activitypb.Notification_SentUsdc:
		if typed.SentUsdc.CanInitiateCancelAction {
			localizedText = "Sending"
		} else {
			localizedText = "Sent"

			giftCardVaultAccount, err := codecommon.NewAccountFromPublicKeyBytes(typed.SentUsdc.Vault.Value)
			if err != nil {
				return err
			}

			isClaimedByUser, err := isClaimedGiftCardReturnedToSender(ctx, codeData, userOwnerAccount, giftCardVaultAccount)
			if err != nil {
				return err
			} else if isClaimedByUser {
				localizedText = "Cancelled"
			}
		}
	case *activitypb.Notification_DepositedUsdc:
		localizedText = "Deposited"
	default:
		return errors.New("unsupported notification type")
	}
	notification.LocalizedText = localizedText
	return nil
}
