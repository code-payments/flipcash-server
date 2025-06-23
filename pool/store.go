package pool

import (
	"context"
	"errors"
	"time"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"
	"github.com/code-payments/flipcash-server/database"
)

var (
	ErrPoolNotFound                 = errors.New("pool not found")
	ErrPoolIDExists                 = errors.New("pool id already exists")
	ErrPoolFundingDestinationExists = errors.New("pool funding address already exists")
	ErrPoolOpen                     = errors.New("pool is open")
	ErrPoolResolved                 = errors.New("pool is already resolved")
	ErrBetNotFound                  = errors.New("bet not found")
	ErrBetExists                    = errors.New("bet already exists")
	ErrMaxBetCountExceeded          = errors.New("max bet count exceeded")
	ErrMemberNotFound               = errors.New("pool member not found")
)

type Store interface {
	// Create pool creates a new betting pool
	CreatePool(ctx context.Context, pool *Pool) error

	// GetPool gets a betting pool by ID
	GetPoolByID(ctx context.Context, poolID *poolpb.PoolId) (*Pool, error)

	// ClosePool closes a pool
	ClosePool(ctx context.Context, poolID *poolpb.PoolId, closedAt time.Time, newSignature *commonpb.Signature) error

	// ResolvePool resolves a pool with an outcome
	ResolvePool(ctx context.Context, poolID *poolpb.PoolId, resolution Resolution, newSignature *commonpb.Signature) error

	// CreateBet creates a new bet
	CreateBet(ctx context.Context, bet *Bet) error

	// GetBetByUser gets a bet for a pool made by a user
	GetBetByUser(ctx context.Context, poolID *poolpb.PoolId, userID *commonpb.UserId) (*Bet, error)

	// GetBetsByPool gets all bets for a given pool
	GetBetsByPool(ctx context.Context, poolID *poolpb.PoolId) ([]*Bet, error)

	// GetPagedMembers gets the set of pool memberships for the provided user
	// over a paged API
	GetPagedMembers(ctx context.Context, userID *commonpb.UserId, options ...database.QueryOption) ([]*Member, error)
}
