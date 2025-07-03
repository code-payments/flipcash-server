package pool

import (
	"bytes"
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codecommonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeaccount "github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/flipcash-server/account"
	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/database"
	"github.com/code-payments/flipcash-server/model"
)

const (
	MaxParticipants      = 100
	defaultMaxPagedPools = 1024
	maxTsDelta           = time.Minute
)

type Server struct {
	log      *zap.Logger
	auth     auth.Authorizer
	accounts account.Store
	pools    Store
	codeData codedata.Provider

	poolpb.UnimplementedPoolServer
}

func NewServer(log *zap.Logger, auth auth.Authorizer, accounts account.Store, pools Store, codeData codedata.Provider) *Server {
	return &Server{
		log:      log,
		auth:     auth,
		accounts: accounts,
		pools:    pools,
		codeData: codeData,
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

	isRegistered, err := s.accounts.IsRegistered(ctx, userID)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure getting user registration status")
		return nil, status.Error(codes.Internal, "failure getting user registration status")
	}
	if !isRegistered {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	if !VerifyPoolSignature(log, req.Pool, req.RendezvousSignature) {
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

	isValid, reason, err := s.validatePoolFundingDestination(ctx, req.Auth.GetKeyPair().PubKey, req.Pool.Id, req.Pool.FundingDestination)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure validating funding destination")
		return nil, status.Error(codes.Internal, "failure validating funding destination")
	} else if !isValid {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	model := ToPoolModel(req.Pool, req.RendezvousSignature)

	err = database.ExecuteTxWithinCtx(ctx, func(ctx context.Context) error {
		return s.pools.CreatePool(ctx, model)
	})
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

func (s *Server) validatePoolFundingDestination(ctx context.Context, owner *commonpb.PublicKey, poolId *poolpb.PoolId, fundingDestination *commonpb.PublicKey) (bool, string, error) {
	poolAccount, err := codecommon.NewAccountFromPublicKeyBytes(poolId.Value)
	if err != nil {
		return false, "", err
	}

	ownerAccount, err := codecommon.NewAccountFromPublicKeyBytes(owner.Value)
	if err != nil {
		return false, "", err
	}

	fundingDestinationAccount, err := codecommon.NewAccountFromPublicKeyBytes(fundingDestination.Value)
	if err != nil {
		return false, "", err
	}

	timelockVault, err := poolAccount.ToTimelockVault(codecommon.CodeVmAccount, codecommon.CoreMintAccount)
	if err != nil {
		return false, "", err
	}
	if bytes.Equal(timelockVault.PublicKey().ToBytes(), fundingDestination.Value) {
		return false, "pool.id is the private key for funding_destination", err
	}

	accountInfoRecord, err := s.codeData.GetAccountInfoByTokenAddress(ctx, fundingDestinationAccount.PublicKey().ToBase58())
	switch err {
	case nil, codeaccount.ErrAccountInfoNotFound:
		if accountInfoRecord == nil || accountInfoRecord.AccountType != codecommonpb.AccountType_POOL {
			return false, "pool.funding_destination is not a code pool account", nil
		}
		if accountInfoRecord.OwnerAccount != ownerAccount.PublicKey().ToBase58() {
			return false, "pool.funding_destination is not your code pool account", nil
		}
		return true, "", nil
	default:
		return false, "", err
	}
}

func (s *Server) GetPool(ctx context.Context, req *poolpb.GetPoolRequest) (*poolpb.GetPoolResponse, error) {
	log := s.log.With(zap.String("pool_id", PoolIDString(req.Id)))

	protoPool, err := s.getProtoPool(ctx, req.Id, nil, !req.ExcludeBets)
	if err == ErrPoolNotFound {
		return &poolpb.GetPoolResponse{Result: poolpb.GetPoolResponse_NOT_FOUND}, nil
	} else if err != nil {
		log.With(zap.Error(err)).Warn("Failure getting pool with bets")
		return nil, status.Error(codes.Internal, "failure getting pool with bets")
	}

	return &poolpb.GetPoolResponse{Pool: protoPool}, nil
}

func (s *Server) GetPagedPools(ctx context.Context, req *poolpb.GetPagedPoolsRequest) (*poolpb.GetPagedPoolsResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(zap.String("user_id", model.UserIDString(userID)))

	isRegistered, err := s.accounts.IsRegistered(ctx, userID)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure getting user registration status")
		return nil, status.Error(codes.Internal, "failure getting user registration status")
	}
	if !isRegistered {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	if req.QueryOptions != nil {
		if req.QueryOptions.PageSize <= 0 {
			req.QueryOptions.PageSize = defaultMaxPagedPools
		}

		if req.QueryOptions.PagingToken != nil && len(req.QueryOptions.PagingToken.Value) != 8 {
			return nil, status.Error(codes.InvalidArgument, "query_ptions.paging_token.value length must be 8")
		}
	}

	memberships, err := s.pools.GetPagedMembers(ctx, userID, database.FromProtoQueryOptions(req.QueryOptions)...)
	if err == ErrMemberNotFound {
		return &poolpb.GetPagedPoolsResponse{Result: poolpb.GetPagedPoolsResponse_NOT_FOUND}, nil
	} else if err != nil {
		log.With(zap.Error(err)).Warn("Failure getting user memberships")
		return nil, status.Error(codes.Internal, "failure getting user memberships")
	}
	if len(memberships) == 0 {
		return &poolpb.GetPagedPoolsResponse{Result: poolpb.GetPagedPoolsResponse_NOT_FOUND}, nil
	}

	protoPools := make([]*poolpb.PoolMetadata, len(memberships))
	for i, membership := range memberships {
		log := log.With(zap.String("pool_id", PoolIDString(membership.PoolID)))

		protoPool, err := s.getProtoPool(ctx, membership.PoolID, userID, true)
		if err != nil {
			log.With(zap.Error(err)).Warn("Failure getting pool with bets")
			return nil, status.Error(codes.Internal, "failure getting pool with bets")
		}
		protoPool.PagingToken = &commonpb.PagingToken{Value: membership.ID}

		protoPools[i] = protoPool
	}
	return &poolpb.GetPagedPoolsResponse{Pools: protoPools}, nil
}

func (s *Server) ClosePool(ctx context.Context, req *poolpb.ClosePoolRequest) (*poolpb.ClosePoolResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("pool_id", PoolIDString(req.Id)),
	)

	isRegistered, err := s.accounts.IsRegistered(ctx, userID)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure getting user registration status")
		return nil, status.Error(codes.Internal, "failure getting user registration status")
	}
	if !isRegistered {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	if req.ClosedAt.Nanos > 0 {
		return nil, status.Error(codes.InvalidArgument, "closed_at.nanos cannot be set")
	}
	if req.ClosedAt.AsTime().After(time.Now().Add(maxTsDelta)) {
		return nil, status.Error(codes.InvalidArgument, "closed_at is invalid")
	} else if req.ClosedAt.AsTime().Before(time.Now().Add(-maxTsDelta)) {
		return nil, status.Error(codes.InvalidArgument, "closed_at is invalid")
	}

	pool, err := s.pools.GetPoolByID(ctx, req.Id)
	switch err {
	case nil:
	case ErrPoolNotFound:
		return &poolpb.ClosePoolResponse{Result: poolpb.ClosePoolResponse_NOT_FOUND}, nil
	default:
		log.With(zap.Error(err)).Warn("Failure getting pool")
		return nil, status.Error(codes.Internal, "failure getting pool")
	}

	if !bytes.Equal(userID.Value, pool.CreatorID.Value) {
		return &poolpb.ClosePoolResponse{Result: poolpb.ClosePoolResponse_DENIED}, nil
	}
	if !pool.IsOpen {
		return &poolpb.ClosePoolResponse{}, nil
	}

	verifiedProtoPool := pool.ToProto().VerifiedMetadata
	verifiedProtoPool.IsOpen = false
	verifiedProtoPool.ClosedAt = req.ClosedAt
	if !VerifyPoolSignature(log, verifiedProtoPool, req.NewRendezvousSignature) {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	err = database.ExecuteTxWithinCtx(ctx, func(ctx context.Context) error {
		return s.pools.ClosePool(ctx, req.Id, req.ClosedAt.AsTime(), req.NewRendezvousSignature)
	})
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure persisting pool closure")
		return nil, status.Error(codes.Internal, "failure persisting pool closure")
	}

	return &poolpb.ClosePoolResponse{}, nil
}

func (s *Server) ResolvePool(ctx context.Context, req *poolpb.ResolvePoolRequest) (*poolpb.ResolvePoolResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	resolution := ToResolution(req.Resolution)

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("pool_id", PoolIDString(req.Id)),
		zap.String("resolution", resolution.String()),
	)

	isRegistered, err := s.accounts.IsRegistered(ctx, userID)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure getting user registration status")
		return nil, status.Error(codes.Internal, "failure getting user registration status")
	}
	if !isRegistered {
		return nil, status.Error(codes.PermissionDenied, "")
	}

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
	if pool.IsOpen {
		return &poolpb.ResolvePoolResponse{Result: poolpb.ResolvePoolResponse_POOL_OPEN}, nil
	}
	if pool.HasResolution() {
		if pool.Resolution != resolution {
			return &poolpb.ResolvePoolResponse{Result: poolpb.ResolvePoolResponse_DIFFERENT_OUTCOME_DECLARED}, nil
		}
		return &poolpb.ResolvePoolResponse{}, nil
	}

	verifiedProtoPool := pool.ToProto().VerifiedMetadata
	verifiedProtoPool.Resolution = req.Resolution
	if !VerifyPoolSignature(log, verifiedProtoPool, req.NewRendezvousSignature) {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	err = database.ExecuteTxWithinCtx(ctx, func(ctx context.Context) error {
		return s.pools.ResolvePool(ctx, req.Id, resolution, req.NewRendezvousSignature)
	})
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

	isRegistered, err := s.accounts.IsRegistered(ctx, userID)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure getting user registration status")
		return nil, status.Error(codes.Internal, "failure getting user registration status")
	}
	if !isRegistered {
		return nil, status.Error(codes.PermissionDenied, "")
	}

	if !VerifyBetSignature(log, req.PoolId, req.Bet, req.RendezvousSignature) {
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

	existing, err := s.pools.GetBetByUser(ctx, model.PoolID, userID)
	switch err {
	case nil:
		// User has already made a bet with a different ID
		if !bytes.Equal(existing.ID.Value, model.ID.Value) {
			return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
		}
		if !bytes.Equal(existing.PayoutDestination.Value, model.PayoutDestination.Value) {
			return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MULTIPLE_BETS}, nil
		}

		isPaid, err := existing.IsPaid(ctx, s.codeData, s.pools, pool)
		if err != nil {
			return nil, err
		}
		if isPaid {
			return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_BET_OUTCOME_SOLIDIFIED}, nil
		}

		// User hasn't changed their selection, the RPC call is a no-op. Preserve
		// the original bet metadata.
		if existing.SelectedOutcome == model.SelectedOutcome {
			return &poolpb.MakeBetResponse{}, nil
		}

		err = s.pools.UpdateBetOutcome(ctx, model.ID, model.SelectedOutcome, model.Signature, model.Ts)
		if err != nil {
			log.With(zap.Error(err)).Warn("Failure updating bet outcome")
			return nil, status.Error(codes.Internal, "failure updating bet outcome")
		}
	case ErrBetNotFound:
	default:
		log.With(zap.Error(err)).Warn("Failure getting existing bet")
		return nil, status.Error(codes.Internal, "failure getting existing bet")
	}

	isValid, reason, err := s.validateBetPayoutDestination(ctx, req.Auth.GetKeyPair().PubKey, req.Bet.PayoutDestination)
	if err != nil {
		log.With(zap.Error(err)).Warn("Failure validating payout destination")
		return nil, status.Error(codes.Internal, "failure validating payout destination")
	} else if !isValid {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	err = database.ExecuteTxWithinCtx(ctx, func(ctx context.Context) error {
		return s.pools.CreateBet(ctx, model)
	})
	switch err {
	case nil:
	case ErrMaxBetCountExceeded:
		return &poolpb.MakeBetResponse{Result: poolpb.MakeBetResponse_MAX_BETS_RECEIVED}, nil
	default:
		log.With(zap.Error(err)).Warn("Failure persisting new bet")
		return nil, status.Error(codes.Internal, "failure persisting new bet")
	}

	return &poolpb.MakeBetResponse{}, nil
}

