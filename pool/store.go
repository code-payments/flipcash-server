package pool

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"
)

var (
	ErrPoolNotFound                 = errors.New("pool not found")
	ErrPoolIDExists                 = errors.New("pool id already exists")
	ErrPoolFundingDestinationExists = errors.New("pool funding address already exists")
	ErrPoolResolved                 = errors.New("pool is already resolved")
	ErrBetNotFound                  = errors.New("bet not found")
	ErrBetExists                    = errors.New("bet already exists")
)

type Store interface {
	CreatePool(ctx context.Context, pool *Pool) error

	GetPool(ctx context.Context, poolID *poolpb.PoolId) (*Pool, error)

	ResolvePool(ctx context.Context, poolID *poolpb.PoolId, resolution bool, newSignature *commonpb.Signature) error

	CreateBet(ctx context.Context, bet *Bet) error

	GetBetByUser(ctx context.Context, poolID *poolpb.PoolId, userID *commonpb.UserId) (*Bet, error)

	GetBetsByPool(ctx context.Context, poolID *poolpb.PoolId) ([]*Bet, error)
}
