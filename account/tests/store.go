package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipcash-server/account"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/protoutil"
)

func RunStoreTests(t *testing.T, s account.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s account.Store){
		testStore_keyManagement,
		testStore_registrationStatus,
	} {
		tf(t, s)
		teardown()
	}
}

func testStore_keyManagement(t *testing.T, s account.Store) {
	ctx := context.Background()

	user := model.MustGenerateUserID()
	keyPairs := make([]*commonpb.PublicKey, 100)
	for i := range keyPairs {
		keyPairs[i] = model.MustGenerateKeyPair().Proto()

		_, err := s.GetUserId(ctx, keyPairs[i])
		require.ErrorIs(t, err, account.ErrNotFound)

		authorized, err := s.IsAuthorized(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.False(t, authorized)

		actual, err := s.Bind(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))

		actual, err = s.GetUserId(ctx, keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))

		actual, err = s.Bind(ctx, model.MustGenerateUserID(), keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))

		authorized, err = s.IsAuthorized(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.True(t, authorized)

		authorized, err = s.IsAuthorized(ctx, model.MustGenerateUserID(), keyPairs[i])
		require.NoError(t, err)
		require.False(t, authorized)
	}

	actual, err := s.GetPubKeys(ctx, user)
	require.NoError(t, err)
	require.NoError(t, protoutil.SetEqualError(actual, keyPairs))

	t.Logf("testRoundTrip: %d key pairs", len(keyPairs))
}

func testStore_registrationStatus(t *testing.T, s account.Store) {
	ctx := context.Background()

	user := model.MustGenerateUserID()

	isRegistered, err := s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.False(t, isRegistered)

	require.Equal(t, account.ErrNotFound, s.SetRegistrationFlag(ctx, user, true))

	user, err = s.Bind(ctx, user, model.MustGenerateKeyPair().Proto())
	require.NoError(t, err)

	isRegistered, err = s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.False(t, isRegistered)

	require.NoError(t, s.SetRegistrationFlag(ctx, user, true))

	isRegistered, err = s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.True(t, isRegistered)

	require.NoError(t, s.SetRegistrationFlag(ctx, user, false))

	isRegistered, err = s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.False(t, isRegistered)
}
