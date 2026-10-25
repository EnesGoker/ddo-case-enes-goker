//go:build integration

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/claim"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
)

func TestBackdatedAdditionalPremiumDueToday(t *testing.T) {
	defer assertBillingInvariant(t)
	account, agent := tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "acente@guvenpolice.test")
	today := caldate.Today(time.Now())
	for _, paid := range []bool{true, false} {
		pol := billingPolicy(t, 90, 1, 100000)
		if paid {
			payBilling(t, account, pol, 1, "accrue")
			payBilling(t, account, pol, num(billingDetail(t, account, pol)["open_balance_kurus"]), "full")
		}
		s, z, _ := postKey(t, agent, policyPath(pol)+"/endorsements", map[string]any{"effective_date": day(today, -40), "vehicle_group": "D"}, fmt.Sprintf("back-premium-%v", pol["id"]))
		require.Equal(t, http.StatusCreated, s, z)
		b := billingDetail(t, account, pol)
		rows := b["installments"].([]any)
		require.Len(t, rows, 2)
		assert.Equal(t, int64(100000), num(rows[0].(map[string]any)["amount_kurus"]), "gecikmiş taksite ek prim yüklenmez")
		assert.Equal(t, caldate.Format(today), rows[1].(map[string]any)["due_date"])
		if paid {
			var before int
			require.NoError(t, pool.QueryRow(context.Background(), `
 SELECT count(*) FROM late_interest_charges c
 JOIN installments i ON i.id=c.installment_id
 JOIN installment_plans pl ON pl.id=i.plan_id WHERE pl.policy_id=$1`, num(pol["id"])).Scan(&before))
			s, r := newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, nil)
			require.Equal(t, http.StatusOK, s, r)
			assert.Equal(t, "ACTIVE", billingDetail(t, account, pol)["policy_status"])
			var after int
			require.NoError(t, pool.QueryRow(context.Background(), `
 SELECT count(*) FROM late_interest_charges c
 JOIN installments i ON i.id=c.installment_id
 JOIN installment_plans pl ON pl.id=i.plan_id WHERE pl.policy_id=$1`, num(pol["id"])).Scan(&after))
			assert.Equal(t, before, after, "bugün vadeli ek prime geçmiş faiz yok")
		}
	}
}

func TestCancellationAccruesBeforeAdjustment(t *testing.T) {
	defer assertBillingInvariant(t)
	account, uw := tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "underwriter@guvenpolice.test")
	manual, automatic := billingPolicy(t, 30, 3, 300000), billingPolicy(t, 30, 3, 300000)
	s, b, _ := postKey(t, uw, policyPath(manual)+"/cancel", map[string]any{"effective_date": caldate.Format(caldate.Today(time.Now())), "reason": "UNDERWRITER"}, fmt.Sprintf("manual-interest-%v", manual["id"]))
	require.Equal(t, http.StatusCreated, s, b)
	s, b = newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, nil)
	require.Equal(t, http.StatusOK, s, b)
	for _, pol := range []map[string]any{manual, automatic} {
		b = billingDetail(t, account, pol)
		assert.Equal(t, "CANCELLED", b["policy_status"])
		charges := b["interest"].([]any)
		require.Len(t, charges, 1)
		// İlk vade anaparası 100000 × 1000 ppm × 30 gün / 1e6 = 3000.
		assert.Equal(t, int64(3000), num(charges[0].(map[string]any)["amount_kurus"]))
	}
}

func TestBackdatedCancellationPostsOnProcessingDay(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	today := caldate.Today(time.Now())
	var uid int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email='underwriter@guvenpolice.test'`).Scan(&uid))
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{UserID: uid, Role: auth.RoleUnderwriter})
	for _, paid := range []bool{true, false} {
		pol := billingPolicy(t, 30, 1, 100000)
		if paid {
			s, b, _ := postKey(t, account, policyPath(pol)+"/payments", map[string]any{"amount_kurus": 100000, "method": "CASH", "payment_date": pol["start_date"]}, fmt.Sprintf("early-date-%v", pol["id"]))
			require.Equal(t, http.StatusCreated, s, b)
		}
		out, err := policy.NewChangeService(pool, func() time.Time { return today }, 1000).Cancel(ctx, num(pol["id"]), policy.CancelRequest{EffectiveDate: day(today, -15), Reason: "UNDERWRITER"})
		require.NoError(t, err)
		if paid {
			assert.Positive(t, out.CashRefundKurus)
			var at time.Time
			require.NoError(t, pool.QueryRow(ctx, `SELECT paid_at FROM payments WHERE policy_id=$1 AND kind='REFUND'`, num(pol["id"])).Scan(&at))
			assert.Equal(t, caldate.Format(today), caldate.Format(at))
		}
		var bad, entries int
		require.NoError(t, pool.QueryRow(ctx, `
   SELECT count(*) FILTER (WHERE entry_date <> $2), count(*) FROM journal_entries e
    WHERE (e.ref_type='ENDORSEMENT' AND e.ref_id IN (SELECT id FROM endorsements WHERE policy_id=$1))
       OR (e.ref_type='REFUND' AND e.ref_id IN (SELECT id FROM payments WHERE policy_id=$1))
       OR (e.ref_type='LATE_INTEREST' AND e.ref_id IN (
          SELECT c.id FROM late_interest_charges c JOIN installments i ON i.id=c.installment_id
          JOIN installment_plans pl ON pl.id=i.plan_id WHERE pl.policy_id=$1))`, num(pol["id"]), today).Scan(&bad, &entries))
		assert.Zero(t, bad)
		assert.Equal(t, 2, entries)
	}
}

func TestFraudReviewCanReturnToAppraisal(t *testing.T) {
	defer assertBillingInvariant(t)
	adjuster, appraiser, account := tokenFor(t, "hasar@guvenpolice.test"), tokenFor(t, "eksper@guvenpolice.test"), tokenFor(t, "muhasebe@guvenpolice.test")
	pol := claimPolicy(t)
	c := reportClaim(t, adjuster, pol, "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
	c = transitionClaim(t, adjuster, c, "TRIAGED")
	var actor int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email='hasar@guvenpolice.test'`).Scan(&actor))
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{UserID: actor, Role: auth.RoleClaimsAdjuster})
	require.NoError(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		result, err := claim.Transition(ctx, tx, num(c["id"]), "FRAUD_REVIEW", auth.Principal{UserID: actor, Role: auth.RoleClaimsAdjuster}, "inceleme", int(num(c["row_version"])), claim.TransitionOptions{At: time.Now(), SLA: workflow.DefaultConfig()})
		c["row_version"] = float64(result.RowVersion)
		return err
	}))
	s, b := newClient(t).do(http.MethodPost, claimPath(c)+"/transition", adjuster, map[string]any{"to_status": "ADJUSTMENT", "expected_row_version": c["row_version"]})
	require.Equal(t, http.StatusConflict, s, b)
	assert.Equal(t, "APPRAISAL_REQUIRED", errCode(b))
	c = transitionClaim(t, adjuster, c, "APPRAISAL")
	s, b = newClient(t).do(http.MethodPost, claimPath(c)+"/appraisals", appraiser, map[string]any{"damage_amount_kurus": 100000, "inspected_at": time.Now().UTC().Format(time.RFC3339)})
	require.Equal(t, http.StatusCreated, s, b)
	c = transitionClaim(t, adjuster, c, "ADJUSTMENT")
	s, b, _ = postKey(t, adjuster, claimPath(c)+"/settlement/approve", nil, fmt.Sprintf("review-approve-%v", c["id"]))
	require.Equal(t, http.StatusCreated, s, b)
	s, b, _ = postKey(t, account, claimPath(c)+"/pay", nil, fmt.Sprintf("review-pay-%v", c["id"]))
	require.Equal(t, http.StatusOK, s, b)
}

