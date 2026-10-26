//go:build integration

package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventLimitSnapshotAndPerClaimApproval(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := context.Background()
	agent, adjuster := tokenFor(t, "acente@guvenpolice.test"), tokenFor(t, "hasar@guvenpolice.test")
	today := caldate.Today(time.Now())
	request := kaskoBody(t, ayse, caldate.Format(today))
	request["plate"] = fixturePlate()
	q := createQuote(t, agent, request)
	s, pol, _ := postKey(t, agent, fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"]), map[string]any{"installment_count": 1}, fmt.Sprintf("event-policy-%v", q["id"]))
	require.Equal(t, http.StatusCreated, s, pol)
	var event int64
	require.NoError(t, pool.QueryRow(ctx, `
      SELECT event_limit_kurus FROM policy_version_coverages WHERE policy_version_id=$1 AND coverage_code='CAM'`, num(pol["version_id"])).Scan(&event))
	assert.Equal(t, int64(1_000_000), event)
	for i := range 2 {
		c := reportClaim(t, adjuster, pol, "CAM", caldate.Format(today))
		c = transitionClaim(t, adjuster, c, "TRIAGED")
		c = transitionClaim(t, adjuster, c, "APPRAISAL")
		s, body, _ := postKey(t, tokenFor(t, "eksper@guvenpolice.test"), claimPath(c)+"/appraisals", map[string]any{
			"damage_amount_kurus": 1_200_000, "inspected_at": time.Now().UTC().Format(time.RFC3339),
		}, fmt.Sprintf("event-appraisal-%v", c["id"]))
		require.Equal(t, http.StatusCreated, s, body)
		c = transitionClaim(t, adjuster, c, "ADJUSTMENT")
		s, preview := newClient(t).do(http.MethodPost, claimPath(c)+"/settlement/preview", adjuster, nil)
		require.Equal(t, http.StatusOK, s, preview)
		assert.Equal(t, int64(1_000_000), num(preview["payable_kurus"]))
		steps := preview["calc_steps"].([]any)
		require.Len(t, steps, 7)
		assert.Equal(t, "EVENT_LIMIT", steps[3].(map[string]any)["code"])
		assert.Equal(t, int64(150_000), num(steps[3].(map[string]any)["clipped_kurus"]))
		s, approved, _ := postKey(t, adjuster, claimPath(c)+"/settlement/approve", nil, fmt.Sprintf("event-approve-%v", c["id"]))
		require.Equal(t, http.StatusCreated, s, approved)
		assert.Equal(t, event, num(approved["payable_kurus"]), "her olay kendi limitini kullanır; sıra%d", i)
	}
	var used int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT used_kurus FROM policy_limit_balances WHERE policy_id=$1 AND coverage_code='CAM'`, num(pol["id"])).Scan(&used))
	assert.Equal(t, int64(2_000_000), used, "olay başı limit dönem boyunca tüketilen ayrı bir sayaç değildir")
	s, body, _ := postKey(t, agent, policyPath(pol)+"/endorsements", map[string]any{
		"effective_date": caldate.Format(today.AddDate(0, 0, 1)), "vehicle_group": "D", "base_version_no": 1,
	}, fmt.Sprintf("event-zeyil-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, body)
	var versions int
	require.NoError(t, pool.QueryRow(ctx, `
      SELECT count(*) FROM policy_version_coverages c JOIN policy_versions v ON v.id=c.policy_version_id
       WHERE v.policy_id=$1 AND c.coverage_code='CAM' AND c.event_limit_kurus=1000000`, num(pol["id"])).Scan(&versions))
	assert.Equal(t, 2, versions, "zeyil yeni sürüme olay limitini kopyalar")
	legacy := claimPolicy(t)
	var absent *int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT event_limit_kurus FROM policy_version_coverages WHERE policy_version_id=$1 AND coverage_code='CAM'`, num(legacy["version_id"])).Scan(&absent))
	assert.Nil(t, absent, "eski/NULL sözleşme güncel katalogdan limit devralmaz")
}

func TestEventLimitMigrationUpDownUpAndDataGuard(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_event_migration")
	require.NoError(t, err)
	defer drop()
	require.NoError(t, db.Migrate(ctx, u, migrationsDir))
	p, err := pgxpool.New(ctx, u)
	require.NoError(t, err)
	defer p.Close()
	require.NoError(t, seed.Demo(ctx, p, demoPassword))
	sqlDB, err := sql.Open("pgx", u)
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 11))
	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir))
	var limits int
	require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM policy_version_coverages WHERE event_limit_kurus IS NOT NULL`).Scan(&limits))
	assert.Zero(t, limits, "mevcut kayıtlar NULL kalır")
	_, err = p.Exec(ctx, `
      INSERT INTO policy_version_coverages(policy_version_id,coverage_code,limit_kurus,deductible_type,deductible_value,annual_premium_kurus,event_limit_kurus)
      SELECT id,'CAM',2500000,'FIXED',50000,100,1000000 FROM policy_versions ORDER BY id LIMIT 1`)
	require.NoError(t, err)
	assert.ErrorContains(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 11), "Down veri kaybı")
	_, err = p.Exec(ctx, `
      INSERT INTO policy_version_coverages(policy_version_id,coverage_code,limit_kurus,deductible_type,deductible_value,annual_premium_kurus,event_limit_kurus)
      SELECT id,'IMM',1000,'NONE',0,0,0 FROM policy_versions ORDER BY id LIMIT 1`)
	assert.ErrorContains(t, err, "event_limit_positive")
}
