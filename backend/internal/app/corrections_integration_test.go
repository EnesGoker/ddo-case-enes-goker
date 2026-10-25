//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
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
