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

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivitySummaryOnlyFinancialRoles(t *testing.T) {
	ctx := context.Background()
	var id int64
	require.NoError(t, pool.QueryRow(ctx, `
   INSERT INTO simulation_runs(seed,parameters,params_hash,status,phase,processed,target,summary)
   VALUES(42,'{}','a8-summary','COMPLETED','DONE',1000,1000,'{"ledger":{"cash_kurus":12345},"balanced":true}') RETURNING id`).Scan(&id))
	defer func() {
		_, err := pool.Exec(ctx, `DELETE FROM simulation_runs WHERE id=$1`, id)
		require.NoError(t, err)
	}()
	for _, name := range []string{"musteri", "acente", "underwriter", "eksper", "hasar", "muhasebe"} {
		s, body := get(t, tokenFor(t, name+"@guvenpolice.test"), "/api/v1/system/activity")
		require.Equal(t, http.StatusOK, s, body)
		assert.Len(t, body["minutes"], 30)
		if name == "underwriter" || name == "muhasebe" {
			require.NotNil(t, body["simulation"])
			summary := body["simulation"].(map[string]any)["summary"].(map[string]any)
			assert.Equal(t, float64(12345), summary["ledger"].(map[string]any)["cash_kurus"])
		} else {
			assert.Nil(t, body["simulation"])
		}
	}
}

func TestBusinessKeyRejectsChangedMeaningWithoutMiddlewareRecord(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	pol := billingPolicy(t, 0, 1, 30000)
	path := policyPath(pol) + "/payments"
	key := fmt.Sprintf("a8-pay-%v", pol["id"])
	body := map[string]any{"amount_kurus": 1000, "method": "CARD"}
	s, first, _ := postKey(t, account, path, body, key)
	require.Equal(t, http.StatusCreated, s, first)
	second := payBilling(t, account, pol, 1000, "a8-second")
	clearKey := func(key string) {
		_, err := pool.Exec(context.Background(), `DELETE FROM idempotency_keys WHERE idem_key=$1`, key)
		require.NoError(t, err)
	}
	baseline := financialCounts(t)
	assertMismatch := func(path, key string, body any) {
		t.Helper()
		clearKey(key)
		s, out, _ := postKey(t, account, path, body, key)
		assert.Equal(t, http.StatusUnprocessableEntity, s, out)
		assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", errCode(out))
		assert.Equal(t, baseline, financialCounts(t))
	}
	assertMismatch(path, key, map[string]any{"amount_kurus": 2000, "method": "CARD"})
	refundPath := fmt.Sprintf("/api/v1/payments/%v/refund", first["id"])
	assertMismatch(refundPath, key, map[string]any{"amount_kurus": 1000, "reason": "yanlış tür"})
	clearKey(key)
	s, replay, _ := postKey(t, account, path, body, key)
	require.Equal(t, http.StatusCreated, s, replay)
	assert.Equal(t, first["id"], replay["id"])
	refundKey := fmt.Sprintf("a8-refund-%v", pol["id"])
	refundBody := map[string]any{"amount_kurus": 100, "reason": "kısmi iade"}
	s, refund, _ := postKey(t, account, refundPath, refundBody, refundKey)
	require.Equal(t, http.StatusCreated, s, refund)
	baseline = financialCounts(t)
	assertMismatch(refundPath, refundKey, map[string]any{"amount_kurus": 200, "reason": "farklı tutar"})
	assertMismatch(fmt.Sprintf("/api/v1/payments/%v/refund", second["id"]), refundKey, refundBody)
	assertMismatch(path, refundKey, body)
	clearKey(refundKey)
	s, replay, _ = postKey(t, account, refundPath, refundBody, refundKey)
	require.Equal(t, http.StatusCreated, s, replay)
	assert.Equal(t, refund["id"], replay["id"])

	agent := tokenFor(t, "acente@guvenpolice.test")
	ep := issuePolicy(t, agent)
	endorsementKey := fmt.Sprintf("a8-zeyil-%v", ep["id"])
	s, out, _ := postKey(t, agent, policyPath(ep)+"/endorsements", map[string]any{"effective_date": day(dateOf(t, ep["start_date"]), 10), "vehicle_group": "D", "base_version_no": 1}, endorsementKey)
	require.Equal(t, http.StatusCreated, s, out)
	baseline = financialCounts(t)
	clearKey(endorsementKey)
	s, out, _ = postKey(t, agent, policyPath(ep)+"/cancel", map[string]any{"effective_date": day(dateOf(t, ep["start_date"]), 20), "reason": "CUSTOMER_REQUEST", "base_version_no": 2}, endorsementKey)
	assert.Equal(t, http.StatusUnprocessableEntity, s, out)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", errCode(out))
	assert.Equal(t, baseline, financialCounts(t))
}

