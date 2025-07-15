package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipcash-protobuf-api/generated/go/profile/v1"

	"github.com/code-payments/flipcash-server/profile"
)

type store struct {
	pool *pgxpool.Pool
}

func NewInPostgres(pool *pgxpool.Pool) profile.Store {
	return &store{
		pool: pool,
	}
}

func (s *store) GetProfile(ctx context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error) {
	displayName, err := dbGetDisplayName(ctx, s.pool, id)
	if err != nil {
		return nil, err
	} else if displayName == nil {
		return nil, profile.ErrNotFound
	}

	userProfile := &profilepb.UserProfile{
		DisplayName: *displayName,
	}

	xProfileModel, err := dbGetXProfile(ctx, s.pool, id)
	if err == nil {
		xProfile, err := fromXProfileModel(xProfileModel)
		if err != nil {
			return nil, err
		}

		userProfile.SocialProfiles = append(userProfile.SocialProfiles, &profilepb.SocialProfile{
			Type: &profilepb.SocialProfile_X{
				X: xProfile,
			},
		})
	} else if err != profile.ErrNotFound {
		return nil, err
	}

	return userProfile, nil
}

func (s *store) SetDisplayName(ctx context.Context, id *commonpb.UserId, displayName string) error {
	return dbSetDisplayName(ctx, s.pool, id, displayName)
}

func (s *store) LinkXAccount(ctx context.Context, userID *commonpb.UserId, xProfile *profilepb.XProfile, accessToken string) error {
	model, err := toXProfileModel(userID, xProfile, accessToken)
	if err != nil {
		return err
	}

	existing, err := dbGetXProfile(ctx, s.pool, userID)
	if err != nil && err != profile.ErrNotFound {
		return err
	}

	if existing != nil && existing.ID != xProfile.Id {
		return profile.ErrExistingSocialLink
	}

	return model.dbUpsert(ctx, s.pool)
}

func (s *store) UnlinkXAccount(ctx context.Context, userID *commonpb.UserId, xUserID string) error {
	return dbUnlinkXAccount(ctx, s.pool, userID, xUserID)
}

func (s *store) GetXProfile(ctx context.Context, userID *commonpb.UserId) (*profilepb.XProfile, error) {
	model, err := dbGetXProfile(ctx, s.pool, userID)
	if err != nil {
		return nil, err
	}
	return fromXProfileModel(model)
}

func (s *store) reset() {
	_, err := s.pool.Exec(context.Background(), "DELETE FROM "+xProfilesTableName)
	if err != nil {
		panic(err)
	}

	_, err = s.pool.Exec(context.Background(), `UPDATE `+usersTableName+` SET "displayName" = NULL`)
	if err != nil {
		panic(err)
	}
}
