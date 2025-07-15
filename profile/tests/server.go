package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	profilepb "github.com/code-payments/flipcash-protobuf-api/generated/go/profile/v1"

	"github.com/code-payments/flipcash-server/account"
	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/profile"
	"github.com/code-payments/flipcash-server/protoutil"
	"github.com/code-payments/flipcash-server/social/x"
	"github.com/code-payments/flipcash-server/testutil"
)

func RunServerTests(t *testing.T, accounts account.Store, profiles profile.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, accounts account.Store, profiles profile.Store){
		testServer,
	} {
		tf(t, accounts, profiles)
		teardown()
	}
}

func testServer(t *testing.T, accounts account.Store, profiles profile.Store) {
	ctx := context.Background()
	log := zaptest.NewLogger(t)

	authz := account.NewAuthorizer(log, accounts, auth.NewKeyPairAuthenticator())

	serv := profile.NewServer(log, authz, accounts, profiles, x.NewClient())
	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		profilepb.RegisterProfileServer(s, serv)
	}))

	client := profilepb.NewProfileClient(cc)
	userID := model.MustGenerateUserID()
	keyPair := model.MustGenerateKeyPair()

	t.Run("No User", func(t *testing.T) {
		get, err := client.GetProfile(ctx, &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.Equal(t, profilepb.GetProfileResponse_NOT_FOUND, get.Result)
		require.Nil(t, get.UserProfile)

		req := &profilepb.SetDisplayNameRequest{
			DisplayName: "my name",
		}
		require.NoError(t, keyPair.Auth(req, &req.Auth))
		_, err = client.SetDisplayName(ctx, req)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("Registered user", func(t *testing.T) {
		_, err := accounts.Bind(ctx, userID, keyPair.Proto())
		require.NoError(t, err)
		require.NoError(t, accounts.SetRegistrationFlag(ctx, userID, true))

		// Binding of a user isn't sufficient, a profile must be set!
		get, err := client.GetProfile(ctx, &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.Equal(t, profilepb.GetProfileResponse_NOT_FOUND, get.Result)
		require.Nil(t, get.UserProfile)

		setDisplayName := &profilepb.SetDisplayNameRequest{
			DisplayName: "my name",
		}
		require.NoError(t, keyPair.Auth(setDisplayName, &setDisplayName.Auth))
		setDisplayNameResp, err := client.SetDisplayName(ctx, setDisplayName)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.SetDisplayNameResponse{Result: profilepb.SetDisplayNameResponse_OK}, setDisplayNameResp))

		expected := &profilepb.UserProfile{DisplayName: "my name"}

		get, err = client.GetProfile(ctx, &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, get.UserProfile))

		xProfile := &profilepb.XProfile{
			Id:            "123",
			Username:      "registered_user",
			Name:          "registered name",
			Description:   "description",
			ProfilePicUrl: "url",
			VerifiedType:  profilepb.XProfile_BLUE,
			FollowerCount: 888,
		}
		// todo: Need mock X client to use the RPC
		require.NoError(t, profiles.LinkXAccount(ctx, userID, xProfile, "access_token"))

		expected.SocialProfiles = append(expected.SocialProfiles, &profilepb.SocialProfile{
			Type: &profilepb.SocialProfile_X{
				X: xProfile,
			},
		})
		get, err = client.GetProfile(ctx, &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, get.UserProfile))

		unlink := &profilepb.UnlinkSocialAccountRequest{
			SocialIdentifier: &profilepb.UnlinkSocialAccountRequest_XUserId{
				XUserId: xProfile.Id,
			},
		}
		require.NoError(t, keyPair.Auth(unlink, &unlink.Auth))

		unlinkResp, err := client.UnlinkSocialAccount(ctx, unlink)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.UnlinkSocialAccountResponse{}, unlinkResp))

		expected.SocialProfiles = nil
		get, err = client.GetProfile(ctx, &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, get.UserProfile))
	})

	t.Run("Unregistered user", func(t *testing.T) {
		userID2 := model.MustGenerateUserID()
		keypair2 := model.MustGenerateKeyPair()

		_, err := accounts.Bind(ctx, userID2, keypair2.Proto())
		require.NoError(t, err)
		require.NoError(t, accounts.SetRegistrationFlag(ctx, userID, false))

		setDisplayName := &profilepb.SetDisplayNameRequest{
			DisplayName: "my name",
		}
		require.NoError(t, keypair2.Auth(setDisplayName, &setDisplayName.Auth))
		setDisplayNameResp, err := client.SetDisplayName(ctx, setDisplayName)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.SetDisplayNameResponse{Result: profilepb.SetDisplayNameResponse_DENIED}, setDisplayNameResp))

		linkXAccount := &profilepb.LinkSocialAccountRequest{
			LinkingToken: &profilepb.LinkSocialAccountRequest_LinkingToken{
				Type: &profilepb.LinkSocialAccountRequest_LinkingToken_X{
					X: &profilepb.LinkSocialAccountRequest_LinkingToken_XLinkingToken{
						AccessToken: "access_token",
					},
				},
			},
		}
		require.NoError(t, keypair2.Auth(linkXAccount, &linkXAccount.Auth))
		linkXAccountResp, err := client.LinkSocialAccount(ctx, linkXAccount)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.LinkSocialAccountResponse{Result: profilepb.LinkSocialAccountResponse_DENIED}, linkXAccountResp))

		get, err := client.GetProfile(ctx, &profilepb.GetProfileRequest{
			UserId: userID2,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.GetProfileResponse{Result: profilepb.GetProfileResponse_NOT_FOUND}, get))
	})
}