func TestSeedPoliciesUsePricingAndValidPlates(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
  SELECT v.tariff_version_id,v.risk_data,v.annual_premium_kurus,v.vehicle_plate
    FROM policies p JOIN policy_versions v ON v.policy_id=p.id AND v.version_no=1
   WHERE p.policy_no IN ('GP-DEMO-OVERDUE-A','GP-DEMO-OVERDUE-B')`)
	require.NoError(t, err)
	type item struct {
		tid     int64
		risk    []byte
		premium int64
		plate   string
	}
	samples, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (item, error) {
		var v item
		err := r.Scan(&v.tid, &v.risk, &v.premium, &v.plate)
		return v, err
	})
	require.NoError(t, err)
	require.Len(t, samples, 2)
	for _, v := range samples {
		_, ok := policy.NormalizePlate(v.plate)
		assert.True(t, ok)
		var risk policy.RiskData
		require.NoError(t, json.Unmarshal(v.risk, &risk))
		tr, err := tariff.LoadByID(ctx, pool, v.tid)
		require.NoError(t, err)
		result, err := pricing.Calculate(tr, pricing.Input{Product: pricing.Kasko, Risk: risk.Risk, Coverages: []string{"KASKO_ANA"}})
		require.NoError(t, err)
		assert.Equal(t, result.TotalKurus, v.premium)
	}
	var workshops int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM workshops WHERE name LIKE 'Kurgusal %'`).Scan(&workshops))
	assert.Equal(t, 3, workshops)
}

func TestPaymentDateBeforePolicyAndReadOnlySnapshots(t *testing.T) {
	defer assertBillingInvariant(t)
	account, agent := tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "acente@guvenpolice.test")
	pol := issuePolicy(t, agent)
	yesterday := day(caldate.Today(time.Now()), -1)
	s, b, _ := postKey(t, account, policyPath(pol)+"/payments", map[string]any{"amount_kurus": 1, "method": "CASH", "payment_date": yesterday}, fmt.Sprintf("too-early-%v", pol["id"]))
	require.Equal(t, http.StatusUnprocessableEntity, s, b)
	assert.Equal(t, "PAYMENT_DATE_BEFORE_POLICY", errCode(b))
	payBilling(t, account, pol, 1, "before-start")
	gate, err := pool.Begin(context.Background())
	require.NoError(t, err)
	defer gate.Rollback(context.Background())
	_, err = gate.Exec(context.Background(), `SELECT id FROM policies WHERE id=$1 FOR UPDATE`, num(pol["id"]))
	require.NoError(t, err)
	result := make(chan int, 1)
	go func() {
		s, _ := get(t, account, policyPath(pol)+"/billing")
		if s != http.StatusOK {
			result <- s
			return
		}
		s, _ = newClient(t).do(http.MethodPost, policyPath(pol)+"/endorsements/preview", agent, map[string]any{"effective_date": day(dateOf(t, pol["start_date"]), 10), "vehicle_group": "D"})
		if s != http.StatusOK {
			result <- s
			return
		}
		s, _ = newClient(t).do(http.MethodPost, policyPath(pol)+"/cancel/preview", agent, map[string]any{"effective_date": day(dateOf(t, pol["start_date"]), 20), "reason": "CUSTOMER_REQUEST"})
		result <- s
	}()
	select {
	case s := <-result:
		assert.Equal(t, http.StatusOK, s)
	case <-time.After(2 * time.Second):
		t.Error("salt okunur yollar poliçe satırı kilidini beklememeli")
	}
	require.NoError(t, gate.Rollback(context.Background()))
}
