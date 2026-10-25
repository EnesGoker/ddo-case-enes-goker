//go:build integration

package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalTariffsSurviveReset(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_history")
	require.NoError(t, err)
	defer drop()
	require.NoError(t, db.Migrate(ctx, u, migrationsDir))
	p, err := db.Connect(ctx, u, 10*time.Second)
	require.NoError(t, err)
	defer p.Close()
	before := map[pricing.Product]pricing.Tariff{}
	for _, product := range []pricing.Product{pricing.Kasko, pricing.Trafik, pricing.Konut} {
		ts, err := tariff.LoadActive(ctx, p, product)
		require.NoError(t, err)
		require.Len(t, ts, 1, "migration seed'den önce yeterli")
		before[product] = ts[0]
	}
	_, err = p.Exec(ctx, `TRUNCATE tariff_versions CASCADE`)
	require.NoError(t, err)
	require.NoError(t, seed.Demo(ctx, p, demoPassword))
	for _, product := range []pricing.Product{pricing.Kasko, pricing.Trafik, pricing.Konut} {
		ts, err := tariff.LoadActive(ctx, p, product)
		require.NoError(t, err)
		historical := ts[0]
		assert.Zero(t, historical.VersionNo)
		assert.Equal(t, before[product].BaseRates, historical.BaseRates)
		assert.ElementsMatch(t, before[product].Factors, historical.Factors)
		assert.True(t, before[product].ValidFrom.Equal(historical.ValidFrom))
		assert.True(t, before[product].ValidTo.Equal(*historical.ValidTo))
		short, err := tariff.LoadShortPeriod(ctx, p, historical.ID)
		require.NoError(t, err)
		require.Len(t, short, 5)
	}
}
