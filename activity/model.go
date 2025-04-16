package activity

import (
	"encoding/hex"

	activitypb "github.com/code-payments/flipcash-protobuf-api/generated/go/activity/v1"
)

func NotificationIDString(id *activitypb.NotificationId) string {
	if id == nil {
		return "<invalid>"
	}
	return hex.EncodeToString(id.Value)
}
