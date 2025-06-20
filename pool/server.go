package pool

import (
	"bytes"
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
)

const (
	MaxParticipants = 100
	maxTsDelta      = time.Minute
)

type Server struct {
	log   *zap.Logger
	auth  auth.Authorizer
	pools Store

	poolpb.UnimplementedPoolServer
}

func NewServer(log *zap.Logger, auth auth.Authorizer, pools Store) *Server {
	return &Server{
		log:   log,
		auth:  auth,
		pools: pools,
	}
}

// todo: Add buy in amount validation (min/max)
func (s *Server) CreatePool(ctx context.Context, req *poolpb.CreatePoolRequest) (*poolpb.CreatePoolResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("pool_id", PoolIDString(req.Pool.Id)),
	)

	if !VerifyPoolSignature(req.Pool, req.RendezvousSignature) {
		return nil, status.Error(codes.PermissionDenied, "")
	}
	if !req.Pool.IsOpen {
		return nil, status.Error(codes.InvalidArgument, "pool.is_open must be true")
	}
	if req.Pool.Resolution != nil {
		return nil, status.Error(codes.InvalidArgument, "pool.resolution cannot be set")
	}
	if req.Pool.CreatedAt.Nanos > 0 {
		return nil, status.Error(codes.InvalidArgument, "pool.created_at.nanos cannot be set")
	}
	if req.Pool.CreatedAt.AsTime().After(time.Now().Add(maxTsDelta)) {
		return nil, status.Error(codes.InvalidArgument, "pool.created_at is invalid")
	} else if req.Pool.CreatedAt.AsTime().Before(time.Now().Add(-maxTsDelta)) {
		return nil, status.Error(codes.InvalidArgument, "pool.created_at is invalid")
	}

	model := ToPoolModel(req.Pool, req.RendezvousSignature)

	err = s.pools.CreatePool(ctx, model)
	switch err {
	case nil:
	case ErrPoolIDExists:
		return &poolpb.CreatePoolResponse{Result: poolpb.CreatePoolResponse_RENDEZVOUS_EXISTS}, nil
	case ErrPoolFundingDestinationExists:
		return &poolpb.CreatePoolResponse{Result: poolpb.CreatePoolResponse_FUNDING_DESTINATION_EXISTS}, nil
	default:
		log.With(zap.Error(err)).Warn("Failure persisting pool")
		return nil, status.Error(codes.Internal, "failure persisting pool")
	}

	return &poolpb.CreatePoolResponse{}, nil
}

func (s *Server) GetPool(ctx context.Context, req *poolpb.GetPoolRequest) (*poolpb.GetPoolResponse, error) {
	log := s.log.With(zap.String("pool_id", PoolIDString(req.Id)))

	pool, err := s.pools.GetPoolByID(ctx, req.Id)
	switch err {
	case nil:
	case ErrPoolNotFound:
		return &poolpb.GetPoolResponse{Result: poolpb.GetPoolResponse_NOT_FOUND}, nil
	default:
		log.With(zap.Error(err)).Warn("Failure getting pool")
		return nil, status.Error(codes.Internal, "failure getting pool")
	}

	bets, err := s.pools.GetBetsByPool(ctx, req.Id)
	switch err {
	case nil, ErrBetNotFound:
	default:
		log.With(zap.Error(err)).Warn("Failure getting bets")
		return nil, status.Error(codes.Internal, "failure getting bets")
	}

	protoPool := pool.ToProto()

	for _, bet := range bets {
		// log := log.With(zap.String("bet_id", BetIDString(bet.ID)))

		// todo: verify bet has been paid for

		protoPool.Bets = append(protoPool.Bets, bet.ToProto())
	}

	return &poolpb.GetPoolResponse{Pool: protoPool}, nil
}

