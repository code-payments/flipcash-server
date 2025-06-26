package postgres

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"
	"github.com/code-payments/flipcash-server/database"
	pg "github.com/code-payments/flipcash-server/database/postgres"

	"github.com/code-payments/flipcash-server/pool"
)

const (
	poolsTableName = "flipcash_pools"
	allPoolFields  = `"id", "creatorId", "name", "buyInCurrency", "buyInAmount", "fundingDestination", "isOpen", "resolution", "signature", "createdAt", "closedAt", "updatedAt"`

	membersTableName         = "flipcash_poolmembers"
	allMemberFields          = `"id", ` + allMemberFieldsWithoutId
	allMemberFieldsWithoutId = `"poolId", "userId", "createdAt", "updatedAt"`

	betsTableName = "flipcash_bets"
	allBetFields  = `"id", "poolId", "userId", "selectedOutcome", "payoutDestination", "signature", "createdAt", "updatedAt"`
)

type poolModel struct {
	ID                 string       `db:"id"`
	CreatorID          string       `db:"creatorId"`
	Name               string       `db:"name"`
	BuyInCurrency      string       `db:"buyInCurrency"`
	BuyInAmount        float64      `db:"buyInAmount"`
	FundingDestination string       `db:"fundingDestination"`
	IsOpen             bool         `db:"isOpen"`
	Resolution         int          `db:"resolution"`
	Signature          string       `db:"signature"`
	CreatedAt          time.Time    `db:"createdAt"`
	ClosedAt           sql.NullTime `db:"closedAt"`
	UpdatedAt          time.Time    `db:"updatedAt"`
}

func toPoolModel(p *pool.Pool) *poolModel {
	var closedAt sql.NullTime
	if p.ClosedAt != nil {
		closedAt.Valid = true
		closedAt.Time = *p.ClosedAt
	}

	return &poolModel{
		ID:                 pg.Encode(p.ID.Value, pg.Base58),
		CreatorID:          pg.Encode(p.CreatorID.Value),
		Name:               p.Name,
		BuyInCurrency:      p.BuyInCurrency,
		BuyInAmount:        p.BuyInAmount,
		FundingDestination: pg.Encode(p.FundingDestination.Value, pg.Base58),
		IsOpen:             p.IsOpen,
		Resolution:         int(p.Resolution),
		Signature:          pg.Encode(p.Signature.Value, pg.Base58),
		CreatedAt:          p.CreatedAt,
		ClosedAt:           closedAt,
	}
}

func fromPoolModel(m *poolModel) (*pool.Pool, error) {
	decodedID, err := pg.Decode(m.ID)
	if err != nil {
		return nil, err
	}

	decodedCreatorID, err := pg.Decode(m.CreatorID)
	if err != nil {
		return nil, err
	}

	decodedFundingDestination, err := pg.Decode(m.FundingDestination)
	if err != nil {
		return nil, err
	}

	decodedSignature, err := pg.Decode(m.Signature)
	if err != nil {
		return nil, err
	}

	var closedAt *time.Time
	if m.ClosedAt.Valid {
		closedAt = &m.ClosedAt.Time
	}

	return &pool.Pool{
		ID:                 &poolpb.PoolId{Value: decodedID},
		CreatorID:          &commonpb.UserId{Value: decodedCreatorID},
		Name:               m.Name,
		BuyInCurrency:      m.BuyInCurrency,
		BuyInAmount:        m.BuyInAmount,
		FundingDestination: &commonpb.PublicKey{Value: decodedFundingDestination},
		IsOpen:             m.IsOpen,
		Resolution:         pool.Resolution(m.Resolution),
		Signature:          &commonpb.Signature{Value: decodedSignature},
		CreatedAt:          m.CreatedAt,
		ClosedAt:           closedAt,
	}, nil
}

type memberModel struct {
	ID        int64     `db:"id"`
	UserID    string    `db:"userId"`
	PoolID    string    `db:"poolId"`
	CreatedAt time.Time `db:"createdAt"`
	UpdatedAt time.Time `db:"updatedAt"`
}

func toMemberModel(userID *commonpb.UserId, poolID *poolpb.PoolId) *memberModel {
	return &memberModel{
		UserID: pg.Encode(userID.Value),
		PoolID: pg.Encode(poolID.Value, pg.Base58),
	}
}

func fromMemberModel(m *memberModel) (*pool.Member, error) {
	id := make([]byte, 8)
	binary.LittleEndian.PutUint64(id, uint64(m.ID))

	decodedUserID, err := pg.Decode(m.UserID)
	if err != nil {
		return nil, err
	}

	decodedPoolID, err := pg.Decode(m.PoolID)
	if err != nil {
		return nil, err
	}

	return &pool.Member{
		ID:     id,
		UserID: &commonpb.UserId{Value: decodedUserID},
		PoolID: &poolpb.PoolId{Value: decodedPoolID},
	}, nil
}

type betModel struct {
	ID                string    `db:"id"`
	PoolID            string    `db:"poolId"`
	UserID            string    `db:"userId"`
	SelectedOutcome   bool      `db:"selectedOutcome"`
	PayoutDestination string    `db:"payoutDestination"`
	Signature         string    `db:"signature"`
	CreatedAt         time.Time `db:"createdAt"`
	UpdatedAt         time.Time `db:"updatedAt"`
}

