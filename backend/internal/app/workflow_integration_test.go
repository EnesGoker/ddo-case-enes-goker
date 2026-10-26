//go:build integration

package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/app"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/claim"
	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/EnesGoker/guvenpolice/backend/internal/task"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http/httptest"
)

func principalContext(t *testing.T, email string, role auth.Role) context.Context {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email=$1`, email).Scan(&id))
	return auth.WithPrincipal(context.Background(), auth.Principal{UserID: id, Role: role})
}

func fraudDetail(t *testing.T, token string, c map[string]any, score int, flagged bool, codes ...string) map[string]any {
	t.Helper()
	status, detail := get(t, token, claimPath(c))
	require.Equal(t, http.StatusOK, status, detail)
	result := detail["fraud"].(map[string]any)
	assert.Equal(t, int64(score), num(result["score"]))
	assert.Equal(t, flagged, result["flagged"])
	assert.Equal(t, int64(60), num(result["threshold"]))
	got := []string{}
	for _, value := range result["signals"].([]any) {
		signal := value.(map[string]any)
		got = append(got, signal["rule_code"].(string))
		assert.NotEmpty(t, signal["explanation"])
		assert.NotEmpty(t, signal["detail"])
	}
	assert.ElementsMatch(t, codes, got)
	return detail
}

func TestFraudRulesPersistenceAndAutomaticWorkflow(t *testing.T) {
	defer assertBillingInvariant(t)
	token := tokenFor(t, "hasar@guvenpolice.test")
	today := caldate.Format(caldate.Today(time.Now()))
	t.Run("early_and_repeat", func(t *testing.T) {
		pol := billingPolicy(t, 5, 1, 100000)
		a := reportClaim(t, token, pol, "KASKO_ANA", today)
		fraudDetail(t, token, a, 40, false, "EARLY_CLAIM")
		b := reportClaim(t, token, pol, "KASKO_ANA", today)
		fraudDetail(t, token, b, 40, false, "EARLY_CLAIM")
		c := reportClaim(t, token, pol, "KASKO_ANA", today)
		detail := fraudDetail(t, token, c, 75, true, "EARLY_CLAIM", "REPEAT_PLATE")
		assert.Equal(t, "FRAUD_REVIEW", c["status"])
		history := detail["history"].([]any)
		require.Len(t, history, 3)
		for _, value := range history[1:] {
			h := value.(map[string]any)
			assert.Nil(t, h["actor_user_id"])
			assert.Equal(t, "Otomatik: fraud skoru 75 ≥ 60", h["note"])
		}
		tasks := detail["tasks"].([]any)
		require.Len(t, tasks, 1)
		row := tasks[0].(map[string]any)
		assert.Equal(t, "FRAUD_REVIEW", row["task_type"])
		assert.NotNil(t, row["assigned_to"])
		created, err := time.Parse(time.RFC3339Nano, row["created_at"].(string))
		require.NoError(t, err)
		due, err := time.Parse(time.RFC3339Nano, row["sla_due_at"].(string))
		require.NoError(t, err)
		assert.Equal(t, 24*time.Hour, due.Sub(created))
		var automatic int
		require.NoError(t, pool.QueryRow(context.Background(), `
   SELECT count(*) FROM audit_log WHERE entity_type='claim' AND entity_id=$1::text
    AND action='CLAIM_TRANSITIONED' AND new_value->>'auto'='true'`, fmt.Sprint(c["id"])).Scan(&automatic))
		assert.Equal(t, 2, automatic)
	})
	t.Run("repeat_alone", func(t *testing.T) {
		pol := billingPolicy(t, 30, 1, 100000)
		for i := 0; i < 2; i++ {
			c := reportClaim(t, token, pol, "KASKO_ANA", today)
			fraudDetail(t, token, c, 0, false)
		}
		c := reportClaim(t, token, pol, "KASKO_ANA", today)
		fraudDetail(t, token, c, 35, false, "REPEAT_PLATE")
	})
	t.Run("workshop_and_combination", func(t *testing.T) {
		var workshop int64
		require.NoError(t, pool.QueryRow(context.Background(), `
   INSERT INTO workshops(name,region_code) VALUES('Kurgusal Test Yoğunluk','34') RETURNING id`).Scan(&workshop))
		report := func(pol map[string]any) map[string]any {
			status, c, _ := postKey(t, token, "/api/v1/claims", map[string]any{"policy_id": pol["id"], "coverage_code": "KASKO_ANA", "loss_date": today, "description": "atölye testi", "claimed_amount_kurus": 1, "workshop_id": workshop}, fmt.Sprintf("workshop-%v-%d", pol["id"], time.Now().UnixNano()))
			require.Equal(t, http.StatusCreated, status, c)
			return c
		}
		for i := 0; i < 4; i++ {
			c := report(billingPolicy(t, 30, 1, 100000))
			fraudDetail(t, token, c, 0, false)
		}
		pol := billingPolicy(t, 30, 1, 100000)
		c := report(pol)
		fraudDetail(t, token, c, 30, false, "WORKSHOP_CONCENTRATION")
		report(pol)
		c = report(pol)
		fraudDetail(t, token, c, 65, true, "REPEAT_PLATE", "WORKSHOP_CONCENTRATION")
	})
}

func TestFraudPlateWindowInclusive365(t *testing.T) {
	defer assertBillingInvariant(t)
	token := tokenFor(t, "hasar@guvenpolice.test")
	today := caldate.Today(time.Now())
	plate := fixturePlate()
	old := billingPolicyWithPlate(t, 400, 1, 100000, plate)
	outside := reportClaim(t, token, old, "KASKO_ANA", caldate.Format(today.AddDate(0, 0, -366)))
	inside := reportClaim(t, token, old, "KASKO_ANA", caldate.Format(today.AddDate(0, 0, -365)))
	pol := billingPolicyWithPlate(t, 30, 1, 100000, plate)
	a := reportClaim(t, token, pol, "KASKO_ANA", caldate.Format(today))
	fraudDetail(t, token, a, 0, false) // One prior claim: day 366 excluded.
	b := reportClaim(t, token, pol, "KASKO_ANA", caldate.Format(today))
	d := fraudDetail(t, token, b, 35, false, "REPEAT_PLATE")
	signal := d["fraud"].(map[string]any)["signals"].([]any)[0].(map[string]any)
	previous := signal["detail"].(map[string]any)["previous_claim_nos"].([]any)
	assert.ElementsMatch(t, []any{inside["claim_no"], a["claim_no"]}, previous)
	assert.NotContains(t, previous, outside["claim_no"])
}

func TestTaskLifecycleSLAAndRBAC(t *testing.T) {
	defer assertBillingInvariant(t)
	adjuster, appraiser := tokenFor(t, "hasar@guvenpolice.test"), tokenFor(t, "eksper@guvenpolice.test")
	pol := claimPolicy(t)
	c := reportClaim(t, adjuster, pol, "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
	c = transitionClaim(t, adjuster, c, "TRIAGED")
	status, body := newClient(t).do(http.MethodPost, claimPath(c)+"/transition", adjuster, map[string]any{"to_status": "FRAUD_REVIEW", "expected_row_version": c["row_version"]})
	assert.Equal(t, http.StatusUnprocessableEntity, status, body)
	assert.Equal(t, "FRAUD_REVIEW_REASON_REQUIRED", errCode(body))
	c = transitionClaim(t, adjuster, c, "FRAUD_REVIEW")
	past := time.Now().Add(-4 * 24 * time.Hour).Truncate(time.Microsecond)
	svc := claim.NewService(pool, func() time.Time { return past }, fraud.DefaultConfig(), workflow.DefaultConfig())
	ctx := principalContext(t, "hasar@guvenpolice.test", auth.RoleClaimsAdjuster)
	entered, err := svc.ChangeStatus(ctx, num(c["id"]), claim.TransitionRequest{ToStatus: "APPRAISAL", ExpectedRowVersion: int(num(c["row_version"])), Note: "temiz"})
	require.NoError(t, err)
	status, list := get(t, adjuster, fmt.Sprintf("/api/v1/claim-tasks?claim_id=%d&breached=true", entered.ID))
	require.Equal(t, http.StatusOK, status, list)
	items := list["items"].([]any)
	require.Len(t, items, 1)
	row := items[0].(map[string]any)
	assert.Equal(t, "APPRAISAL", row["task_type"])
	assert.True(t, row["breached"].(bool))
	assert.Negative(t, num(row["remaining_seconds"]))
	// Server forces own assignment for appraisers, even when assigned_to is omitted.
	assigned := row["assigned_to"].(map[string]any)
	var assignedEmail string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT email FROM users WHERE id=$1`, num(assigned["id"])).Scan(&assignedEmail))
	otherEmail := "eksper@guvenpolice.test"
	if assignedEmail == otherEmail {
		otherEmail = "eksper2@guvenpolice.test"
	}
	status, other := get(t, tokenFor(t, otherEmail), fmt.Sprintf("/api/v1/claim-tasks?claim_id=%d&status=all", entered.ID))
	assert.Equal(t, http.StatusOK, status, other)
	assert.Empty(t, other["items"])
	appraiser = tokenFor(t, assignedEmail)
	status, body = newClient(t).do(http.MethodPost, claimPath(c)+"/appraisals", appraiser, map[string]any{"damage_amount_kurus": 100, "inspected_at": time.Now().UTC().Format(time.RFC3339)})
	require.Equal(t, http.StatusCreated, status, body)
	var taskStatus string
	var completed, due time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT status,completed_at,sla_due_at FROM claim_tasks WHERE claim_id=$1 AND task_type='APPRAISAL'`, entered.ID).Scan(&taskStatus, &completed, &due))
	assert.Equal(t, "DONE", taskStatus)
	assert.True(t, completed.After(due))
	c["row_version"] = float64(entered.RowVersion)
	c["status"] = entered.Status
	c = transitionClaim(t, adjuster, c, "ADJUSTMENT")
	transitionClaim(t, adjuster, c, "REJECTED")
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status FROM claim_tasks WHERE claim_id=$1 AND task_type='ADJUSTMENT'`, entered.ID).Scan(&taskStatus))
	assert.Equal(t, "DONE", taskStatus)
	status, summary := get(t, adjuster, "/api/v1/claim-tasks/sla-summary")
	require.Equal(t, http.StatusOK, status, summary)
	for _, value := range summary["items"].([]any) {
		r := value.(map[string]any)
		if r["task_type"] == "APPRAISAL" {
			assert.GreaterOrEqual(t, num(r["done_late"]), int64(1))
			assert.NotNil(t, r["on_time_bp"])
		}
	}
	// A fixed clock at exactly due is not breached; a microsecond later is breached.
	extra := claimPolicy(t)
	ec := reportClaim(t, adjuster, extra, "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
	ec = transitionClaim(t, adjuster, ec, "TRIAGED")
	ec = transitionClaim(t, adjuster, ec, "APPRAISAL")
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT sla_due_at FROM claim_tasks WHERE claim_id=$1`, num(ec["id"])).Scan(&due))
	for _, offset := range []time.Duration{0, time.Microsecond} {
		clock := due.Add(offset)
		fixed := httptest.NewServer(app.NewRouter(app.Deps{Pool: pool, Now: func() time.Time { return clock }, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), JWTSecret: []byte("integration-secret-at-least-32-bytes"), AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour}))
		req, err := http.NewRequest("GET", fixed.URL+fmt.Sprintf("/api/v1/claim-tasks?claim_id=%v&breached=true", ec["id"]), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+adjuster)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, err)
		if offset == 0 {
			assert.Contains(t, string(raw), `"items":[]`)
		} else {
			assert.Contains(t, string(raw), `"breached":true`)
		}
		metrics, err := http.Get(fixed.URL + "/metrics")
		require.NoError(t, err)
		raw, err = io.ReadAll(metrics.Body)
		metrics.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, metrics.StatusCode)
		var expectedBreached int64
		require.NoError(t, pool.QueryRow(context.Background(), `
         SELECT count(*) FROM claim_tasks WHERE task_type='APPRAISAL'
          AND status IN ('OPEN','IN_PROGRESS') AND sla_due_at<$1`, clock).Scan(&expectedBreached))
		assert.Contains(t, string(raw), fmt.Sprintf(`guvenpolice_claim_tasks_sla_breached{task_type="APPRAISAL"} %d`+"\n", expectedBreached))
		summaries, err := task.NewService(pool, func() time.Time { return clock }).Summary(context.Background())
		require.NoError(t, err)
		for _, summary := range summaries {
			if summary.Type == "APPRAISAL" {
				assert.Equal(t, expectedBreached, summary.OpenBreached)
			}
		}
		fixed.Close()
	}
	for _, email := range []string{"musteri@guvenpolice.test", "acente@guvenpolice.test"} {
		token := tokenFor(t, email)
		for _, path := range []string{"/api/v1/claim-tasks", "/api/v1/claim-tasks/sla-summary", "/api/v1/claim-tasks/workload"} {
			s, b := get(t, token, path)
			assert.Equal(t, http.StatusForbidden, s, b)
		}
		s, d := get(t, token, claimPath(c))
		require.Equal(t, http.StatusOK, s, d)
		assert.NotContains(t, d, "fraud")
		assert.NotContains(t, d, "fraud_score")
		assert.NotContains(t, d, "fraud_flagged")
		s, d = get(t, token, "/api/v1/claims")
		require.Equal(t, http.StatusOK, s, d)
		for _, v := range d["items"].([]any) {
			r := v.(map[string]any)
			assert.NotContains(t, r, "fraud")
			assert.NotContains(t, r, "fraud_score")
			assert.NotContains(t, r, "fraud_flagged")
		}
	}
}

func TestTaskEmptyPoolIsUnassigned(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT id FROM users WHERE role='APPRAISER' AND is_active`)
	require.NoError(t, err)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE users SET is_active=false WHERE id=ANY($1)`, ids)
	require.NoError(t, err)
	defer func() {
		_, err := pool.Exec(ctx, `UPDATE users SET is_active=true WHERE id=ANY($1)`, ids)
		assert.NoError(t, err)
	}()
	token := tokenFor(t, "hasar@guvenpolice.test")
	c := reportClaim(t, token, claimPolicy(t), "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
	c = transitionClaim(t, token, c, "TRIAGED")
	c = transitionClaim(t, token, c, "APPRAISAL")
	s, b := get(t, token, "/api/v1/claim-tasks?claim_id="+fmt.Sprint(c["id"]))
	require.Equal(t, http.StatusOK, s, b)
	require.Len(t, b["items"], 1)
	assert.Nil(t, b["items"].([]any)[0].(map[string]any)["assigned_to"])
}

func TestConcurrentTaskAssignment(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := context.Background()
	// A dedicated pool supplies 20 independent transactions plus the gate and observer.
	cfg, err := pgxpool.ParseConfig(pool.Config().ConnString())
	require.NoError(t, err)
	cfg.MaxConns = 24
	wide, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer wide.Close()
	token := tokenFor(t, "hasar@guvenpolice.test")
	actorCtx := principalContext(t, "hasar@guvenpolice.test", auth.RoleClaimsAdjuster)
	cs := make([]map[string]any, 20)
	for i := range cs {
		cs[i] = reportClaim(t, token, claimPolicy(t), "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
		cs[i] = transitionClaim(t, token, cs[i], "TRIAGED")
	}
	rows, err := pool.Query(ctx, `SELECT id FROM users WHERE role='CLAIMS_ADJUSTER' AND is_active`)
	require.NoError(t, err)
	old, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE users SET is_active=false WHERE id=ANY($1)`, old)
	require.NoError(t, err)
	ids := []int64{}
	defer func() {
		_, e := pool.Exec(ctx, `UPDATE users SET is_active=false WHERE id=ANY($1)`, ids)
		assert.NoError(t, e)
		_, e = pool.Exec(ctx, `UPDATE users SET is_active=true WHERE id=ANY($1)`, old)
		assert.NoError(t, e)
	}()
	for i := 0; i < 3; i++ {
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `
  INSERT INTO users(email,password_hash,full_name,role)
  SELECT $1,password_hash,'Kurgusal Yarış Uzmanı','CLAIMS_ADJUSTER' FROM users WHERE email='hasar@guvenpolice.test' RETURNING id`, fmt.Sprintf("race-%d-%d@test.invalid", time.Now().UnixNano(), i)).Scan(&id))
		ids = append(ids, id)
	}
	gate, err := wide.Begin(ctx)
	require.NoError(t, err)
	defer gate.Rollback(ctx)
	_, err = gate.Exec(ctx, `SELECT pg_advisory_xact_lock($1,hashtext($2))`, task.AssignmentLockNamespace, "CLAIMS_ADJUSTER")
	require.NoError(t, err)
	svc := claim.NewService(wide, time.Now, fraud.DefaultConfig(), workflow.DefaultConfig())
	errors := make(chan error, 20)
	var wg sync.WaitGroup
	for _, c := range cs {
		wg.Add(1)
		go func(c map[string]any) {
			defer wg.Done()
			_, e := svc.ChangeStatus(actorCtx, num(c["id"]), claim.TransitionRequest{ToStatus: "FRAUD_REVIEW", Note: "eşzamanlı atama", ExpectedRowVersion: int(num(c["row_version"]))})
			errors <- e
		}(c)
	}
	gateObserved := assert.Eventually(t, func() bool {
		var n int
		e := wide.QueryRow(ctx, `
  SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory'
   AND query LIKE '%pg_advisory_xact_lock%' AND pid<>pg_backend_pid()`).Scan(&n)
		return e == nil && n >= 20
	}, 5*time.Second, 10*time.Millisecond, "20 transaction advisory rol kilidinde beklemeli")
	require.NoError(t, gate.Commit(ctx))
	wg.Wait()
	close(errors)
	for e := range errors {
		assert.NoError(t, e)
	}
	loads := []int{}
	for _, id := range ids {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM claim_tasks WHERE assigned_to=$1 AND status='OPEN'`, id).Scan(&n))
		loads = append(loads, n)
	}
	assert.Equal(t, []int{7, 7, 6}, loads, "eşit başlangıç, küçük id önceliği")
	t.Logf("20 bağımsız transaction: rol kapısı=%t, yükler=%v", gateObserved, loads)
}

func TestFraudSeedStableAndMetricsDatabaseFailure(t *testing.T) {
	defer assertBillingInvariant(t)
	before := financialCounts(t)
	require.NoError(t, seed.Demo(context.Background(), pool, demoPassword))
	assert.Equal(t, before, financialCounts(t), "yeniden seed mali kayıt, sinyal ve görev üretmez")
	var total, zero, flagged int
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT count(*),count(*) FILTER(WHERE c.fraud_score=0 AND NOT c.fraud_flagged),
         count(*) FILTER(WHERE c.fraud_score=75 AND c.fraud_flagged AND c.status='FRAUD_REVIEW')
    FROM claims c JOIN policies p ON p.id=c.policy_id WHERE p.policy_no IN ('GP-DEMO-FP-1','GP-DEMO-FP-2')`).Scan(&total, &zero, &flagged))
	assert.Equal(t, 3, total)
	assert.Equal(t, 2, zero)
	assert.Equal(t, 1, flagged)
	var unpaidInterest int64
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT coalesce(sum(l.amount_kurus),0) FROM late_interest_charges l
   JOIN installments i ON i.id=l.installment_id JOIN installment_plans pl ON pl.id=i.plan_id
   JOIN policies p ON p.id=pl.policy_id WHERE p.policy_no IN ('GP-DEMO-FP-1','GP-DEMO-FP-2')`).Scan(&unpaidInterest))
	assert.Zero(t, unpaidInterest, "başlangıç tarihli tam ödeme faiz üretmez")
	closed, err := pgxpool.New(context.Background(), pool.Config().ConnString())
	require.NoError(t, err)
	closed.Close()
	handler := app.NewRouter(app.Deps{Pool: closed, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), `guvenpolice_claim_tasks_open{`)
	assert.NotContains(t, recorder.Body.String(), `guvenpolice_claim_tasks_sla_breached{`)
}
