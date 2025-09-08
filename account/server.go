package account

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountpb "github.com/code-payments/flipcash-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/database"
	"github.com/code-payments/flipcash-server/model"
)

// todo: env configs
const (
	loginWindow                 = 2 * time.Minute
	requireIapOnAccountCreation = false
)

var (
	defaultOnRampProviders = []accountpb.UserFlags_OnRampProvider{
		accountpb.UserFlags_PHANTOM,
		accountpb.UserFlags_CRYPTO_WALLET,
	}
	onRampProvidersByCountry = map[string]map[commonpb.Platform][]accountpb.UserFlags_OnRampProvider{}

	staffAppleOnRampProviders = []accountpb.UserFlags_OnRampProvider{
		accountpb.UserFlags_COINBASE_VIRTUAL,
		accountpb.UserFlags_PHANTOM,
		accountpb.UserFlags_CRYPTO_WALLET,
	}
	staffGoogleOnRampProviders = []accountpb.UserFlags_OnRampProvider{
		accountpb.UserFlags_PHANTOM,
		accountpb.UserFlags_CRYPTO_WALLET,
	}
)

type Server struct {
	log      *zap.Logger
	store    Store
	verifier auth.Authenticator

	accountpb.UnimplementedAccountServer
}

func NewServer(log *zap.Logger, store Store, verifier auth.Authenticator) *Server {
	return &Server{
		log:      log,
		store:    store,
		verifier: verifier,
	}
}

func (s *Server) Register(ctx context.Context, req *accountpb.RegisterRequest) (*accountpb.RegisterResponse, error) {
	verify := &accountpb.RegisterRequest{
		PublicKey: req.PublicKey,
	}
	err := s.verifier.Verify(ctx, verify, &commonpb.Auth{
		Kind: &commonpb.Auth_KeyPair_{
			KeyPair: &commonpb.Auth_KeyPair{
				PubKey:    req.PublicKey,
				Signature: req.Signature,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	userID, err := model.GenerateUserId()
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate user id")
	}

	var prev *commonpb.UserId
	err = database.ExecuteTxWithinCtx(ctx, func(ctx context.Context) error {
		prev, err = s.store.Bind(ctx, userID, req.PublicKey)
		if err != nil {
			return err
		}

		if !requireIapOnAccountCreation {
			return s.store.SetRegistrationFlag(ctx, prev, true)
		}
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	return &accountpb.RegisterResponse{
		UserId: prev,
	}, nil
}

func (s *Server) Login(ctx context.Context, req *accountpb.LoginRequest) (*accountpb.LoginResponse, error) {
	t := req.Timestamp.AsTime()
	if t.After(time.Now().Add(loginWindow)) {
		return &accountpb.LoginResponse{Result: accountpb.LoginResponse_INVALID_TIMESTAMP}, nil
	} else if t.Before(time.Now().Add(-loginWindow)) {
		return &accountpb.LoginResponse{Result: accountpb.LoginResponse_INVALID_TIMESTAMP}, nil
	}

	a := req.Auth
	req.Auth = nil
	if err := s.verifier.Verify(ctx, req, a); err != nil {
		if status.Code(err) == codes.Unauthenticated {
			return &accountpb.LoginResponse{Result: accountpb.LoginResponse_DENIED}, nil
		}

		return nil, err
	}

	keyPair := a.GetKeyPair()
	if keyPair == nil {
		return nil, status.Error(codes.InvalidArgument, "missing keypair")
	}
	if err := keyPair.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid keypair: %v", err)
	}

	userID, err := s.store.GetUserId(ctx, keyPair.GetPubKey())
	if errors.Is(err, ErrNotFound) {
		return &accountpb.LoginResponse{Result: accountpb.LoginResponse_DENIED}, nil
	} else if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	return &accountpb.LoginResponse{Result: accountpb.LoginResponse_OK, UserId: userID}, nil
}

func (s *Server) GetUserFlags(ctx context.Context, req *accountpb.GetUserFlagsRequest) (*accountpb.GetUserFlagsResponse, error) {
	authorized, err := s.store.GetPubKeys(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get keys")
	}

	if len(authorized) == 0 {
		// Don't leak that the user does not exist.
		return &accountpb.GetUserFlagsResponse{Result: accountpb.GetUserFlagsResponse_DENIED}, nil
	}

	var signerAuthorized bool
	for _, key := range authorized {
		if bytes.Equal(key.Value, req.GetAuth().GetKeyPair().PubKey.Value) {
			signerAuthorized = true
			break
		}
	}

	if !signerAuthorized {
		return &accountpb.GetUserFlagsResponse{Result: accountpb.GetUserFlagsResponse_DENIED}, nil
	}

	isStaff, err := s.store.IsStaff(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get staff flag")
	}

	isRegistered, err := s.store.IsRegistered(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get registration flag")
	}

	var preferredOnRampProviderForUser accountpb.UserFlags_OnRampProvider
	var supportedOnRampProvidersForUser []accountpb.UserFlags_OnRampProvider
	if isStaff {
		switch req.Platform {
		case commonpb.Platform_APPLE:
			supportedOnRampProvidersForUser = staffAppleOnRampProviders
		case commonpb.Platform_GOOGLE:
			supportedOnRampProvidersForUser = staffGoogleOnRampProviders
		}
	} else {
		supportedOnRampProvidersForUser = getSupportedOnRampProviders(req.CountryCode, req.Platform)
	}
	for _, onRampProvider := range supportedOnRampProvidersForUser {
		if onRampProvider == accountpb.UserFlags_COINBASE_VIRTUAL {
			preferredOnRampProviderForUser = accountpb.UserFlags_COINBASE_VIRTUAL
			break
		}
	}

	return &accountpb.GetUserFlagsResponse{
		Result: accountpb.GetUserFlagsResponse_OK,
		UserFlags: &accountpb.UserFlags{
			IsStaff:                    isStaff,
			IsRegisteredAccount:        isRegistered,
			RequiresIapForRegistration: requireIapOnAccountCreation,
			SupportedOnRampProviders:   supportedOnRampProvidersForUser,
			PreferredOnRampProvider:    preferredOnRampProviderForUser,
		},
	}, nil
}

func getSupportedOnRampProviders(countryCode *commonpb.CountryCode, platform commonpb.Platform) []accountpb.UserFlags_OnRampProvider {
	supported := make([]accountpb.UserFlags_OnRampProvider, len(defaultOnRampProviders))
	copy(supported, defaultOnRampProviders)

	if countryCode == nil {
		return supported
	}

	if platform == commonpb.Platform_UNKNOWN {
		return supported
	}

	byCountry, ok := onRampProvidersByCountry[strings.ToLower(countryCode.Value)]
	if !ok {
		return supported
	}

	byPlatform, ok := byCountry[platform]
	if !ok {
		return supported
	}

	return append(supported, byPlatform...)
}
