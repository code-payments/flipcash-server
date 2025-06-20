package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"
	pg "github.com/code-payments/flipcash-server/database/postgres"

	"github.com/code-payments/flipcash-server/pool"
)

const (
	poolsTableName = "flipcash_pools"
	allPoolFields  = `"id", "creatorId", "name", "buyInCurrency", "buyInAmount", "fundingDestination", "isOpen", "resolution", "signature", "createdAt", "updatedAt"`

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
	Resolution         sql.NullBool `db:"resolution"`
	Signature          string       `db:"signature"`
	CreatedAt          time.Time    `db:"createdAt"`
	UpdatedAt          time.Time    `db:"updatedAt"`
}

func toPoolModel(p *pool.Pool) *poolModel {
	var resolution sql.NullBool
	if p.Resolution != nil {
		resolution.Valid = true
		resolution.Bool = *p.Resolution
	}

	return &poolModel{
		ID:                 pg.Encode(p.ID.Value, pg.Base58),
		CreatorID:          pg.Encode(p.CreatorID.Value),
		Name:               p.Name,
		BuyInCurrency:      p.BuyInCurrency,
		BuyInAmount:        p.BuyInAmount,
		FundingDestination: pg.Encode(p.FundingDestination.Value, pg.Base58),
		IsOpen:             p.IsOpen,
		Resolution:         resolution,
		Signature:          pg.Encode(p.Signature.Value, pg.Base58),
		CreatedAt:          p.CreatedAt,
	}
}

func fromPoolModel(m *poolModel) (*pool.Pool, error) {
	decodedId, err := pg.Decode(m.ID)
	if err != nil {
		return nil, err
	}

	decodedCreatorId, err := pg.Decode(m.CreatorID)
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

	var resolution *bool
	if m.Resolution.Valid {
		resolution = &m.Resolution.Bool
	}

	return &pool.Pool{
		ID:                 &poolpb.PoolId{Value: decodedId},
		CreatorID:          &commonpb.UserId{Value: decodedCreatorId},
		Name:               m.Name,
		BuyInCurrency:      m.BuyInCurrency,
		BuyInAmount:        m.BuyInAmount,
		FundingDestination: &commonpb.PublicKey{Value: decodedFundingDestination},
		IsOpen:             m.IsOpen,
		Resolution:         resolution,
		Signature:          &commonpb.Signature{Value: decodedSignature},
		CreatedAt:          m.CreatedAt,
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
	decodedId, err := pg.Decode(m.ID)
	if err != nil {
		return nil, err
	}

	decodedPoolId, err := pg.Decode(m.PoolID)
	if err != nil {
		return nil, err
	}

	decodedUserId, err := pg.Decode(m.UserID)
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
		ID:                &poolpb.BetId{Value: decodedId},
		PoolID:            &poolpb.PoolId{Value: decodedPoolId},
		UserID:            &commonpb.UserId{Value: decodedUserId},
		SelectedOutcome:   m.SelectedOutcome,
		PayoutDestination: &commonpb.PublicKey{Value: decodedPayoutDestination},
		Signature:         &commonpb.Signature{Value: decodedSignature},
		Ts:                m.CreatedAt,
	}, nil
}

func (m *poolModel) dbPut(ctx context.Context, pgxPool *pgxpool.Pool) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `INSERT INTO ` + poolsTableName + `(` + allPoolFields + `)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
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

func (m *betModel) dbPut(ctx context.Context, pgxPool *pgxpool.Pool) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `INSERT INTO ` + betsTableName + `(` + allBetFields + `)
			SELECT $1, $2, $3, $4, $5, $6, $7, NOW()
			WHERE (SELECT COUNT(*) FROM ` + betsTableName + ` WHERE "poolId" = $2) <= $8
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

func dbResolvePool(ctx context.Context, pgxPool *pgxpool.Pool, poolID *poolpb.PoolId, resolution bool, newSignature *commonpb.Signature) error {
	return pg.ExecuteInTx(ctx, pgxPool, func(tx pgx.Tx) error {
		query := `UPDATE ` + poolsTableName + `
			SET "resolution" = $2, "signature" = $3, "isOpen" = FALSE, "updatedAt" = NOW()
			WHERE "id" = $1 AND "resolution" IS NULL`
		cmd, err := tx.Exec(
			ctx,
			query,
			pg.Encode(poolID.Value, pg.Base58),
			resolution,
			pg.Encode(newSignature.Value, pg.Base58),
		)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			_, err = dbGetPoolByID(ctx, pgxPool, poolID)
			switch err {
			case nil:
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
