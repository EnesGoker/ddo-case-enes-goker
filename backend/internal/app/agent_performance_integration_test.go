//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/report"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentPerformanceAccessAndGoldenRows(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := context.Background()
	type agency struct {
		id, userID int64
		email      string
	}
	makeAgent := func(profile bool) agency {
		var a agency
		a.email = fmt.Sprintf("b1-%d@test.invalid", time.Now().UnixNano())
		require.NoError(t, pool.QueryRow(ctx, `
            INSERT INTO users(email,password_hash,full_name,role)
            SELECT $1,password_hash,'B1 Test Acentesi','AGENT'
              FROM users WHERE email='acente@guvenpolice.test' RETURNING id`, a.email).Scan(&a.userID))
		if profile {
			require.NoError(t, pool.QueryRow(ctx, `
                INSERT INTO agents(user_id,agency_name) VALUES($1,$2) RETURNING id`, a.userID, a.email).Scan(&a.id))
		}
		return a
	}
	a, b, empty, noProfile := makeAgent(true), makeAgent(true), makeAgent(true), makeAgent(false)
	from := dateOf(t, "2026-09-01")
	to := from.AddDate(0, 0, 1)
	issue := func(agentID *int64, premium int64) policy.Issued {
		var tariff int64
		require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM tariff_versions WHERE product='KONUT' AND version_no=1`).Scan(&tariff))
		var issued policy.Issued
		require.NoError(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			var err error
			issued, err = policy.Issue(ctx, tx, policy.IssueInput{
				ActorID: a.userID, PolicyNoOverride: fmt.Sprintf("B1-%d", time.Now().UnixNano()), Product: pricing.Konut,
				CustomerID: customerID(t, ayse), AgentID: agentID, TariffVersionID: tariff,
				StartDate: from, EndDate: caldate.AddYear(from), RiskData: []byte(`{"ncd_level":2,"region_code":"34"}`),
				AnnualPremium: premium, AnnualLimitKurus: 10000000,
				Coverages: []policy.CoverageTerm{{Code: "YANGIN", LimitKurus: 10000000, DeductibleType: pricing.DeductibleNone, PremiumKurus: premium}},
			})
			return err
		}))
		return issued
	}
	pol := issue(&a.id, 100000)
	issue(&b.id, 200000)
	issue(nil, 300000)
	// Cash receipt/refund changes the receivable/cash ledger, never written premium.
	payer := principalContext(t, "muhasebe@guvenpolice.test", auth.RoleAccounting)
	payments := payment.NewService(pool, func() time.Time { return from }, 1000)
	paymentAmount, refundAmount := int64(10000), int64(5000)
	paid, err := payments.Pay(payer, pol.ID, payment.PayRequest{AmountKurus: &paymentAmount, Method: "TRANSFER"})
	require.NoError(t, err)
	_, err = payments.Refund(payer, paid.ID, payment.RefundRequest{AmountKurus: &refundAmount, Reason: "B1 nakit iade primi değiştirmez"})
	require.NoError(t, err)
	path := "/api/v1/reports/agents?from=&to="
	for _, role := range []struct {
		email  string
		status int
	}{
		{"", http.StatusUnauthorized}, {"musteri", http.StatusForbidden}, {"eksper", http.StatusForbidden},
		{"hasar", http.StatusForbidden}, {"underwriter", http.StatusOK}, {"muhasebe", http.StatusOK},
	} {
		token := ""
		if role.email != "" {
			token = tokenFor(t, role.email+"@guvenpolice.test")
		}
		status, body := get(t, token, path)
		require.Equal(t, role.status, status, body)
		if role.status == http.StatusOK {
			require.Contains(t, body, "total")
			found := map[string]map[string]any{}
			for _, item := range body["items"].([]any) {
				row := item.(map[string]any)
				found[row["key"].(string)] = row
			}
			require.Contains(t, found, fmt.Sprint(a.id))
			require.Contains(t, found, fmt.Sprint(b.id))
			require.Contains(t, found, "direct")
			require.Contains(t, found, fmt.Sprint(empty.id))
			assert.Equal(t, int64(100000), num(found[fmt.Sprint(a.id)]["written_premium_kurus"]))
			assert.Equal(t, int64(200000), num(found[fmt.Sprint(b.id)]["written_premium_kurus"]))
			assert.Equal(t, int64(0), num(found[fmt.Sprint(empty.id)]["policy_count"]))
			assert.Nil(t, found[fmt.Sprint(empty.id)]["loss_ratio_bp"])
		}
	}
	for _, own := range []struct {
		agency  agency
		premium int64
	}{{a, 100000}, {b, 200000}, {empty, 0}} {
		token := tokenFor(t, own.agency.email)
		for _, query := range []string{"from=&to=", fmt.Sprintf("from=&to=&agent_id=%d&scope=all&group_by=agent", b.id), "from=&to=&agent_id=direct&include_total=true"} {
			status, body := get(t, token, "/api/v1/reports/agents?"+query)
			require.Equal(t, http.StatusOK, status, body)
			require.NotContains(t, body, "total")
			items := body["items"].([]any)
			require.Len(t, items, 1)
			row := items[0].(map[string]any)
			assert.Equal(t, fmt.Sprint(own.agency.id), row["key"])
			assert.Equal(t, own.premium, num(row["written_premium_kurus"]))
			assert.NotEqual(t, "direct", row["key"])
		}
	}
	status, body := get(t, tokenFor(t, noProfile.email), path)
	assert.Equal(t, http.StatusForbidden, status, body)
	for _, query := range []string{"from=bad", "to=2026-02-30", "from=2026-10-26&to=2026-10-26", "from=2026-10-27&to=2026-10-26"} {
		status, body := get(t, tokenFor(t, a.email), "/api/v1/reports/agents?"+query)
		assert.Equal(t, http.StatusUnprocessableEntity, status, body)
		assert.Equal(t, "INVALID_PERIOD", errCode(body))
	}
	// Exact [) issue-date boundary for an otherwise isolated agency.
	for _, period := range []struct {
		from, to       time.Time
		premium, count int64
	}{
		{from, to, 100000, 1}, {from.AddDate(0, 0, -1), from, 0, 0}, {to, to.AddDate(0, 0, 1), 0, 0},
	} {
		out, err := report.ReadAgentPerformance(ctx, pool, report.AgentFilter{From: &period.from, To: &period.to, AgentID: &a.id})
		require.NoError(t, err)
		require.Len(t, out.Items, 1)
		assert.Equal(t, period.premium, out.Items[0].WrittenPremium)
		assert.Equal(t, period.count, out.Items[0].PolicyCount)
		assert.Nil(t, out.Total)
	}
	defaults, err := report.AgentPeriod(nil, time.Now())
	require.NoError(t, err)
	narrowFrom, narrowTo := from, to
	for _, period := range []report.AgentFilter{{}, defaults, {From: &narrowFrom, To: &narrowTo}} {
		pane, err := report.ReadAgentPerformance(ctx, pool, period)
		require.NoError(t, err)
		legacy, err := report.ReadProduction(ctx, pool, report.Filter{From: period.From, To: period.To, GroupBy: "agent"})
		require.NoError(t, err)
		assert.Equal(t, legacy.Total, *pane.Total)
	}
}
