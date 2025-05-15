package tests

import (
	"context"
	"fmt"
	"testing"

	"firebase.google.com/go/v4/messaging"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/flipcash-protobuf-api/generated/go/push/v1"

	"github.com/code-payments/flipcash-server/push"
)

// testFCMClient captures the messages sent for verification
type testFCMClient struct {
	sentMessage *messaging.MulticastMessage
}

func (c *testFCMClient) SendEachForMulticast(_ context.Context, message *messaging.MulticastMessage) (*messaging.BatchResponse, error) {
	c.sentMessage = message
	return &messaging.BatchResponse{
		SuccessCount: len(message.Tokens),
		Responses:    make([]*messaging.SendResponse, len(message.Tokens)),
	}, nil
}

func RunPusherTests(t *testing.T, s push.TokenStore, teardown func()) {
	for _, tf := range []func(t *testing.T, s push.TokenStore){
		testFCMPusher_SendBasicPushes,
	} {
		tf(t, s)
		teardown()
	}
}

func testFCMPusher_SendBasicPushes(t *testing.T, store push.TokenStore) {
	ctx := context.Background()

	fcmClient := &testFCMClient{}
	pusher := push.NewFCMPusher(zap.NewNop(), store, fcmClient)

	users := make([]*commonpb.UserId, 5)
	for i := 0; i < 5; i++ {
		users[i] = &commonpb.UserId{Value: []byte(fmt.Sprintf("user%d", i))}

		installId := &commonpb.AppInstallId{Value: fmt.Sprintf("install%d_1", i)}
		err := store.AddToken(ctx, users[i], installId, pushpb.TokenType_FCM_APNS, fmt.Sprintf("token%d_1", i))
		require.NoError(t, err)

		installId = &commonpb.AppInstallId{Value: fmt.Sprintf("install%d_2", i)}
		err = store.AddToken(ctx, users[i], installId, pushpb.TokenType_FCM_APNS, fmt.Sprintf("token%d_2", i))
		require.NoError(t, err)
	}

	targetUsers := users[:3]

	require.NoError(t, pusher.SendBasicPushes(ctx, "title", "body", targetUsers...))

	require.NotNil(t, fcmClient.sentMessage)

	require.Equal(t, "title", fcmClient.sentMessage.Notification.Title)
	require.Equal(t, "body", fcmClient.sentMessage.Notification.Body)
	require.Len(t, fcmClient.sentMessage.Tokens, 6)
	expectedTokens := []string{
		"token0_1", "token0_2",
		"token1_1", "token1_2",
		"token2_1", "token2_2",
	}
	require.ElementsMatch(t, expectedTokens, fcmClient.sentMessage.Tokens)
	require.Empty(t, fcmClient.sentMessage.Data)
}
