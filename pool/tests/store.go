package tests

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/pool"
	"github.com/code-payments/flipcash-server/protoutil"
)

func RunStoreTests(t *testing.T, s pool.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s pool.Store){
		testPoolStore_PoolHappyPath,
		testPoolStore_BetHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testPoolStore_PoolHappyPath(t *testing.T, s pool.Store) {
	ctx := context.Background()

	rendezvousKey := model.MustGenerateKeyPair()
	poolID := pool.ToPoolID(rendezvousKey)
	creatorID := model.MustGenerateUserID()

	_, err := s.GetPool(ctx, poolID)
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
		CreatedAt:          time.Now(),
		Signature:          &commonpb.Signature{Value: make([]byte, 64)},
	}
	rand.Read(expected.Signature.Value[:])
	require.NoError(t, s.CreatePool(ctx, expected))

	actual, err := s.GetPool(ctx, poolID)
	require.NoError(t, err)
	assertEquivalentPools(t, expected, actual)

	newSignature := &commonpb.Signature{Value: make([]byte, 64)}
	rand.Read(expected.Signature.Value[:])
	require.NoError(t, s.ResolvePool(ctx, poolID, true, newSignature))

	actual, err = s.GetPool(ctx, poolID)
	require.NoError(t, err)
	require.False(t, actual.IsOpen)
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

	rendezvousKey := model.MustGenerateKeyPair()
	poolID := pool.ToPoolID(rendezvousKey)

	_, err := s.GetBetsByPool(ctx, poolID)
	require.Equal(t, pool.ErrBetNotFound, err)

	var allExpected []*pool.Bet
	for i := 0; i < 2*pool.MaxParticipants; i++ {
		intentID := model.MustGenerateKeyPair()
		betID := pool.ToBetID(intentID)
		userID := model.MustGenerateUserID()

		_, err = s.GetBetByUser(ctx, poolID, userID)
		require.Equal(t, pool.ErrBetNotFound, err)

		expected := &pool.Bet{
			PoolID:            poolID,
			ID:                betID,
			UserID:            userID,
			SelectedOutcome:   true,
			PayoutDestination: model.MustGenerateKeyPair().Proto(),
			Ts:                time.Now(),
			Signature:         &commonpb.Signature{Value: make([]byte, 64)},
		}
		rand.Read(expected.Signature.Value[:])

		err = s.CreateBet(ctx, expected)
		if i >= pool.MaxParticipants {
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

func assertEquivalentPools(t *testing.T, obj1, obj2 *pool.Pool) {
	require.NoError(t, protoutil.ProtoEqualError(obj1.ID, obj2.ID))
	require.NoError(t, protoutil.ProtoEqualError(obj1.CreatorID, obj2.CreatorID))
	require.Equal(t, obj1.Name, obj2.Name)
	require.Equal(t, obj1.BuyInCurrency, obj2.BuyInCurrency)
	require.Equal(t, obj1.BuyInAmount, obj2.BuyInAmount)
	require.NoError(t, protoutil.ProtoEqualError(obj1.FundingDestination, obj2.FundingDestination))
	require.Equal(t, obj1.IsOpen, obj2.IsOpen)
	require.EqualValues(t, obj1.Resolution, obj2.Resolution)
	require.Equal(t, obj1.CreatedAt, obj2.CreatedAt)
	require.NoError(t, protoutil.ProtoEqualError(obj1.Signature, obj2.Signature))
}

func assertEquivalentBets(t *testing.T, obj1, obj2 *pool.Bet) {
	require.NoError(t, protoutil.ProtoEqualError(obj1.PoolID, obj2.PoolID))
	require.NoError(t, protoutil.ProtoEqualError(obj1.ID, obj2.ID))
	require.NoError(t, protoutil.ProtoEqualError(obj1.UserID, obj2.UserID))
	require.Equal(t, obj1.SelectedOutcome, obj2.SelectedOutcome)
	require.NoError(t, protoutil.ProtoEqualError(obj1.PayoutDestination, obj2.PayoutDestination))
	require.Equal(t, obj1.Ts, obj2.Ts)
	require.NoError(t, protoutil.ProtoEqualError(obj1.Signature, obj2.Signature))
}