func (s *Server) validateBetPayoutDestination(ctx context.Context, owner, payoutDestination *commonpb.PublicKey) (bool, string, error) {
	ownerAccount, err := codecommon.NewAccountFromPublicKeyBytes(owner.Value)
	if err != nil {
		return false, "", err
	}
	payoutDestinationAccount, err := codecommon.NewAccountFromPublicKeyBytes(payoutDestination.Value)
	if err != nil {
		return false, "", err
	}

	accountInfoRecord, err := s.codeData.GetAccountInfoByTokenAddress(ctx, payoutDestinationAccount.PublicKey().ToBase58())
	switch err {
	case nil, codeaccount.ErrAccountInfoNotFound:
		if accountInfoRecord == nil || accountInfoRecord.AccountType != codecommonpb.AccountType_PRIMARY {
			return false, "bet.payout_destination is not a code primary account", nil
		}
		if accountInfoRecord.OwnerAccount != ownerAccount.PublicKey().ToBase58() {
			return false, "bet.payout_destination is not your code primary account", nil
		}
		return true, "", nil
	default:
		return false, "", err
	}
}

func (s *Server) getProtoPool(ctx context.Context, id *poolpb.PoolId, requestingUser *commonpb.UserId, includeBets bool) (*poolpb.PoolMetadata, error) {
	pool, err := s.pools.GetPoolByID(ctx, id)
	if err != nil {
		return nil, err
	}

	protoPool := pool.ToProto()
	protoPool.IsFundingDestinationInitialized = true

	bets, err := s.pools.GetBetsByPool(ctx, id)
	switch err {
	case nil, ErrBetNotFound:
	default:
		return nil, err
	}

	var numYes, numNo int
	protoBets := make([]*poolpb.BetMetadata, len(bets))
	for i, bet := range bets {
		isPaid, err := bet.IsPaid(ctx, s.codeData, s.pools, pool)
		if err != nil {
			return nil, err
		}

		protoBet := bet.ToProto()
		protoBet.IsIntentSubmitted = isPaid
		protoBets[i] = protoBet

		if !isPaid {
			continue
		}

		if bet.SelectedOutcome {
			numYes++
		} else {
			numNo++
		}
	}

	protoPool.BetSummary = &poolpb.BetSummary{
		Kind: &poolpb.BetSummary_BooleanSummary{
			BooleanSummary: &poolpb.BetSummary_BooleanBetSummary{
				NumYes: uint32(numYes),
				NumNo:  uint32(numNo),
			},
		},
	}
	if includeBets {
		protoPool.Bets = protoBets
	}

	if requestingUser != nil && bytes.Equal(requestingUser.Value, protoPool.VerifiedMetadata.Creator.Value) {
		fundingDestinationAccount, err := codecommon.NewAccountFromPublicKeyBytes(pool.FundingDestination.Value)
		if err != nil {
			return nil, err
		}

		accountInfoRecord, err := s.codeData.GetAccountInfoByTokenAddress(ctx, fundingDestinationAccount.PublicKey().ToBase58())
		if err != nil {
			return nil, err
		}
		protoPool.DerivationIndex = accountInfoRecord.Index
	}

	return protoPool, nil
}
