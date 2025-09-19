package activity

import (
	"context"
	"errors"

	"github.com/mr-tron/base58"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	activitypb "github.com/code-payments/flipcash-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codetransaction "github.com/code-payments/code-server/pkg/code/server/transaction"
	codecurrency "github.com/code-payments/code-server/pkg/currency"
	codequery "github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/pool"
)

const (
	defaultMaxNotifications = 100
)

var (
	errNotificationNotFound     = errors.New("notification not found")
	errDeniedNotificationAccess = errors.New("notification access is denied")
)

type Server struct {
	log *zap.Logger

	authz auth.Authorizer

	pools pool.Store

	codeData codedata.Provider

	activitypb.UnimplementedActivityFeedServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	pools pool.Store,
	codeData codedata.Provider,
) *Server {
	return &Server{
		log: log,

		authz: authz,

		pools: pools,

		codeData: codeData,
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

	notifications, err := s.getPagedNotifications(ctx, log, userID, req.Auth.GetKeyPair().PubKey, &commonpb.QueryOptions{
		PageSize: req.MaxItems,
		Order:    commonpb.QueryOptions_DESC,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}
	return &activitypb.GetLatestNotificationsResponse{Notifications: notifications}, nil
}

func (s *Server) GetPagedNotifications(ctx context.Context, req *activitypb.GetPagedNotificationsRequest) (*activitypb.GetPagedNotificationsResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("activity_feed_type", req.Type.String()),
	)

	notifications, err := s.getPagedNotifications(ctx, log, userID, req.Auth.GetKeyPair().PubKey, req.QueryOptions)
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}
	return &activitypb.GetPagedNotificationsResponse{Notifications: notifications}, nil
}

func (s *Server) GetBatchNotifications(ctx context.Context, req *activitypb.GetBatchNotificationsRequest) (*activitypb.GetBatchNotificationsResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.Int("notification_count", len(req.Ids)),
	)

	notifications, err := s.getBatchNotifications(ctx, log, userID, req.Auth.GetKeyPair().PubKey, req.Ids)
	switch err {
	case nil:
		return &activitypb.GetBatchNotificationsResponse{Notifications: notifications}, nil
	case errDeniedNotificationAccess:
		return &activitypb.GetBatchNotificationsResponse{Result: activitypb.GetBatchNotificationsResponse_DENIED}, nil
	case errNotificationNotFound:
		return &activitypb.GetBatchNotificationsResponse{Result: activitypb.GetBatchNotificationsResponse_NOT_FOUND}, nil
	default:
		return nil, status.Error(codes.Internal, "")
	}
}

func (s *Server) getPagedNotifications(ctx context.Context, log *zap.Logger, userID *commonpb.UserId, pubKey *commonpb.PublicKey, queryOptions *commonpb.QueryOptions) ([]*activitypb.Notification, error) {
	limit := defaultMaxNotifications
	if queryOptions.PageSize > 0 {
		limit = int(queryOptions.PageSize)
	}

	direction := codequery.Ascending
	if queryOptions.Order == commonpb.QueryOptions_DESC {
		direction = codequery.Descending
	}

	var pagingToken *string
	if queryOptions.PagingToken != nil {
		pagingToken = pointer.String(base58.Encode(queryOptions.PagingToken.Value))
	}

	notifications, err := s.getNotificationsFromPagedIntents(ctx, log, userID, pubKey, pagingToken, direction, limit)
	if err != nil {
		log.Warn("Failed to get notifications", zap.Error(err))
		return nil, err
	}
	return notifications, nil
}

