package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

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
	return toPoolModel(pool).dbPut(ctx, s.pgxPool)
}

func (s *store) GetPoolByID(ctx context.Context, poolID *poolpb.PoolId) (*pool.Pool, error) {
	model, err := dbGetPoolByID(ctx, s.pgxPool, poolID)
	if err != nil {
		return nil, err
	}
	return fromPoolModel(model)
}

func (s *store) ResolvePool(ctx context.Context, poolID *poolpb.PoolId, resolution bool, newSignature *commonpb.Signature) error {
	return dbResolvePool(ctx, s.pgxPool, poolID, resolution, newSignature)
}

func (s *store) CreateBet(ctx context.Context, bet *pool.Bet) error {
	return toBetModel(bet).dbPut(ctx, s.pgxPool)
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
