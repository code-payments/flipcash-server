package activity

import (
	"context"

	"github.com/mr-tron/base58"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	activitypb "github.com/code-payments/flipcash-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codequery "github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
)

const (
	defaultMaxNotifications = 256
)

type Server struct {
	log      *zap.Logger
	authz    auth.Authorizer
	codeData codedata.Provider

	knownAirdropAccounts map[string]any

	activitypb.UnimplementedActivityFeedServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	codeData codedata.Provider,
	airdropOwnerPublicKeys []string,
) *Server {
	knownAirdropAccounts := make(map[string]any)
	for _, airdropOwnerPublicKey := range airdropOwnerPublicKeys {
		knownAirdropAccounts[airdropOwnerPublicKey] = struct{}{}
	}

	return &Server{
		log:      log,
		authz:    authz,
		codeData: codeData,

		knownAirdropAccounts: knownAirdropAccounts,
	}
}

func (s *Server) GetLatestNotifications(ctx context.Context, req *activitypb.GetLatestNotificationsRequest) (*activitypb.GetLatestNotificationsResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("activity_feed_type", req.Type.String()),
	)

	limit := defaultMaxNotifications
	if req.MaxItems > 0 {
		limit = int(req.MaxItems)
	}

	notifications, err := s.getLatestNotificationsFromIntents(ctx, req.Auth.GetKeyPair().PubKey, limit)
	if err != nil {
		log.Warn("Failed to get notifications", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get notifications")
	}

	notificationsWithLocalizedText := make([]*activitypb.Notification, 0)
	for _, notification := range notifications {
		log := log.With(zap.String("notification_id", NotificationIDString(notification.Id)))

		err = InjectLocalizedText(ctx, s.codeData, req.Auth.GetKeyPair().PubKey, notification)
		if err != nil {
			log.Warn("Failed to inject localized notification text", zap.Error(err))
			continue
		}
		notificationsWithLocalizedText = append(notificationsWithLocalizedText, notification)
	}

	return &activitypb.GetLatestNotificationsResponse{Notifications: notificationsWithLocalizedText}, nil
}

func (s *Server) getLatestNotificationsFromIntents(ctx context.Context, pubKey *commonpb.PublicKey, limit int) ([]*activitypb.Notification, error) {
	userOwnerAccount, err := codecommon.NewAccountFromPublicKeyBytes(pubKey.Value)
	if err != nil {
		return nil, err
	}

	intentRecords, err := s.codeData.GetAllIntentsByOwner(
		ctx,
		userOwnerAccount.PublicKey().ToBase58(),
		codequery.WithDirection(codequery.Descending),
		codequery.WithLimit(uint64(limit)),
	)
	if err == codeintent.ErrIntentNotFound {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var notifications []*activitypb.Notification
	for _, intentRecord := range intentRecords {
		rawNotificationID, err := base58.Decode(intentRecord.IntentId)
		if err != nil {
			return nil, err
		}

		notification := &activitypb.Notification{
			Id:            &activitypb.NotificationId{Value: rawNotificationID},
			LocalizedText: "",
			Ts:            timestamppb.New(intentRecord.CreatedAt),
		}

		switch intentRecord.IntentType {
		case codeintent.SendPublicPayment:
			intentMetadata := intentRecord.SendPublicPaymentMetadata
			notification.PaymentAmount = &commonpb.PaymentAmount{
				Currency:     string(intentMetadata.ExchangeCurrency),
				NativeAmount: intentMetadata.NativeAmount,
				Quarks:       intentMetadata.Quantity,
			}

			if intentRecord.InitiatorOwnerAccount == userOwnerAccount.PublicKey().ToBase58() {
				if intentMetadata.IsRemoteSend {
					vaultAccount, err := codecommon.NewAccountFromPublicKeyString(intentMetadata.DestinationTokenAccount)
					if err != nil {
						return nil, err
					}

					isClaimed, err := isGiftCardClaimed(ctx, s.codeData, vaultAccount)
					if err != nil {
						return nil, err
					}

					notification.AdditionalMetadata = &activitypb.Notification_SentUsdc{SentUsdc: &activitypb.SentUsdcNotificationMetadata{
						Vault:                   &commonpb.PublicKey{Value: vaultAccount.ToProto().Value},
						CanInitiateCancelAction: !isClaimed,
					}}
				} else if intentMetadata.IsWithdrawal {
					notification.AdditionalMetadata = &activitypb.Notification_WithdrewUsdc{WithdrewUsdc: &activitypb.WithdrewUsdcNotificationMetadata{}}
				} else {
					notification.AdditionalMetadata = &activitypb.Notification_GaveUsdc{GaveUsdc: &activitypb.GaveUsdcNotificationMetadata{}}
				}
			} else {
				_, ok := s.knownAirdropAccounts[intentRecord.InitiatorOwnerAccount]
				if ok {
					notification.AdditionalMetadata = &activitypb.Notification_WelcomeBonus{WelcomeBonus: &activitypb.WelcomeBonusNotificationMetadata{}}
				} else {
					notification.AdditionalMetadata = &activitypb.Notification_ReceivedUsdc{ReceivedUsdc: &activitypb.ReceivedUsdcNotificationMetadata{}}
				}
			}
		case codeintent.ReceivePaymentsPublicly:
			intentMetadata := intentRecord.ReceivePaymentsPubliclyMetadata

			vaultAccount, err := codecommon.NewAccountFromPublicKeyString(intentMetadata.Source)
			if err != nil {
				return nil, err
			}

			isIssuedByUser, err := isGiftCardIssuedByUser(ctx, s.codeData, userOwnerAccount, vaultAccount)
			if err != nil {
				return nil, err
			} else if isIssuedByUser {
				continue
			}

			notification.PaymentAmount = &commonpb.PaymentAmount{
				Currency:     string(intentMetadata.OriginalExchangeCurrency),
				NativeAmount: intentMetadata.OriginalNativeAmount,
				Quarks:       intentMetadata.Quantity,
			}
			notification.AdditionalMetadata = &activitypb.Notification_ReceivedUsdc{ReceivedUsdc: &activitypb.ReceivedUsdcNotificationMetadata{}}
		default:
			continue
		}

		notifications = append(notifications, notification)
	}

	return notifications, nil
}