func toBetModel(b *pool.Bet) *betModel {
	return &betModel{
		ID:                pg.Encode(b.ID.Value, pg.Base58),
		PoolID:            pg.Encode(b.PoolID.Value, pg.Base58),
		UserID:            pg.Encode(b.UserID.Value),
		SelectedOutcome:   b.SelectedOutcome,
		PayoutDestination: pg.Encode(b.PayoutDestination.Value, pg.Base58),
		Signature:         pg.Encode(b.Signature.Value, pg.Base58),
		CreatedAt:         b.Ts,
	}
}

func fromBetModel(m *betModel) (*pool.Bet, error) {
	decodedID, err := pg.Decode(m.ID)
	if err != nil {
		return nil, err
	}

	decodedPoolID, err := pg.Decode(m.PoolID)
	if err != nil {
		return nil, err
	}

	decodedUserID, err := pg.Decode(m.UserID)
	if err != nil {
		return nil, err
	}

	decodedPayoutDestination, err := pg.Decode(m.PayoutDestination)
	if err != nil {
		return nil, err
	}

	decodedSignature, err := pg.Decode(m.Signature)
	if err != nil {
		return nil, err
	}

	return &pool.Bet{
		ID:                &poolpb.BetId{Value: decodedID},
		PoolID:            &poolpb.PoolId{Value: decodedPoolID},
		UserID:            &commonpb.UserId{Value: decodedUserID},
		SelectedOutcome:   m.SelectedOutcome,
		PayoutDestination: &commonpb.PublicKey{Value: decodedPayoutDestination},
		Signature:         &commonpb.Signature{Value: decodedSignature},
		Ts:                m.CreatedAt,
	}, nil
}

func (m *poolModel) dbPut(ctx context.Context, pgxPool *pgxpool.Pool) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `INSERT INTO ` + poolsTableName + `(` + allPoolFields + `)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
			RETURNING ` + allPoolFields
		err := pgxscan.Get(
			ctx,
			tx,
			m,
			query,
			m.ID,
			m.CreatorID,
			m.Name,
			m.BuyInCurrency,
			m.BuyInAmount,
			m.FundingDestination,
			m.IsOpen,
			m.Resolution,
			m.Signature,
			m.CreatedAt,
			m.ClosedAt,
		)
		if err == nil {
			return nil
		} else if strings.Contains(err.Error(), "23505") { // todo: better utility for detecting unique violations with pgxscan
			if strings.Contains(err.Error(), "fundingDestination") {
				return pool.ErrPoolFundingDestinationExists
			}
			return pool.ErrPoolIDExists
		}
		return err
	})
}

func (m *memberModel) dbPut(ctx context.Context, pgxPool *pgxpool.Pool) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `INSERT INTO ` + membersTableName + `(` + allMemberFieldsWithoutId + `)
			VALUES ($1, $2, NOW(), NOW())
			ON CONFLICT DO NOTHING
			RETURNING ` + allMemberFields
		err := pgxscan.Get(
			ctx,
			tx,
			m,
			query,
			m.PoolID,
			m.UserID,
		)
		if pgxscan.NotFound(err) {
			return nil
		}
		return err
	})
}

func (m *betModel) dbPut(ctx context.Context, pgxPool *pgxpool.Pool) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `INSERT INTO ` + betsTableName + `(` + allBetFields + `)
			SELECT $1, $2, $3, $4, $5, $6, $7, NOW()
			WHERE (SELECT COUNT(*) FROM ` + betsTableName + ` WHERE "poolId" = $2) < $8
			RETURNING ` + allBetFields
		err := pgxscan.Get(
			ctx,
			tx,
			m,
			query,
			m.ID,
			m.PoolID,
			m.UserID,
			m.SelectedOutcome,
			m.PayoutDestination,
			m.Signature,
			m.CreatedAt,
			pool.MaxParticipants,
		)
		if err == nil {
			return nil
		} else if pgxscan.NotFound(err) {
			return pool.ErrMaxBetCountExceeded
		} else if strings.Contains(err.Error(), "23505") { // todo: better utility for detecting unique violations with pgxscan
			return pool.ErrBetExists
		}
		return err
	})
}

func dbGetPoolByID(ctx context.Context, pgxPool *pgxpool.Pool, poolID *poolpb.PoolId) (*poolModel, error) {
	res := &poolModel{}
	query := `SELECT ` + allPoolFields + ` FROM ` + poolsTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pgxPool,
		res,
		query,
		pg.Encode(poolID.Value, pg.Base58),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, pool.ErrPoolNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbClosePool(ctx context.Context, pgxPool *pgxpool.Pool, poolID *poolpb.PoolId, closedAt time.Time, newSignature *commonpb.Signature) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `UPDATE ` + poolsTableName + `
			SET "isOpen" = FALSE, "closedAt" = $2, "signature" = $3, "updatedAt" = NOW()
			WHERE "id" = $1 AND "isOpen" = TRUE AND "resolution" = $4`
		cmd, err := tx.Exec(
			ctx,
			query,
			pg.Encode(poolID.Value, pg.Base58),
			closedAt,
			pg.Encode(newSignature.Value, pg.Base58),
			pool.ResolutionUnknown,
		)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			_, err := dbGetPoolByID(ctx, pgxPool, poolID)
			switch err {
			case nil:
				return nil
			case pool.ErrPoolNotFound:
				return pool.ErrPoolNotFound
			default:
				return err
			}
		}
		return nil
	})
}

