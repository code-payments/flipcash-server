package tests

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	"github.com/code-payments/flipcash-server/database"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/pool"
	"github.com/code-payments/flipcash-server/protoutil"
)

func RunStoreTests(t *testing.T, s pool.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s pool.Store){
		testPoolStore_PoolHappyPath,
		testPoolStore_BetHappyPath,
		testPoolStore_MemberHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testPoolStore_PoolHappyPath(t *testing.T, s pool.Store) {
	ctx := context.Background()

	poolID := pool.ToPoolID(model.MustGenerateKeyPair())
	creatorID := model.MustGenerateUserID()

	_, err := s.GetPoolByID(ctx, poolID)
	require.Equal(t, pool.ErrPoolNotFound, err)

	expected := &pool.Pool{
		ID:                 poolID,
		CreatorID:          creatorID,
		Name:               "Will Flipcash go viral tomorrow?",
		BuyInCurrency:      "usd",
		BuyInAmount:        250.00,
		FundingDestination: model.MustGenerateKeyPair().Proto(),
		IsOpen:             true,
		Resolution:         nil,
		CreatedAt:          time.Now().UTC().Truncate(time.Second),
		Signature:          &commonpb.Signature{Value: make([]byte, 64)},
	}
	rand.Read(expected.Signature.Value[:])
	require.NoError(t, s.CreatePool(ctx, expected))

	actual, err := s.GetPoolByID(ctx, poolID)
	require.NoError(t, err)
	assertEquivalentPools(t, expected, actual)

	newSignature := &commonpb.Signature{Value: make([]byte, 64)}
	rand.Read(expected.Signature.Value[:])
	require.Error(t, pool.ErrPoolOpen, s.ResolvePool(ctx, poolID, true, newSignature))

	closedAt := time.Now().UTC().Truncate(time.Second)
	newSignature = &commonpb.Signature{Value: make([]byte, 64)}
	rand.Read(expected.Signature.Value[:])
	require.Error(t, pool.ErrPoolOpen, s.ClosePool(ctx, poolID, closedAt, newSignature))

	actual, err = s.GetPoolByID(ctx, poolID)
	require.NoError(t, err)
	require.False(t, actual.IsOpen)
	require.Equal(t, closedAt, *actual.ClosedAt)
	require.NoError(t, protoutil.ProtoEqualError(newSignature, actual.Signature))

	newSignature = &commonpb.Signature{Value: make([]byte, 64)}
	rand.Read(expected.Signature.Value[:])
	require.Error(t, pool.ErrPoolOpen, s.ResolvePool(ctx, poolID, true, newSignature))

	actual, err = s.GetPoolByID(ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, true, *actual.Resolution)
	require.NoError(t, protoutil.ProtoEqualError(newSignature, actual.Signature))

	require.Equal(t, pool.ErrPoolIDExists, s.CreatePool(ctx, expected))

	cloned := expected.Clone()
	cloned.ID = pool.ToPoolID(model.MustGenerateKeyPair())
	require.Equal(t, pool.ErrPoolFundingDestinationExists, s.CreatePool(ctx, cloned))

	cloned = expected.Clone()
	cloned.FundingDestination = model.MustGenerateKeyPair().Proto()
	require.Equal(t, pool.ErrPoolIDExists, s.CreatePool(ctx, cloned))
}

func testPoolStore_BetHappyPath(t *testing.T, s pool.Store) {
	ctx := context.Background()

	poolID := pool.ToPoolID(model.MustGenerateKeyPair())

	_, err := s.GetBetsByPool(ctx, poolID)
	require.Equal(t, pool.ErrBetNotFound, err)

	var allExpected []*pool.Bet
	for i := 0; i < 2*pool.MaxParticipants; i++ {
		betID := pool.ToBetID(model.MustGenerateKeyPair())
		userID := model.MustGenerateUserID()

		_, err = s.GetBetByUser(ctx, poolID, userID)
		require.Equal(t, pool.ErrBetNotFound, err)

		expected := &pool.Bet{
			PoolID:            poolID,
			ID:                betID,
			UserID:            userID,
			SelectedOutcome:   true,
			PayoutDestination: model.MustGenerateKeyPair().Proto(),
			Ts:                time.Now().UTC().Truncate(time.Second),
			Signature:         &commonpb.Signature{Value: make([]byte, 64)},
		}
		rand.Read(expected.Signature.Value[:])

		err = s.CreateBet(ctx, expected)
		if i > pool.MaxParticipants {
			require.Equal(t, pool.ErrMaxBetCountExceeded, err)

			_, err = s.GetBetByUser(ctx, poolID, userID)
			require.Equal(t, pool.ErrBetNotFound, err)

			continue
		}
		require.NoError(t, err)

		allExpected = append(allExpected, expected)

		actual, err := s.GetBetByUser(ctx, poolID, userID)
		require.NoError(t, err)
		assertEquivalentBets(t, expected, actual)

		if i >= pool.MaxParticipants {
			continue
		}

		require.Equal(t, pool.ErrBetExists, s.CreateBet(ctx, expected))

		cloned := expected.Clone()
		cloned.ID = pool.ToBetID(model.MustGenerateKeyPair())
		require.Equal(t, pool.ErrBetExists, s.CreateBet(ctx, cloned))

		cloned = expected.Clone()
		cloned.UserID = model.MustGenerateUserID()
		require.Equal(t, pool.ErrBetExists, s.CreateBet(ctx, cloned))
	}

	allActual, err := s.GetBetsByPool(ctx, poolID)
	require.NoError(t, err)
	require.Len(t, allActual, len(allExpected))
	for i, expected := range allExpected {
		assertEquivalentBets(t, expected, allActual[i])
	}
}

func testPoolStore_MemberHappyPath(t *testing.T, s pool.Store) {
	ctx := context.Background()

	userID := model.MustGenerateUserID()

	_, err := s.GetPagedMembers(ctx, userID)
	require.Equal(t, pool.ErrMemberNotFound, err)

	var expectedPoolIDs []*poolpb.PoolId
	for range 10 {
		createdPoolID := pool.ToPoolID(model.MustGenerateKeyPair())
		expectedPoolIDs = append(expectedPoolIDs, createdPoolID)
		p := &pool.Pool{
			ID:                 createdPoolID,
			CreatorID:          userID,
			Name:               "Will Flipcash go viral tomorrow?",
			BuyInCurrency:      "usd",
			BuyInAmount:        250.00,
			FundingDestination: model.MustGenerateKeyPair().Proto(),
			IsOpen:             true,
			Resolution:         nil,
			CreatedAt:          time.Now().UTC().Truncate(time.Second),
			Signature:          &commonpb.Signature{Value: make([]byte, 64)},
		}
		rand.Read(p.Signature.Value[:])
		require.NoError(t, s.CreatePool(ctx, p))

		betPoolID := pool.ToPoolID(model.MustGenerateKeyPair())
		expectedPoolIDs = append(expectedPoolIDs, betPoolID)
		b := &pool.Bet{
			PoolID:            betPoolID,
			ID:                pool.ToBetID(model.MustGenerateKeyPair()),
			UserID:            userID,
			SelectedOutcome:   true,
			PayoutDestination: model.MustGenerateKeyPair().Proto(),
			Ts:                time.Now().UTC().Truncate(time.Second),
			Signature:         &commonpb.Signature{Value: make([]byte, 64)},
		}
		rand.Read(b.Signature.Value[:])
		require.NoError(t, s.CreateBet(ctx, b))
	}

	allActual, err := s.GetPagedMembers(ctx, userID)
	require.NoError(t, err)
	require.Len(t, allActual, len(expectedPoolIDs))
	for i, expectedPoolID := range expectedPoolIDs {
		require.NoError(t, protoutil.ProtoEqualError(expectedPoolID, allActual[i].PoolID))
		require.NoError(t, protoutil.ProtoEqualError(userID, allActual[i].UserID))
	}

	reversedActual, err := s.GetPagedMembers(ctx, userID, database.WithDescending())
	require.NoError(t, err)
	require.Len(t, allActual, len(expectedPoolIDs))
	for i, expectedPoolID := range expectedPoolIDs {
		require.NoError(t, protoutil.ProtoEqualError(expectedPoolID, reversedActual[len(allActual)-i-1].PoolID))
		require.NoError(t, protoutil.ProtoEqualError(userID, reversedActual[len(allActual)-i-1].UserID))
	}

	limitedActual, err := s.GetPagedMembers(ctx, userID, database.WithLimit(3))
	require.NoError(t, err)
	require.Len(t, limitedActual, 3)
	for i, expectedPoolID := range expectedPoolIDs[:3] {
		require.NoError(t, protoutil.ProtoEqualError(expectedPoolID, limitedActual[i].PoolID))
		require.NoError(t, protoutil.ProtoEqualError(userID, limitedActual[i].UserID))
	}

	pagedActual, err := s.GetPagedMembers(ctx, userID, database.WithPagingToken(&commonpb.PagingToken{Value: allActual[5].ID}))
	require.NoError(t, err)
	require.Len(t, pagedActual, len(expectedPoolIDs[6:]))
	for i, expectedPoolID := range expectedPoolIDs[6:] {
		require.NoError(t, protoutil.ProtoEqualError(expectedPoolID, pagedActual[i].PoolID))
		require.NoError(t, protoutil.ProtoEqualError(userID, pagedActual[i].UserID))
	}

	pagedActual, err = s.GetPagedMembers(
		ctx,
		userID,
		database.WithPagingToken(&commonpb.PagingToken{Value: allActual[5].ID}),
		database.WithLimit(2),
		database.WithDescending(),
	)
	require.NoError(t, err)
	require.Len(t, pagedActual, 2)
	for i, expectedPoolID := range expectedPoolIDs[3:5] {
		require.NoError(t, protoutil.ProtoEqualError(expectedPoolID, pagedActual[len(pagedActual)-i-1].PoolID))
		require.NoError(t, protoutil.ProtoEqualError(userID, pagedActual[len(pagedActual)-i-1].UserID))
	}
}

func assertEquivalentPools(t *testing.T, obj1, obj2 *pool.Pool) {
	require.NoError(t, protoutil.ProtoEqualError(obj1.ID, obj2.ID))
	require.NoError(t, protoutil.ProtoEqualError(obj1.CreatorID, obj2.CreatorID))
	require.Equal(t, obj1.Name, obj2.Name)
	require.Equal(t, obj1.BuyInCurrency, obj2.BuyInCurrency)
	require.Equal(t, obj1.BuyInAmount, obj2.BuyInAmount)
	require.NoError(t, protoutil.ProtoEqualError(obj1.FundingDestination, obj2.FundingDestination))
	require.Equal(t, obj1.IsOpen, obj2.IsOpen)
	require.EqualValues(t, obj1.Resolution, obj2.Resolution)
	require.Equal(t, obj1.CreatedAt.UTC(), obj2.CreatedAt.UTC())
	require.NoError(t, protoutil.ProtoEqualError(obj1.Signature, obj2.Signature))
}

func assertEquivalentBets(t *testing.T, obj1, obj2 *pool.Bet) {
	require.NoError(t, protoutil.ProtoEqualError(obj1.PoolID, obj2.PoolID))
	require.NoError(t, protoutil.ProtoEqualError(obj1.ID, obj2.ID))
	require.NoError(t, protoutil.ProtoEqualError(obj1.UserID, obj2.UserID))
	require.Equal(t, obj1.SelectedOutcome, obj2.SelectedOutcome)
	require.NoError(t, protoutil.ProtoEqualError(obj1.PayoutDestination, obj2.PayoutDestination))
	require.Equal(t, obj1.Ts.UTC(), obj2.Ts.UTC())
	require.NoError(t, protoutil.ProtoEqualError(obj1.Signature, obj2.Signature))
}
