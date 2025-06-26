package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codetestutil "github.com/code-payments/code-server/pkg/testutil"
	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/pool"
	"github.com/code-payments/flipcash-server/protoutil"
)

// todo: Add tests around more edge case result codes and flows
// todo: Add tests around signature verification
// todo: Add tests around verified bet payments (when implemented)
// todo: Add more test for paging APIs, but those are well covered in store tests

func RunServerTests(t *testing.T, s pool.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s pool.Store){
		testServer_PoolManagement_HappyPath,
		testServer_Betting_HappyPath,
		testServer_Membership_HappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testServer_PoolManagement_HappyPath(t *testing.T, store pool.Store) {
	ctx := context.Background()
	log := zaptest.NewLogger(t)

	authz := auth.NewStaticAuthorizer()
	server := pool.NewServer(log, authz, store)
	codetestutil.SetupRandomSubsidizer(t, codedata.NewTestDataProvider())

	rendezvousKey := model.MustGenerateKeyPair()
	poolID := pool.ToPoolID(rendezvousKey)
	expected := generateNewProtoPool(poolID)

	creatorKey := model.MustGenerateKeyPair()
	authz.Add(expected.Creator, creatorKey)

	getReq := &poolpb.GetPoolRequest{
		Id: poolID,
	}
	getResp, err := server.GetPool(ctx, getReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPoolResponse_NOT_FOUND, getResp.Result)

	createReq := &poolpb.CreatePoolRequest{
		Pool: expected,
	}
	require.NoError(t, rendezvousKey.Sign(expected, &createReq.RendezvousSignature))
	require.NoError(t, creatorKey.Auth(createReq, &createReq.Auth))

	createResp, err := server.CreatePool(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.CreatePoolResponse_OK, createResp.Result)

	getResp, err = server.GetPool(ctx, getReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPoolResponse_OK, getResp.Result)
	require.NoError(t, protoutil.ProtoEqualError(expected, getResp.Pool.VerifiedMetadata))
	require.NoError(t, protoutil.ProtoEqualError(createReq.RendezvousSignature, getResp.Pool.RendezvousSignature))
	require.Empty(t, getResp.Pool.Bets)

	expected.IsOpen = false
	expected.ClosedAt = &timestamppb.Timestamp{Seconds: time.Now().Unix()}
	closeReq := &poolpb.ClosePoolRequest{
		Id:       poolID,
		ClosedAt: expected.ClosedAt,
	}
	require.NoError(t, rendezvousKey.Sign(expected, &closeReq.NewRendezvousSignature))
	require.NoError(t, creatorKey.Auth(closeReq, &closeReq.Auth))

	closeResp, err := server.ClosePool(ctx, closeReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.ClosePoolResponse_OK, closeResp.Result)

	getResp, err = server.GetPool(ctx, getReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPoolResponse_OK, getResp.Result)
	require.NoError(t, protoutil.ProtoEqualError(expected, getResp.Pool.VerifiedMetadata))
	require.NoError(t, protoutil.ProtoEqualError(closeReq.NewRendezvousSignature, getResp.Pool.RendezvousSignature))
	require.Empty(t, getResp.Pool.Bets)

	expected.Resolution = &poolpb.Resolution{Kind: &poolpb.Resolution_BooleanResolution{
		BooleanResolution: true,
	}}
	resolveReq := &poolpb.ResolvePoolRequest{
		Id:         poolID,
		Resolution: expected.Resolution,
	}
	require.NoError(t, rendezvousKey.Sign(expected, &resolveReq.NewRendezvousSignature))
	require.NoError(t, creatorKey.Auth(resolveReq, &resolveReq.Auth))

	resolveResp, err := server.ResolvePool(ctx, resolveReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.ResolvePoolResponse_OK, resolveResp.Result)

	getResp, err = server.GetPool(ctx, getReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPoolResponse_OK, getResp.Result)
	require.NoError(t, protoutil.ProtoEqualError(expected, getResp.Pool.VerifiedMetadata))
	require.NoError(t, protoutil.ProtoEqualError(resolveReq.NewRendezvousSignature, getResp.Pool.RendezvousSignature))
	require.Empty(t, getResp.Pool.Bets)
}

func testServer_Betting_HappyPath(t *testing.T, store pool.Store) {
	ctx := context.Background()
	log := zaptest.NewLogger(t)

	authz := auth.NewStaticAuthorizer()
	server := pool.NewServer(log, authz, store)
	codetestutil.SetupRandomSubsidizer(t, codedata.NewTestDataProvider())

	rendezvousKey := model.MustGenerateKeyPair()
	poolID := pool.ToPoolID(rendezvousKey)
	protoPool := generateNewProtoPool(poolID)

	creatorKey := model.MustGenerateKeyPair()
	authz.Add(protoPool.Creator, creatorKey)

	var allBets []*poolpb.SignedBetMetadata
	var betterKeys []model.KeyPair
	for i := range 2 * pool.MaxParticipants {
		bet := generateNewProtoBet(i%3 == 0)
		allBets = append(allBets, bet)
		betterKey := model.MustGenerateKeyPair()
		authz.Add(bet.UserId, betterKey)
		betterKeys = append(betterKeys, betterKey)
	}

	createPoolReq := &poolpb.CreatePoolRequest{
		Pool: protoPool,
	}
	require.NoError(t, rendezvousKey.Sign(protoPool, &createPoolReq.RendezvousSignature))
	require.NoError(t, creatorKey.Auth(createPoolReq, &createPoolReq.Auth))

	createPoolResp, err := server.CreatePool(ctx, createPoolReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.CreatePoolResponse_OK, createPoolResp.Result)

	var expectedBets []*poolpb.SignedBetMetadata
	var expectedBetSignatures []*commonpb.Signature
	for i, bet := range allBets {
		makeBetReq := &poolpb.MakeBetRequest{
			PoolId: poolID,
			Bet:    bet,
		}
		require.NoError(t, rendezvousKey.Sign(bet, &makeBetReq.RendezvousSignature))
		require.NoError(t, betterKeys[i].Auth(makeBetReq, &makeBetReq.Auth))

		makeBetResp, err := server.MakeBet(ctx, makeBetReq)
		require.NoError(t, err)
		if i >= pool.MaxParticipants {
			require.Equal(t, poolpb.MakeBetResponse_MAX_BETS_RECEIVED, makeBetResp.Result)
			continue
		}
		require.Equal(t, poolpb.MakeBetResponse_OK, makeBetResp.Result)

		expectedBets = append(expectedBets, bet)
		expectedBetSignatures = append(expectedBetSignatures, makeBetReq.RendezvousSignature)
	}

	getReq := &poolpb.GetPoolRequest{
		Id: poolID,
	}
	getPoolResp, err := server.GetPool(ctx, getReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPoolResponse_OK, getPoolResp.Result)
	require.Len(t, getPoolResp.Pool.Bets, len(expectedBets))
	for i, actual := range getPoolResp.Pool.Bets {
		require.NoError(t, protoutil.ProtoEqualError(expectedBets[i], actual.VerifiedMetadata))
		require.NoError(t, protoutil.ProtoEqualError(expectedBetSignatures[i], actual.RendezvousSignature))
	}
	require.EqualValues(t, getPoolResp.Pool.BetSummary.GetBooleanSummary().NumYes, 34)
	require.EqualValues(t, getPoolResp.Pool.BetSummary.GetBooleanSummary().NumNo, 66)

	protoPool.IsOpen = false
	protoPool.ClosedAt = &timestamppb.Timestamp{Seconds: time.Now().Unix()}
	closeReq := &poolpb.ClosePoolRequest{
		Id:       poolID,
		ClosedAt: protoPool.ClosedAt,
	}
	require.NoError(t, rendezvousKey.Sign(protoPool, &closeReq.NewRendezvousSignature))
	require.NoError(t, creatorKey.Auth(closeReq, &closeReq.Auth))

	closeResp, err := server.ClosePool(ctx, closeReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.ClosePoolResponse_OK, closeResp.Result)

	makeBetReq := &poolpb.MakeBetRequest{
		PoolId: poolID,
		Bet:    generateNewProtoBet(true),
	}
	betterKey := model.MustGenerateKeyPair()
	authz.Add(makeBetReq.Bet.UserId, betterKey)
	require.NoError(t, rendezvousKey.Sign(makeBetReq.Bet, &makeBetReq.RendezvousSignature))
	require.NoError(t, betterKey.Auth(makeBetReq, &makeBetReq.Auth))

	makeBetResp, err := server.MakeBet(ctx, makeBetReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.MakeBetResponse_POOL_CLOSED, makeBetResp.Result)

	getPoolResp, err = server.GetPool(ctx, getReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPoolResponse_OK, getPoolResp.Result)
	require.Len(t, getPoolResp.Pool.Bets, len(expectedBets))
}

func testServer_Membership_HappyPath(t *testing.T, store pool.Store) {
	ctx := context.Background()
	log := zaptest.NewLogger(t)

	authz := auth.NewStaticAuthorizer()
	server := pool.NewServer(log, authz, store)
	codetestutil.SetupRandomSubsidizer(t, codedata.NewTestDataProvider())

	rendezvousKey := model.MustGenerateKeyPair()
	poolID := pool.ToPoolID(rendezvousKey)
	expected := generateNewProtoPool(poolID)

	creatorKey := model.MustGenerateKeyPair()
	authz.Add(expected.Creator, creatorKey)

	getPagedReq := &poolpb.GetPagedPoolsRequest{}
	require.NoError(t, creatorKey.Auth(getPagedReq, &getPagedReq.Auth))

	getPagedResp, err := server.GetPagedPools(ctx, getPagedReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPagedPoolsResponse_NOT_FOUND, getPagedResp.Result)

	createReq := &poolpb.CreatePoolRequest{
		Pool: expected,
	}
	require.NoError(t, rendezvousKey.Sign(expected, &createReq.RendezvousSignature))
	require.NoError(t, creatorKey.Auth(createReq, &createReq.Auth))

	createResp, err := server.CreatePool(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.CreatePoolResponse_OK, createResp.Result)

	getPagedResp, err = server.GetPagedPools(ctx, getPagedReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPagedPoolsResponse_OK, getPagedResp.Result)
	require.Len(t, getPagedResp.Pools, 1)
	require.NoError(t, protoutil.ProtoEqualError(expected, getPagedResp.Pools[0].VerifiedMetadata))
	require.NoError(t, protoutil.ProtoEqualError(createReq.RendezvousSignature, getPagedResp.Pools[0].RendezvousSignature))
	require.NotNil(t, getPagedResp.Pools[0].PagingToken)

	bet := generateNewProtoBet(true)
	betterKey := model.MustGenerateKeyPair()
	authz.Add(bet.UserId, betterKey)

	getPagedReq = &poolpb.GetPagedPoolsRequest{}
	require.NoError(t, betterKey.Auth(getPagedReq, &getPagedReq.Auth))

	getPagedResp, err = server.GetPagedPools(ctx, getPagedReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPagedPoolsResponse_NOT_FOUND, getPagedResp.Result)

	makeBetReq := &poolpb.MakeBetRequest{
		PoolId: poolID,
		Bet:    bet,
	}
	require.NoError(t, rendezvousKey.Sign(bet, &makeBetReq.RendezvousSignature))
	require.NoError(t, betterKey.Auth(makeBetReq, &makeBetReq.Auth))

	makeBetResp, err := server.MakeBet(ctx, makeBetReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.MakeBetResponse_OK, makeBetResp.Result)

	getPagedResp, err = server.GetPagedPools(ctx, getPagedReq)
	require.NoError(t, err)
	require.Equal(t, poolpb.GetPagedPoolsResponse_OK, getPagedResp.Result)
	require.Len(t, getPagedResp.Pools, 1)
	require.NoError(t, protoutil.ProtoEqualError(expected, getPagedResp.Pools[0].VerifiedMetadata))
	require.NoError(t, protoutil.ProtoEqualError(createReq.RendezvousSignature, getPagedResp.Pools[0].RendezvousSignature))
	require.NotNil(t, getPagedResp.Pools[0].PagingToken)
}

func generateNewProtoPool(id *poolpb.PoolId) *poolpb.SignedPoolMetadata {
	return &poolpb.SignedPoolMetadata{
		Id:      id,
		Creator: model.MustGenerateUserID(),
		Name:    "Will Flipcash go viral tomorrow?",
		BuyIn: &commonpb.FiatPaymentAmount{
			Currency:     "usd",
			NativeAmount: 250.00,
		},
		FundingDestination: model.MustGenerateKeyPair().Proto(),
		IsOpen:             true,
		Resolution:         nil,
		CreatedAt:          &timestamppb.Timestamp{Seconds: time.Now().Unix()},
	}
}

func generateNewProtoBet(outcome bool) *poolpb.SignedBetMetadata {
	return &poolpb.SignedBetMetadata{
		BetId:  pool.ToBetID(model.MustGenerateKeyPair()),
		UserId: model.MustGenerateUserID(),
		SelectedOutcome: &poolpb.BetOutcome{
			Kind: &poolpb.BetOutcome_BooleanOutcome{
				BooleanOutcome: outcome,
			},
		},
		PayoutDestination: model.MustGenerateKeyPair().Proto(),
		Ts:                &timestamppb.Timestamp{Seconds: time.Now().Unix()},
	}
}
