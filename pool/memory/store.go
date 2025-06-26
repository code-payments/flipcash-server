package memory

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	"github.com/code-payments/flipcash-server/database"
	"github.com/code-payments/flipcash-server/pool"
)

type MembersById []*pool.Member

func (a MembersById) Len() int      { return len(a) }
func (a MembersById) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a MembersById) Less(i, j int) bool {
	id1 := binary.LittleEndian.Uint64(a[i].ID)
	id2 := binary.LittleEndian.Uint64(a[j].ID)
	return id1 < id2
}

type InMemoryStore struct {
	mu sync.RWMutex

	nextMembershipIndex uint64

	pools   []*pool.Pool
	members []*pool.Member
	bets    []*pool.Bet
}

func NewInMemory() pool.Store {
	return &InMemoryStore{}
}

func (s *InMemoryStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextMembershipIndex = 0

	s.pools = nil
	s.members = nil
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
	s.addMemberIfNotFound(newPool.CreatorID, newPool.ID)

	return nil
}

func (s *InMemoryStore) GetPoolByID(_ context.Context, poolID *poolpb.PoolId) (*pool.Pool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := s.findPoolByID(poolID)

	if res == nil {
		return nil, pool.ErrPoolNotFound
	}
	return res.Clone(), nil
}

func (s *InMemoryStore) ClosePool(_ context.Context, poolID *poolpb.PoolId, closedAt time.Time, newSignature *commonpb.Signature) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findPoolByID(poolID)
	if item == nil {
		return pool.ErrPoolNotFound
	}
	if !item.IsOpen {
		return nil
	}

	item.IsOpen = false
	item.ClosedAt = &closedAt
	item.Signature = newSignature

	return nil
}

func (s *InMemoryStore) ResolvePool(_ context.Context, poolID *poolpb.PoolId, resolution pool.Resolution, newSignature *commonpb.Signature) error {
	if resolution == pool.ResolutionUnknown {
		return errors.New("resolution cannot be unknown")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findPoolByID(poolID)
	if item == nil {
		return pool.ErrPoolNotFound
	}
	if item.IsOpen {
		return pool.ErrPoolOpen
	}
	if item.Resolution != pool.ResolutionUnknown {
		return pool.ErrPoolResolved
	}

	item.Resolution = resolution
	item.Signature = newSignature

	return nil
}

func (s *InMemoryStore) CreateBet(_ context.Context, newBet *pool.Bet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.findBetsByPool(newBet.PoolID)) >= pool.MaxParticipants {
		return pool.ErrMaxBetCountExceeded
	}

	existing := s.findBetByID(newBet.ID)
	if existing != nil {
		return pool.ErrBetExists
	}

	existing = s.findBetByPoolAndUser(newBet.PoolID, newBet.UserID)
	if existing != nil {
		return pool.ErrBetExists
	}

	s.bets = append(s.bets, newBet.Clone())
	s.addMemberIfNotFound(newBet.UserID, newBet.PoolID)

	return nil
}

func (s *InMemoryStore) UpdateBetOutcome(_ context.Context, betId *poolpb.BetId, newOutcome bool, newSignature *commonpb.Signature, newTs time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findBetByID(betId)
	if item == nil {
		return pool.ErrBetNotFound
	}

	item.SelectedOutcome = newOutcome
	item.Signature = proto.Clone(newSignature).(*commonpb.Signature)
	item.Ts = newTs

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

func (s *InMemoryStore) GetPagedMembers(ctx context.Context, userID *commonpb.UserId, queryOptions ...database.QueryOption) ([]*pool.Member, error) {
	appliedQueryOptions := database.ApplyQueryOptions(queryOptions...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.findMembersByUserID(userID)

	var cloned []*pool.Member
	for _, member := range all {
		if appliedQueryOptions.PagingToken != nil && appliedQueryOptions.Order == commonpb.QueryOptions_ASC && compareMemberIds(member.ID, appliedQueryOptions.PagingToken.Value) <= 0 {
			continue
		}

		if appliedQueryOptions.PagingToken != nil && appliedQueryOptions.Order == commonpb.QueryOptions_DESC && compareMemberIds(member.ID, appliedQueryOptions.PagingToken.Value) >= 0 {
			continue
		}

		cloned = append(cloned, member.Clone())
	}

	sorted := MembersById(cloned)
	if appliedQueryOptions.Order == commonpb.QueryOptions_DESC {
		sort.Sort(sort.Reverse(sorted))
	}

	limited := sorted
	if len(limited) > appliedQueryOptions.Limit {
		limited = limited[:appliedQueryOptions.Limit]
	}

	if len(limited) == 0 {
		return nil, pool.ErrMemberNotFound
	}
	return limited, nil
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

func (s *InMemoryStore) findMember(userID *commonpb.UserId, poolID *poolpb.PoolId) *pool.Member {
	for _, member := range s.members {
		if !bytes.Equal(member.UserID.Value, userID.Value) {
			continue
		}

		if !bytes.Equal(member.PoolID.Value, poolID.Value) {
			continue
		}

		return member
	}

	return nil
}

func (s *InMemoryStore) findMembersByUserID(userID *commonpb.UserId) []*pool.Member {
	var res []*pool.Member
	for _, member := range s.members {
		if bytes.Equal(member.UserID.Value, userID.Value) {
			res = append(res, member)
		}
	}
	return res
}

func (s *InMemoryStore) addMemberIfNotFound(userID *commonpb.UserId, poolID *poolpb.PoolId) {
	existing := s.findMember(userID, poolID)
	if existing != nil {
		return
	}

	s.nextMembershipIndex++
	member := &pool.Member{
		ID:     make([]byte, 8),
		UserID: proto.Clone(userID).(*commonpb.UserId),
		PoolID: proto.Clone(poolID).(*poolpb.PoolId),
	}
	binary.LittleEndian.PutUint64(member.ID, s.nextMembershipIndex)
	s.members = append(s.members, member)
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

func compareMemberIds(id1, id2 []byte) int {
	int1 := binary.LittleEndian.Uint64(id1)
	int2 := binary.LittleEndian.Uint64(id2)
	if int1 == int2 {
		return 0
	}
	if int1 < int2 {
		return -1
	}
	return 1
}
