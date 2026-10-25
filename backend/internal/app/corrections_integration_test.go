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

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
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
			require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM late_interest_charges c JOIN installments i ON i.id=c.installment_id JOIN installment_plans pl ON pl.id=i.plan_id WHERE pl.policy_id=$1`, num(pol["id"])).Scan(&before))
			s, r := newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, nil)
			require.Equal(t, http.StatusOK, s, r)
			assert.Equal(t, "ACTIVE", billingDetail(t, account, pol)["policy_status"])
			var after int
			require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM late_interest_charges c JOIN installments i ON i.id=c.installment_id JOIN installment_plans pl ON pl.id=i.plan_id WHERE pl.policy_id=$1`, num(pol["id"])).Scan(&after))
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