func dbResolvePool(ctx context.Context, pgxPool *pgxpool.Pool, poolID *poolpb.PoolId, resolution pool.Resolution, newSignature *commonpb.Signature) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `UPDATE ` + poolsTableName + `
			SET "resolution" = $2, "signature" = $3, "updatedAt" = NOW()
			WHERE "id" = $1 AND "isOpen" = FALSE AND "resolution" = $4`
		cmd, err := tx.Exec(
			ctx,
			query,
			pg.Encode(poolID.Value, pg.Base58),
			resolution,
			pg.Encode(newSignature.Value, pg.Base58),
			pool.ResolutionUnknown,
		)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			existing, err := dbGetPoolByID(ctx, pgxPool, poolID)
			switch err {
			case nil:
				if existing.IsOpen {
					return pool.ErrPoolOpen
				}
				return pool.ErrPoolResolved
			case pool.ErrPoolNotFound:
				return pool.ErrPoolNotFound
			default:
				return err
			}
		}
		return nil
	})
}

func dbUpdateBetOutcome(ctx context.Context, pgxPool *pgxpool.Pool, betID *poolpb.BetId, newOutcome bool, newSignature *commonpb.Signature, newTs time.Time) error {
	query := `UPDATE ` + betsTableName + `
		SET  "selectedOutcome" = $2, "signature" = $3, "createdAt" = $4
		WHERE "id" = $1`

	cmd, err := pgxPool.Exec(
		ctx,
		query,
		pg.Encode(betID.Value, pg.Base58),
		newOutcome,
		pg.Encode(newSignature.Value, pg.Base58),
		newTs,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pool.ErrBetNotFound
	}
	return nil
}

func dbGetBetByUser(ctx context.Context, pgxPool *pgxpool.Pool, poolID *poolpb.PoolId, userID *commonpb.UserId) (*betModel, error) {
	res := &betModel{}
	query := `SELECT ` + allBetFields + ` FROM ` + betsTableName +
		` WHERE "poolId" = $1 AND "userId" = $2`
	err := pgxscan.Get(
		ctx,
		pgxPool,
		res,
		query,
		pg.Encode(poolID.Value, pg.Base58),
		pg.Encode(userID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, pool.ErrBetNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbGetBetsByPool(ctx context.Context, pgxPool *pgxpool.Pool, poolID *poolpb.PoolId) ([]*betModel, error) {
	var res []*betModel
	query := `SELECT ` + allBetFields + ` FROM ` + betsTableName +
		` WHERE "poolId" = $1`
	err := pgxscan.Select(
		ctx,
		pgxPool,
		&res,
		query,
		pg.Encode(poolID.Value, pg.Base58),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, pool.ErrBetNotFound
		}
		return nil, err
	}
	if len(res) == 0 {
		return nil, pool.ErrBetNotFound
	}
	return res, nil
}

func dbGetPagedMembers(ctx context.Context, pgxPool *pgxpool.Pool, userID *commonpb.UserId, queryOptions ...database.QueryOption) ([]*memberModel, error) {
	var res []*memberModel

	appliedQueryptions := database.ApplyQueryOptions(queryOptions...)
	queryParameters := []any{pg.Encode(userID.Value)}
	query := `SELECT ` + allMemberFields + ` FROM ` + membersTableName +
		` WHERE "userId" = $1`

	if appliedQueryptions.PagingToken != nil {
		if len(appliedQueryptions.PagingToken.Value) != 8 {
			return nil, errors.New("invalid paging token")
		}

		integerPagingToken := binary.LittleEndian.Uint64(appliedQueryptions.PagingToken.Value)

		queryParameters = append(queryParameters, integerPagingToken)
		if appliedQueryptions.Order == commonpb.QueryOptions_ASC {
			query += fmt.Sprintf(` AND "id" > $%d`, len(queryParameters))
		} else {
			query += fmt.Sprintf(` AND "id" < $%d`, len(queryParameters))
		}
	}

	if appliedQueryptions.Order == commonpb.QueryOptions_ASC {
		query += ` ORDER BY "id" ASC`
	} else {
		query += ` ORDER BY "id" DESC`
	}

	if appliedQueryptions.Limit > 0 {
		queryParameters = append(queryParameters, appliedQueryptions.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(queryParameters))
	}

	err := pgxscan.Select(
		ctx,
		pgxPool,
		&res,
		query,
		queryParameters...,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, pool.ErrMemberNotFound
		}
		return nil, err
	}
	if len(res) == 0 {
		return nil, pool.ErrMemberNotFound
	}
	return res, nil
}
