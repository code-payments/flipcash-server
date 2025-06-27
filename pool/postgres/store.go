package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	"github.com/code-payments/flipcash-server/database"
	"github.com/code-payments/flipcash-server/pool"
)

type store struct {
	pgxPool *pgxpool.Pool
}

func NewInPostgres(pgxPool *pgxpool.Pool) pool.Store {
	return &store{
		pgxPool: pgxPool,
	}
}

func (s *store) CreatePool(ctx context.Context, pool *pool.Pool) error {
	err := toPoolModel(pool).dbPut(ctx, s.pgxPool)
	if err != nil {
		return err
	}

	return toMemberModel(pool.CreatorID, pool.ID).dbPut(ctx, s.pgxPool)
}

func (s *store) ClosePool(ctx context.Context, poolID *poolpb.PoolId, closedAt time.Time, newSignature *commonpb.Signature) error {
	return dbClosePool(ctx, s.pgxPool, poolID, closedAt, newSignature)
}

func (s *store) ResolvePool(ctx context.Context, poolID *poolpb.PoolId, resolution pool.Resolution, newSignature *commonpb.Signature) error {
	return dbResolvePool(ctx, s.pgxPool, poolID, resolution, newSignature)
}

func (s *store) GetPoolByID(ctx context.Context, poolID *poolpb.PoolId) (*pool.Pool, error) {
	model, err := dbGetPoolByID(ctx, s.pgxPool, poolID)
	if err != nil {
		return nil, err
	}
	return fromPoolModel(model)
}

func (s *store) GetPoolByFundingDestination(ctx context.Context, fundingDestination *commonpb.PublicKey) (*pool.Pool, error) {
	model, err := dbGetPoolByFundingDestination(ctx, s.pgxPool, fundingDestination)
	if err != nil {
		return nil, err
	}
	return fromPoolModel(model)
}

func (s *store) CreateBet(ctx context.Context, bet *pool.Bet) error {
	err := toBetModel(bet).dbPut(ctx, s.pgxPool)
	if err != nil {
		return err
	}

	return toMemberModel(bet.UserID, bet.PoolID).dbPut(ctx, s.pgxPool)
}

func (s *store) UpdateBetOutcome(ctx context.Context, betId *poolpb.BetId, newOutcome bool, newSignature *commonpb.Signature, newTs time.Time) error {
	return dbUpdateBetOutcome(ctx, s.pgxPool, betId, newOutcome, newSignature, newTs)
}

func (s *store) MarkBetAsPaid(ctx context.Context, betId *poolpb.BetId) error {
	return dbMarkBetAsPaid(ctx, s.pgxPool, betId)
}

func (s *store) GetBetByID(ctx context.Context, betID *poolpb.BetId) (*pool.Bet, error) {
	model, err := dbGetBetByID(ctx, s.pgxPool, betID)
	if err != nil {
		return nil, err
	}
	return fromBetModel(model)
}

func (s *store) GetBetByUser(ctx context.Context, poolID *poolpb.PoolId, userID *commonpb.UserId) (*pool.Bet, error) {
	model, err := dbGetBetByUser(ctx, s.pgxPool, poolID, userID)
	if err != nil {
		return nil, err
	}
	return fromBetModel(model)
}

func (s *store) GetBetsByPool(ctx context.Context, poolID *poolpb.PoolId) ([]*pool.Bet, error) {
	models, err := dbGetBetsByPool(ctx, s.pgxPool, poolID)
	if err != nil {
		return nil, err
	}

	res := make([]*pool.Bet, len(models))
	for i, model := range models {
		res[i], err = fromBetModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *store) GetPagedMembers(ctx context.Context, userID *commonpb.UserId, queryOptions ...database.QueryOption) ([]*pool.Member, error) {
	models, err := dbGetPagedMembers(ctx, s.pgxPool, userID, queryOptions...)
	if err != nil {
		return nil, err
	}

	res := make([]*pool.Member, len(models))
	for i, model := range models {
		res[i], err = fromMemberModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *store) reset() {
	_, err := s.pgxPool.Exec(context.Background(), "DELETE FROM "+poolsTableName)
	if err != nil {
		panic(err)
	}

	_, err = s.pgxPool.Exec(context.Background(), "DELETE FROM "+betsTableName)
	if err != nil {
		panic(err)
	}
}
