package memory

import (
	"bytes"
	"context"
	"sync"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"
	"github.com/code-payments/flipcash-server/pool"
)

type InMemoryStore struct {
	mu    sync.RWMutex
	pools []*pool.Pool
	bets  []*pool.Bet
}

func NewInMemory() pool.Store {
	return &InMemoryStore{}
}

func (s *InMemoryStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pools = nil
	s.bets = nil
}

func (s *InMemoryStore) CreatePool(_ context.Context, newPool *pool.Pool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := s.findPoolByID(newPool.ID)
	if existing != nil {
		return pool.ErrPoolIDExists
	}

	existing = s.findPoolByFundingDestination(newPool.FundingDestination)
	if existing != nil {
		return pool.ErrPoolFundingDestinationExists
	}

	s.pools = append(s.pools, newPool.Clone())

	return nil
}

func (s *InMemoryStore) GetPool(_ context.Context, poolID *poolpb.PoolId) (*pool.Pool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := s.findPoolByID(poolID)

	if res == nil {
		return nil, pool.ErrPoolNotFound
	}
	return res.Clone(), nil
}

func (s *InMemoryStore) ResolvePool(_ context.Context, poolID *poolpb.PoolId, resolution bool, newSignature *commonpb.Signature) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findPoolByID(poolID)
	if item == nil {
		return pool.ErrPoolNotFound
	}
	if item.Resolution != nil {
		return pool.ErrPoolResolved
	}

	item.IsOpen = false
	item.Resolution = &resolution
	item.Signature = newSignature

	return nil
}

func (s *InMemoryStore) CreateBet(_ context.Context, newBet *pool.Bet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := s.findBetByID(newBet.ID)
	if existing != nil {
		return pool.ErrBetExists
	}

	existing = s.findBetByPoolAndUser(newBet.PoolID, newBet.UserID)
	if existing != nil {
		return pool.ErrBetExists
	}

	if len(s.findBetsByPool(newBet.PoolID)) >= pool.MaxParticipants {
		return pool.ErrMaxBetCountExceeded
	}

	s.bets = append(s.bets, newBet.Clone())

	return nil
}

func (s *InMemoryStore) GetBetByUser(_ context.Context, poolID *poolpb.PoolId, userID *commonpb.UserId) (*pool.Bet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := s.findBetByPoolAndUser(poolID, userID)

	if res == nil {
		return nil, pool.ErrBetNotFound
	}
	return res.Clone(), nil
}

func (s *InMemoryStore) GetBetsByPool(_ context.Context, poolID *poolpb.PoolId) ([]*pool.Bet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := s.findBetsByPool(poolID)

	if len(res) == 0 {
		return nil, pool.ErrBetNotFound
	}
	return pool.CloneBets(res), nil
}

func (s *InMemoryStore) findPoolByID(poolID *poolpb.PoolId) *pool.Pool {
	for _, pool := range s.pools {
		if bytes.Equal(pool.ID.Value, poolID.Value) {
			return pool
		}
	}
	return nil
}

func (s *InMemoryStore) findPoolByFundingDestination(fundingDestination *commonpb.PublicKey) *pool.Pool {
	for _, pool := range s.pools {
		if bytes.Equal(pool.FundingDestination.Value, fundingDestination.Value) {
			return pool
		}
	}
	return nil
}

func (s *InMemoryStore) findBetByID(betID *poolpb.BetId) *pool.Bet {
	for _, bet := range s.bets {
		if bytes.Equal(bet.ID.Value, betID.Value) {
			return bet
		}
	}
	return nil
}

func (s *InMemoryStore) findBetByPoolAndUser(poolID *poolpb.PoolId, userID *commonpb.UserId) *pool.Bet {
	for _, bet := range s.bets {
		if bytes.Equal(bet.PoolID.Value, poolID.Value) && bytes.Equal(bet.UserID.Value, userID.Value) {
			return bet
		}
	}
	return nil
}

func (s *InMemoryStore) findBetsByPool(poolID *poolpb.PoolId) []*pool.Bet {
	var res []*pool.Bet
	for _, bet := range s.bets {
		if bytes.Equal(bet.PoolID.Value, poolID.Value) {
			res = append(res, bet)
		}
	}
	return res
}