func (s *Server) ResolvePool(ctx context.Context, req *poolpb.ResolvePoolRequest) (*poolpb.ResolvePoolResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("pool_id", PoolIDString(req.Id)),
	)

	pool, err := s.pools.GetPoolByID(ctx, req.Id)
	switch err {
	case nil:
	case ErrPoolNotFound:
		return &poolpb.ResolvePoolResponse{Result: poolpb.ResolvePoolResponse_NOT_FOUND}, nil
	default:
		log.With(zap.Error(err)).Warn("Failure getting pool")
		return nil, status.Error(codes.Internal, "failure getting pool")
	}

	if !bytes.Equal(userID.Value, pool.CreatorID.Value) {
		return &poolpb.ResolvePoolResponse{Result: poolpb.ResolvePoolResponse_DENIED}, nil
	}
	if pool.Resolution != nil {
		if *pool.Resolution != req.Resolution.GetBooleanResolution() {
			return &poolpb.ResolvePoolResponse{Result: poolpb.ResolvePoolResponse_DIFFERENT_OUTCOME_DECLARED}, nil
		}
		return &poolpb.ResolvePoolResponse{}, nil
	}

	verifiedProtoPool := pool.ToProto().VerifiedMetadata
	verifiedProtoPool.IsOpen = false
	verifiedProtoPool.Resolution = req.Resolution
	if !VerifyPoolSignature(verifiedProtoPool, req.NewRendezvousSignature) {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	err = s.pools.ResolvePool(ctx, req.Id, req.Resolution.GetBooleanResolution(), req.NewRendezvousSignature)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure persisting pool resolution")
		return nil, status.Error(codes.Internal, "failure persisting pool resolution")
	}

	return &poolpb.ResolvePoolResponse{}, nil
}

func (s *Server) MakeBet(ctx context.Context, req *poolpb.MakeBetRequest) (*poolpb.MakeBetResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("pool_id", PoolIDString(req.PoolId)),
		zap.String("bet_id", BetIDString(req.Bet.BetId)),
	)

	if !VerifyBetSignature(req.PoolId, req.Bet, req.RendezvousSignature) {
		return nil, status.Error(codes.PermissionDenied, "")
	}
	if req.Bet.Ts.Nanos > 0 {
		return nil, status.Error(codes.InvalidArgument, "bet.ts.nanos cannot be set")
	}
	if req.Bet.Ts.AsTime().After(time.Now().Add(maxTsDelta)) {
		return nil, status.Error(codes.InvalidArgument, "bet.ts is invalid")
	} else if req.Bet.Ts.AsTime().Before(time.Now().Add(-maxTsDelta)) {
		return nil, status.Error(codes.InvalidArgument, "bet.ts is invalid")
	}

	pool, err := s.pools.GetPoolByID(ctx, req.PoolId)
	switch err {
	case nil:
	case ErrBetNotFound:
		return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_POOL_NOT_FOUND}, nil
	default:
		log.With(zap.Error(err)).Warn("Failure getting pool")
		return nil, status.Error(codes.Internal, "failure getting pool")
	}
	if !pool.IsOpen {
		return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_POOL_CLOSED}, nil
	}

	model := ToBetModel(req.PoolId, req.Bet, req.RendezvousSignature)

	err = s.pools.CreateBet(ctx, model)
	switch err {
	case nil:
	case ErrBetExists:
		existing, err := s.pools.GetBetByUser(ctx, req.PoolId, userID)
		switch err {
		case nil:
			// User made a bet with a different ID for this pool
			if !bytes.Equal(existing.ID.Value, req.Bet.BetId.Value) {
				return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
			}
			// User made a bet with a different outcome for this pool
			if existing.SelectedOutcome != req.Bet.SelectedOutcome.GetBooleanOutcome() {
				return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
			}

			// We can proceed with an OK response, RPC call is a no-op for an existing bet
		case ErrBetNotFound:
			// Someone else made a bet with the same bet ID. This is unlikely to
			// happen in practice.
			return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
		default:
			log.With(zap.Error(err)).Warn("Failure getting bet")
			return nil, status.Error(codes.Internal, "failure getting bet")
		}
	case ErrMaxBetCountExceeded:
		existing, err := s.pools.GetBetByUser(ctx, req.PoolId, userID)
		switch err {
		case nil:
			// User made a bet with a different ID for this pool
			if !bytes.Equal(existing.ID.Value, req.Bet.BetId.Value) {
				return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
			}
			// User made a bet with a different outcome for this pool
			if existing.SelectedOutcome != req.Bet.SelectedOutcome.GetBooleanOutcome() {
				return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
			}

			// We can proceed with an OK response, RPC call is a no-op for an existing bet
		case ErrBetNotFound:
			// The user doesn't have a bet. We've reached the limit
			return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MAX_BETS_RECEIVED}, nil
		default:
			log.With(zap.Error(err)).Warn("Failure getting bet")
			return nil, status.Error(codes.Internal, "failure getting bet")
		}
	default:
		log.With(zap.Error(err)).Warn("Failure persisting bet")
		return nil, status.Error(codes.Internal, "failure persisting bet")
	}

	return &poolpb.MakeBetResponse{}, nil
}
