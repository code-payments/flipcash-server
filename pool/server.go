package pool

import (
	"bytes"
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
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

	model, err := ToPoolModel(req.Pool, req.RendezvousSignature)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure converting pool to model")
		return nil, status.Error(codes.Internal, "failure converting pool to model")
	}

	// todo: add duplication checks (rendezvous and funding)
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

	pool, err := s.pools.GetPool(ctx, req.Id)
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

	protoPool, err := pool.ToProto()
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure converting pool to proto")
		return nil, status.Error(codes.Internal, "failure converting pool to proto")
	}

	for _, bet := range bets {
		log := log.With(zap.String("bet_id", BetIDString(bet.ID)))

		betProto, err := bet.ToProto()
		if err != nil {
			log.With(zap.Error(err)).Warn("Failure converting bet to proto")
			return nil, status.Error(codes.Internal, "failure converting bet to proto")
		}

		// todo: verify bet has been paid for

		protoPool.Bets = append(protoPool.Bets, betProto)
	}

	return &poolpb.GetPoolResponse{Pool: protoPool}, nil
}

func (s *Server) DeclarePoolOutcome(ctx context.Context, req *poolpb.DeclarePoolOutcomeRequest) (*poolpb.DeclarePoolOutcomeResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("pool_id", PoolIDString(req.Id)),
	)

	pool, err := s.pools.GetPool(ctx, req.Id)
	switch err {
	case nil:
	case ErrPoolNotFound:
		return &poolpb.DeclarePoolOutcomeResponse{Result: poolpb.DeclarePoolOutcomeResponse_NOT_FOUND}, nil
	default:
		log.With(zap.Error(err)).Warn("Failure getting pool")
		return nil, status.Error(codes.Internal, "failure getting pool")
	}

	if !bytes.Equal(userID.Value, pool.Creator.Value) {
		return &poolpb.DeclarePoolOutcomeResponse{Result: poolpb.DeclarePoolOutcomeResponse_DENIED}, nil
	}
	if pool.Resolution != nil {
		if *pool.Resolution != req.Resolution.GetBooleanResolution() {
			return &poolpb.DeclarePoolOutcomeResponse{Result: poolpb.DeclarePoolOutcomeResponse_DIFFERENT_OUTCOME_DECLARED}, nil
		}
		return &poolpb.DeclarePoolOutcomeResponse{}, nil
	}

	protoPool, err := pool.ToProto()
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure converting pool to proto")
		return nil, status.Error(codes.Internal, "failure converting pool to proto")
	}
	protoPool.VerifiedMetadata.IsOpen = false
	protoPool.VerifiedMetadata.Resolution = req.Resolution
	if !VerifyPoolSignature(protoPool.VerifiedMetadata, req.NewRendezvousSignature) {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	err = s.pools.ResolvePool(ctx, req.Id, req.Resolution.GetBooleanResolution(), req.NewRendezvousSignature)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure persisting pool resolution")
		return nil, status.Error(codes.Internal, "failure persisting pool resolution")
	}

	return &poolpb.DeclarePoolOutcomeResponse{}, nil
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

	model, err := ToBetModel(req.PoolId, req.Bet, req.RendezvousSignature)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure converting bet to model")
		return nil, status.Error(codes.Internal, "failure converting bet to model")
	}

	pool, err := s.pools.GetPool(ctx, req.PoolId)
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

	err = s.pools.CreateBet(ctx, model)
	switch err {
	case nil:
	case ErrBetExists:
		existing, err := s.pools.GetBetByUser(ctx, req.PoolId, userID)
		if err != nil {
			log.With(zap.Error(err)).Warn("Failure getting bet")
			return nil, status.Error(codes.Internal, "failure getting bet")
		}

		if !bytes.Equal(existing.ID.Value, req.Bet.BetId.Value) {
			return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
		}
		if existing.SelectedOutcome != req.Bet.SelectedOutcome.GetBooleanOutcome() {
			return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
		}
	default:
		log.With(zap.Error(err)).Warn("Failure persisting bet")
		return nil, status.Error(codes.Internal, "failure persisting bet")
	}

	return &poolpb.MakeBetResponse{}, nil
}
