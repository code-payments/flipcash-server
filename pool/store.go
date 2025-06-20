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
	ErrMaxBetCountExceeded          = errors.New("max bet count exceeded")
)

type Store interface {
	// Create pool creates a new betting pool
	CreatePool(ctx context.Context, pool *Pool) error

	// GetPool gets a betting pool by ID
	GetPoolByID(ctx context.Context, poolID *poolpb.PoolId) (*Pool, error)

	// ResolvePool resolves a pool with an outcome. If the pool is open, its state
	// is also closed.
	ResolvePool(ctx context.Context, poolID *poolpb.PoolId, resolution bool, newSignature *commonpb.Signature) error

	// CreateBet creates a new bet
	CreateBet(ctx context.Context, bet *Bet) error

	// GetBetByUser gets a bet for a pool made by a user
	GetBetByUser(ctx context.Context, poolID *poolpb.PoolId, userID *commonpb.UserId) (*Bet, error)

	// GetBetsByPool gets all bets for a given pool
	GetBetsByPool(ctx context.Context, poolID *poolpb.PoolId) ([]*Bet, error)
}