func (s *Server) getNotificationsFromPagedIntents(ctx context.Context, log *zap.Logger, userID *commonpb.UserId, pubKey *commonpb.PublicKey, pagingToken *string, direction codequery.Ordering, limit int) ([]*activitypb.Notification, error) {
	userOwnerAccount, err := codecommon.NewAccountFromPublicKeyBytes(pubKey.Value)
	if err != nil {
		return nil, err
	}

	queryOptions := []codequery.Option{
		codequery.WithDirection(direction),
		codequery.WithLimit(uint64(limit)),
	}
	if pagingToken != nil {
		intentRecord, err := s.codeData.GetIntent(ctx, *pagingToken)
		if err != nil {
			return nil, err
		}
		queryOptions = append(queryOptions, codequery.WithCursor(codequery.ToCursor(uint64(intentRecord.Id))))
	}

	intentRecords, err := s.codeData.GetAllIntentsByOwner(
		ctx,
		userOwnerAccount.PublicKey().ToBase58(),
		queryOptions...,
	)
	if err == codeintent.ErrIntentNotFound {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return s.toLocalizedNotifications(ctx, log, userID, userOwnerAccount, intentRecords)
}

func (s *Server) getBatchNotifications(ctx context.Context, log *zap.Logger, userID *commonpb.UserId, pubKey *commonpb.PublicKey, ids []*activitypb.NotificationId) ([]*activitypb.Notification, error) {
	notifications, err := s.getNotificationsFromBatchIntents(ctx, log, userID, pubKey, ids)
	if err != nil {
		log.Warn("Failed to get notifications", zap.Error(err))
		return nil, err
	}
	return notifications, nil
}

func (s *Server) getNotificationsFromBatchIntents(ctx context.Context, log *zap.Logger, userID *commonpb.UserId, pubKey *commonpb.PublicKey, ids []*activitypb.NotificationId) ([]*activitypb.Notification, error) {
	userOwnerAccount, err := codecommon.NewAccountFromPublicKeyBytes(pubKey.Value)
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	// todo: fetch via a batched DB called
	var intentRecords []*codeintent.Record
	for _, id := range ids {
		intentID := base58.Encode(id.Value)

		log := log.With(zap.String("notification_id", intentID))

		intentRecord, err := s.codeData.GetIntent(ctx, intentID)
		switch err {
		case nil:
		case codeintent.ErrIntentNotFound:
			return nil, errNotificationNotFound
		default:
			log.Warn("Failed to get intent", zap.Error(err))
			return nil, err
		}

		var destinationOwner string
		switch intentRecord.IntentType {
		case codeintent.SendPublicPayment:
			destinationOwner = intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount
		case codeintent.ReceivePaymentsPublicly:
		case codeintent.ExternalDeposit:
		default:
			return nil, errNotificationNotFound
		}
		if userOwnerAccount.PublicKey().ToBase58() != intentRecord.InitiatorOwnerAccount && userOwnerAccount.PublicKey().ToBase58() != destinationOwner {
			return nil, errDeniedNotificationAccess
		}
		intentRecords = append(intentRecords, intentRecord)
	}

	return s.toLocalizedNotifications(ctx, log, userID, userOwnerAccount, intentRecords)
}

func (s *Server) toLocalizedNotifications(ctx context.Context, log *zap.Logger, userID *commonpb.UserId, userOwnerAccount *codecommon.Account, intentRecords []*codeintent.Record) ([]*activitypb.Notification, error) {
	welcomeBonusIntentID := codetransaction.GetAirdropIntentId(codetransaction.AirdropTypeWelcomeBonus, userOwnerAccount.PublicKey().ToBase58())

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
			State:         activitypb.NotificationState_NOTIFICATION_STATE_COMPLETED,
		}

		mintAccount, err := codecommon.NewAccountFromPublicKeyString(intentRecord.MintAccount)
		if err != nil {
			return nil, err
		}

		switch intentRecord.IntentType {
		case codeintent.SendPublicPayment:
			intentMetadata := intentRecord.SendPublicPaymentMetadata
			notification.PaymentAmount = &commonpb.CryptoPaymentAmount{
				Currency:     string(intentMetadata.ExchangeCurrency),
				NativeAmount: intentMetadata.NativeAmount,
				Quarks:       intentMetadata.Quantity,
			}

			destinationAccount, err := codecommon.NewAccountFromPublicKeyString(intentMetadata.DestinationTokenAccount)
			if err != nil {
				return nil, err
			}

			bettingPool, err := s.pools.GetPoolByFundingDestination(ctx, &commonpb.PublicKey{Value: destinationAccount.PublicKey().ToBytes()})
			if err != nil && err != pool.ErrPoolNotFound {
				return nil, err
			}
			isBettingPoolPayment := bettingPool != nil

			if intentRecord.InitiatorOwnerAccount == userOwnerAccount.PublicKey().ToBase58() {
				if intentMetadata.IsRemoteSend {
					isClaimed, err := isGiftCardClaimed(ctx, s.codeData, destinationAccount)
					if err != nil {
						return nil, err
					}

					notification.AdditionalMetadata = &activitypb.Notification_SentCrypto{SentCrypto: &activitypb.SentCryptoNotificationMetadata{
						Vault:                   &commonpb.PublicKey{Value: destinationAccount.ToProto().Value},
						CanInitiateCancelAction: !isClaimed,
					}}
					if !isClaimed {
						notification.State = activitypb.NotificationState_NOTIFICATION_STATE_PENDING
					}
				} else if intentMetadata.IsWithdrawal {
					notification.AdditionalMetadata = &activitypb.Notification_WithdrewCrypto{WithdrewCrypto: &activitypb.WithdrewCryptoNotificationMetadata{}}
				} else if isBettingPoolPayment {
					notification.AdditionalMetadata = &activitypb.Notification_PaidCrypto{PaidCrypto: &activitypb.PaidCryptoNotificationMetadata{
						PaymentMetadata: &activitypb.PaidCryptoNotificationMetadata_Pool{
							Pool: &activitypb.PaidCryptoNotificationMetadata_PoolPaymentMetadata{
								PoolId: bettingPool.ID,
							},
						},
					}}
				} else {
					notification.AdditionalMetadata = &activitypb.Notification_GaveCrypto{GaveCrypto: &activitypb.GaveCryptoNotificationMetadata{}}
				}
			} else {
				if intentRecord.IntentId == welcomeBonusIntentID {
					notification.AdditionalMetadata = &activitypb.Notification_WelcomeBonus{WelcomeBonus: &activitypb.WelcomeBonusNotificationMetadata{}}
				} else if intentMetadata.IsWithdrawal {
					notification.AdditionalMetadata = &activitypb.Notification_DepositedCrypto{DepositedCrypto: &activitypb.DepositedCryptoNotificationMetadata{}}
				} else if isBettingPoolPayment {
					// Show nothing on receiver side for reiving a bet payment to their pool
					continue
				} else {
					notification.AdditionalMetadata = &activitypb.Notification_ReceivedCrypto{ReceivedCrypto: &activitypb.ReceivedCryptoNotificationMetadata{}}
				}
			}

		case codeintent.ReceivePaymentsPublicly:
			intentMetadata := intentRecord.ReceivePaymentsPubliclyMetadata

			if intentMetadata.IsIssuerVoidingGiftCard || intentMetadata.IsReturned {
				continue
			}

			notification.PaymentAmount = &commonpb.CryptoPaymentAmount{
				Currency:     string(intentMetadata.OriginalExchangeCurrency),
				NativeAmount: intentMetadata.OriginalNativeAmount,
				Quarks:       intentMetadata.Quantity,
			}
			notification.AdditionalMetadata = &activitypb.Notification_ReceivedCrypto{ReceivedCrypto: &activitypb.ReceivedCryptoNotificationMetadata{}}

		case codeintent.ExternalDeposit:
			intentMetadata := intentRecord.ExternalDepositMetadata
			notification.PaymentAmount = &commonpb.CryptoPaymentAmount{
				Currency:     string(codecurrency.USD),
				NativeAmount: intentMetadata.UsdMarketValue,
				Quarks:       intentMetadata.Quantity,
			}
			notification.AdditionalMetadata = &activitypb.Notification_DepositedCrypto{DepositedCrypto: &activitypb.DepositedCryptoNotificationMetadata{}}

		case codeintent.PublicDistribution:
			intentMetadata := intentRecord.PublicDistributionMetadata

			sourceAccount, err := codecommon.NewAccountFromPublicKeyString(intentMetadata.Source)
			if err != nil {
				return nil, err
			}

			bettingPool, err := s.pools.GetPoolByFundingDestination(ctx, &commonpb.PublicKey{Value: sourceAccount.PublicKey().ToBytes()})
			if err != nil {
				return nil, err
			}

			var userDistribution *codeintent.Distribution
			for _, distribution := range intentMetadata.Distributions {
				if distribution.DestinationOwnerAccount == userOwnerAccount.PublicKey().ToBase58() {
					userDistribution = distribution
					break
				}
			}
			if userDistribution == nil {
				continue
			}

			var userOutcome poolpb.UserOutcome
			var nativeAmount float64
			userPoolSummary, err := pool.GetUserSummary(ctx, s.pools, s.codeData, userID, bettingPool)
			if err != nil {
				return nil, err
			}
			switch typed := userPoolSummary.Outcome.(type) {
			case *poolpb.UserPoolSummary_Refund:
				userOutcome = poolpb.UserOutcome_REFUND_OUTCOME
				nativeAmount = typed.Refund.AmountRefunded.NativeAmount
			case *poolpb.UserPoolSummary_Win:
				userOutcome = poolpb.UserOutcome_WIN_OUTCOME
				nativeAmount = typed.Win.TotalAmountReceived.NativeAmount
			default:
				return nil, errors.New("unexpected user pool outcome")
			}

			notification.PaymentAmount = &commonpb.CryptoPaymentAmount{
				Currency:     string(bettingPool.BuyInCurrency),
				NativeAmount: nativeAmount,
				Quarks:       userDistribution.Quantity,
			}
			notification.AdditionalMetadata = &activitypb.Notification_DistributedCrypto{DistributedCrypto: &activitypb.DistributedCryptoNotificationMetadata{
				DistributionMetadata: &activitypb.DistributedCryptoNotificationMetadata_Pool{
					Pool: &activitypb.DistributedCryptoNotificationMetadata_PoolDistributionMetadata{
						PoolId:  bettingPool.ID,
						Outcome: userOutcome,
					},
				},
			}}

		default:
			continue
		}

		if notification.PaymentAmount != nil {
			notification.PaymentAmount.Mint = &commonpb.PublicKey{Value: mintAccount.ToProto().Value}
		}

		notifications = append(notifications, notification)
	}

	for _, notification := range notifications {
		log := log.With(zap.String("notification_id", NotificationIDString(notification.Id)))

		err := InjectLocalizedText(ctx, s.codeData, userOwnerAccount, notification)
		if err != nil {
			log.Warn("Failed to inject localized notification text", zap.Error(err))
			return nil, err
		}
	}
	return notifications, nil
}
