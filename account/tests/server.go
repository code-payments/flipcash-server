package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountpb "github.com/code-payments/flipcash-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codetestutil "github.com/code-payments/code-server/pkg/testutil"

	"github.com/code-payments/flipcash-server/account"
	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/protoutil"
	"github.com/code-payments/flipcash-server/testutil"
)

func RunServerTests(t *testing.T, s account.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s account.Store){
		testServer,
	} {
		tf(t, s)
		teardown()
	}
}

func testServer(t *testing.T, store account.Store) {
	log, err := zap.NewDevelopment()
	require.NoError(t, err)

	codeStores := codedata.NewTestDataProvider()

	server := account.NewServer(
		log,
		store,
		auth.NewKeyPairAuthenticator(),
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		accountpb.RegisterAccountServer(s, server)
	}))

	ctx := context.Background()
	client := accountpb.NewAccountClient(cc)

	codetestutil.SetupRandomSubsidizer(t, codeStores)

	var keys []model.KeyPair
	var userId *commonpb.UserId

	t.Run("Register", func(t *testing.T) {
		keys = append(keys, model.MustGenerateKeyPair())
		req := &accountpb.RegisterRequest{
			PublicKey: keys[0].Proto(),
		}
		require.NoError(t, keys[0].Sign(req, &req.Signature))

		for range 2 {
			resp, err := client.Register(ctx, req)
			require.NoError(t, err)
			require.Equal(t, accountpb.RegisterResponse_OK, resp.Result)
			require.NotNil(t, resp.UserId)

			if userId == nil {
				userId = resp.UserId
			} else {
				require.NoError(t, protoutil.ProtoEqualError(userId, resp.UserId))
			}
		}
	})

	t.Run("Login", func(t *testing.T) {
		for _, key := range keys {
			req := &accountpb.LoginRequest{
				Timestamp: timestamppb.Now(),
			}
			require.NoError(t, key.Auth(req, &req.Auth))

			resp, err := client.Login(ctx, req)
			require.NoError(t, err)
			require.Equal(t, accountpb.LoginResponse_OK, resp.Result)
			require.NotNil(t, resp.UserId)
			require.NoError(t, protoutil.ProtoEqualError(userId, resp.UserId))
		}
	})
}
