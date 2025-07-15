//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	account "github.com/code-payments/flipcash-server/account/postgres"
	pg "github.com/code-payments/flipcash-server/database/postgres"
	"github.com/code-payments/flipcash-server/pool/tests"
	profile "github.com/code-payments/flipcash-server/profile/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPool_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	pg.SetupGlobalPgxPool(pool)

	accounts := account.NewInPostgres(pool)
	profiles := profile.NewInPostgres(pool)
	testStore := NewInPostgres(pool)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunServerTests(t, accounts, testStore, profiles, teardown)
}
