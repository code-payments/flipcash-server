package activity

import (
	"context"
	"errors"

	activitypb "github.com/code-payments/flipcash-protobuf-api/generated/go/activity/v1"
)

func InjectLocalizedText(ctx context.Context, notification *activitypb.Notification) error {
	var localizedText string
	switch notification.AdditionalMetadata.(type) {
	case *activitypb.Notification_WelcomeBonus:
		localizedText = "Welcome Bonus"
	case *activitypb.Notification_GaveUsdc:
		localizedText = "Gave"
	case *activitypb.Notification_ReceivedUsdc:
		localizedText = "Received"
	case *activitypb.Notification_WithdrewUsdc:
		localizedText = "Withdrew"
	default:
		return errors.New("unsupported notification type")
	}
	notification.LocalizedText = localizedText
	return nil
}
