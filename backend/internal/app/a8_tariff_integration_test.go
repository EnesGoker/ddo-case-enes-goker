//go:build integration

package app_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV0StartMigrationPreservesReferencesAndRejectsEarlierBusiness(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_v0_start")
	require.NoError(t, err)
	defer drop()
	require.NoError(t, db.Migrate(ctx, u, migrationsDir))
	p, err := pgxpool.New(ctx, u)
	require.NoError(t, err)
	defer p.Close()
	sqlDB, err := sql.Open("pgx", u)
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 10))
	require.NoError(t, seed.Demo(ctx, p, demoPassword))
	var tariffID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM tariff_versions WHERE product='KASKO' AND version_no=0`).Scan(&tariffID))
	// Yalnız bu geçici DB'de, migration FK korumasını sınayan en küçük kayıt.
	addVersion := func(no, start, end string) int64 {
		var policyID, versionID int64
		require.NoError(t, p.QueryRow(ctx, `
          INSERT INTO policies(policy_no,product,customer_id,start_date,end_date)
          SELECT $1,'KASKO',id,$2::date,$3::date FROM customers ORDER BY id LIMIT 1 RETURNING id`, no, start, end).Scan(&policyID))
		require.NoError(t, p.QueryRow(ctx, `
          INSERT INTO policy_versions(policy_id,version_no,effective_from,tariff_version_id,risk_data,annual_premium_kurus)
          VALUES($1,1,$2::date,$3,'{}',850000) RETURNING id`, policyID, start, tariffID).Scan(&versionID))
		return versionID
	}
	versionID := addVersion("V0-2021", "2021-01-01", "2022-01-01")
	for range 2 {
		require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir))
		var currentTariff, premium int64
		var start string
		require.NoError(t, p.QueryRow(ctx, `
          SELECT v.tariff_version_id,v.annual_premium_kurus,t.valid_from::text
            FROM policy_versions v JOIN tariff_versions t ON t.id=v.tariff_version_id WHERE v.id=$1`, versionID).Scan(&currentTariff, &premium, &start))
		assert.Equal(t, tariffID, currentTariff)
		assert.Equal(t, int64(850000), premium)
		assert.Equal(t, "2020-01-01", start)
		require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 10))
	}
	addVersion("V0-2019", "2019-01-01", "2020-01-01")
	assert.ErrorContains(t, goose.UpContext(ctx, sqlDB, migrationsDir), "2020 öncesi bağlı kayıt")
	var unchanged int
	require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM tariff_versions WHERE version_no=0 AND valid_from=DATE '0001-01-01'`).Scan(&unchanged))
	assert.Equal(t, 3, unchanged, "başarısız migration kısmi tarih düzeltmesi bırakmaz")
}