func TestUnchangedBillingRunDoesNotWriteAudit(t *testing.T) {
	defer assertBillingInvariant(t)
	pol := billingPolicy(t, 45, 1, 30000)
	ctx := principalContext(t, "muhasebe@guvenpolice.test", auth.RoleAccounting)
	now := time.Now()
	opts := payment.RunOptions{AfterPolicyID: num(pol["id"]) - 1, Limit: 1}
	svc := payment.NewService(pool, func() time.Time { return now }, 1000)
	first, err := svc.Run(ctx, "", opts)
	require.NoError(t, err)
	require.Len(t, first.Cancelled, 1)
	baseline := financialCounts(t)
	for _, advance := range []time.Duration{0, 24 * time.Hour} {
		svc = payment.NewService(pool, func() time.Time { return now.Add(advance) }, 1000)
		out, err := svc.Run(ctx, "", opts)
		require.NoError(t, err)
		assert.Zero(t, out.InterestKurus)
		assert.Empty(t, out.Cancelled)
		assert.Equal(t, baseline, financialCounts(t), "değişmeyen poliçe audit üretmez")
	}
}

func TestAppraisalIdempotencyAndRequiredKey(t *testing.T) {
	pol := claimPolicy(t)
	adj, expert := tokenFor(t, "hasar@guvenpolice.test"), tokenFor(t, "eksper@guvenpolice.test")
	c := reportClaim(t, adj, pol, "KASKO_ANA", time.Now().Format("2006-01-02"))
	c = transitionClaim(t, adj, c, "TRIAGED")
	c = transitionClaim(t, adj, c, "APPRAISAL")
	path := claimPath(c) + "/appraisals"
	body := map[string]any{"damage_amount_kurus": 1000, "inspected_at": time.Now().UTC().Format(time.RFC3339)}
	s, out := newClient(t).do(http.MethodPost, path, expert, body)
	assert.Equal(t, http.StatusBadRequest, s, out)
	key := fmt.Sprintf("a8-expert-%v", c["id"])
	s, first, _ := postKey(t, expert, path, body, key)
	require.Equal(t, http.StatusCreated, s, first)
	s, out, h := postKey(t, expert, path, body, key)
	require.Equal(t, http.StatusCreated, s, out)
	assert.Equal(t, first, out)
	assert.Equal(t, "true", h.Get("Idempotent-Replayed"))
	body["damage_amount_kurus"] = 2000
	s, out, _ = postKey(t, expert, path, body, key)
	assert.Equal(t, http.StatusUnprocessableEntity, s, out)
	var appraisals, audits int
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT (SELECT count(*) FROM appraisals WHERE claim_id=$1),
         (SELECT count(*) FROM audit_log WHERE entity_type='claim' AND entity_id=$2 AND action='CLAIM_APPRAISED')`, num(c["id"]), fmt.Sprint(num(c["id"]))).Scan(&appraisals, &audits))
	assert.Equal(t, 1, appraisals)
	assert.Equal(t, 1, audits)
}

func TestRefundIndexRecoversInvalidConcurrentBuild(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_invalid_index")
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
	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 9))
	var before int
	require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM payments`).Scan(&before))
	require.Greater(t, before, 1)
	_, err = p.Exec(ctx, `CREATE UNIQUE INDEX CONCURRENTLY payments_refund_of_payment_idx ON payments(kind)`)
	require.Error(t, err, "kasıtlı yinelenen kind geçersiz indeks bırakır")
	var valid bool
	require.NoError(t, p.QueryRow(ctx, `SELECT indisvalid FROM pg_index WHERE indexrelid='payments_refund_of_payment_idx'::regclass`).Scan(&valid))
	require.False(t, valid)
	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir))
	var after int
	var definition string
	require.NoError(t, p.QueryRow(ctx, `
  SELECT indisvalid AND indisready,pg_get_indexdef(indexrelid), (SELECT count(*) FROM payments)
  FROM pg_index WHERE indexrelid='payments_refund_of_payment_idx'::regclass`).Scan(&valid, &definition, &after))
	assert.True(t, valid)
	assert.Contains(t, definition, "refund_of_payment_id")
	assert.Equal(t, before, after)
}
