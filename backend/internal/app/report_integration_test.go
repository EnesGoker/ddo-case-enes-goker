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
	"github.com/EnesGoker/guvenpolice/backend/internal/claim"
	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/lossratio"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/report"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductionReportGoldenLedgerPeriodsAndGroups(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := context.Background()
	date := func(s string) time.Time { d, e := caldate.Parse(s); require.NoError(t, e); return d }
	from, to := date("2025-07-01"), date("2025-09-01")
	var agent1, agent2, actor int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT id,user_id FROM agents ORDER BY id LIMIT 1`).Scan(&agent1, &actor))
	require.NoError(t, pool.QueryRow(ctx, `
  INSERT INTO users(email,password_hash,full_name,role)
  SELECT $1,password_hash,'Kurgusal İkinci Acente','AGENT' FROM users WHERE id=$2 RETURNING id`, fmt.Sprintf("report-agent-%d@test.invalid", time.Now().UnixNano()), actor).Scan(&agent2))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO agents(user_id,agency_name) VALUES($1,'Kurgusal Rapor Acentesi') RETURNING id`, agent2).Scan(&agent2))
	// Before/after deltas isolate the golden data from other integration scenarios.
	baseline := map[string]report.Production{}
	for _, group := range []string{"month", "product", "agent"} {
		v, e := report.ReadProduction(ctx, pool, report.Filter{From: &from, To: &to, GroupBy: group})
		require.NoError(t, e)
		baseline[group] = v
	}
	issue := func(product pricing.Product, start string, premium int64, agent *int64) policy.Issued {
		var tariff int64
		require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM tariff_versions WHERE product=$1 AND version_no=1`, string(product)).Scan(&tariff))
		plate := fixturePlate()
		var pplate *string
		code := "YANGIN"
		risk := []byte(`{"ncd_level":2,"region_code":"34"}`)
		if product == pricing.Kasko {
			pplate = &plate
			code = "KASKO_ANA"
			risk = []byte(fmt.Sprintf(`{"driver_age":38,"ncd_level":2,"region_code":"34","vehicle_group":"C","plate":%q}`, plate))
		}
		var out policy.Issued
		require.NoError(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			var err error
			out, err = policy.Issue(ctx, tx, policy.IssueInput{ActorID: actor, PolicyNoOverride: fmt.Sprintf("GOLD-%d", time.Now().UnixNano()), Product: product, CustomerID: customerID(t, ayse), AgentID: agent, TariffVersionID: tariff, StartDate: date(start), EndDate: caldate.AddYear(date(start)), Plate: pplate, RiskData: risk, AnnualPremium: premium, AnnualLimitKurus: 10000000, Coverages: []policy.CoverageTerm{{Code: code, LimitKurus: 10000000, DeductibleType: pricing.DeductibleNone, PremiumKurus: premium}}})
			return err
		}))
		return out
	}
	// KASKO C: 1,000,000*.84*1.15*1.15=1,110,900.
	// July 2 D: 1,352,400; +(241,500*364/365)=+240,838.
	// August 1 B: 966,000; -(386,400*334/365)=-353,582.
	// KONUT 20,000 cancelled August 1: -(20,000*334/365)=-18,301.
	// July=1,371,738 (2 issues), August=-341,883 (1 issue), total=1,029,855.
	// August claim approval 50,000; total ratio round(50,000*10,000/1,029,855)=486 bp.
	a := issue(pricing.Kasko, "2025-07-01", 1110900, &agent1)
	b := issue(pricing.Konut, "2025-07-01", 20000, &agent2)
	issue(pricing.Konut, "2025-08-01", 30000, nil)
	uwctx := principalContext(t, "underwriter@guvenpolice.test", auth.RoleUnderwriter)
	changes := policy.NewChangeService(pool, func() time.Time { return date("2025-07-15") }, 1000)
	plus, err := changes.VehicleChange(uwctx, a.ID, policy.VehicleChangeRequest{EffectiveDate: "2025-07-02", VehicleGroup: "D"})
	require.NoError(t, err)
	assert.Equal(t, int64(240838), plus.Endorsement.PremiumDelta)
	changes = policy.NewChangeService(pool, func() time.Time { return date("2025-08-15") }, 1000)
	minus, err := changes.VehicleChange(uwctx, a.ID, policy.VehicleChangeRequest{EffectiveDate: "2025-08-01", VehicleGroup: "B"})
	require.NoError(t, err)
	assert.Equal(t, int64(-353582), minus.Endorsement.PremiumDelta)
	cancelled, err := changes.Cancel(uwctx, b.ID, policy.CancelRequest{EffectiveDate: "2025-08-01", Reason: "UNDERWRITER"})
	require.NoError(t, err)
	assert.Equal(t, int64(-18301), cancelled.Endorsement.PremiumDelta)
	adjctx := principalContext(t, "hasar@guvenpolice.test", auth.RoleClaimsAdjuster)
	svc := claim.NewService(pool, func() time.Time { return date("2025-08-15") }, fraud.DefaultConfig(), workflow.DefaultConfig())
	amount := int64(50000)
	c, err := svc.Report(adjctx, claim.ReportRequest{PolicyID: a.ID, CoverageCode: "KASKO_ANA", LossDate: "2025-07-03", Description: "Altın rapor hasarı", ClaimedAmountKurus: &amount})
	require.NoError(t, err)
	for _, to := range []string{"TRIAGED", "APPRAISAL"} {
		c, err = svc.ChangeStatus(adjctx, c.ID, claim.TransitionRequest{ToStatus: to, Note: "altın veri", ExpectedRowVersion: c.RowVersion})
		require.NoError(t, err)
	}
	_, err = svc.Appraise(principalContext(t, "eksper@guvenpolice.test", auth.RoleAppraiser), c.ID, claim.AppraisalRequest{DamageAmountKurus: &amount, InspectedAt: "2025-08-15T10:00:00Z"})
	require.NoError(t, err)
	c, err = svc.ChangeStatus(adjctx, c.ID, claim.TransitionRequest{ToStatus: "ADJUSTMENT", Note: "altın veri", ExpectedRowVersion: c.RowVersion})
	require.NoError(t, err)
	_, err = svc.Approve(adjctx, c.ID)
	require.NoError(t, err)
	expected := map[string]map[string]int64{
		"month":   {"2025-07": 1371738, "2025-08": -341883},
		"product": {"KASKO": 998156, "KONUT": 31699},
		"agent":   {fmt.Sprint(agent1): 998156, fmt.Sprint(agent2): 1699, "unassigned": 30000},
	}
	for _, group := range []string{"month", "product", "agent"} {
		out, e := report.ReadProduction(ctx, pool, report.Filter{From: &from, To: &to, GroupBy: group})
		require.NoError(t, e)
		prior := baseline[group]
		assert.Equal(t, int64(1029855), out.Total.WrittenPremium-prior.Total.WrittenPremium)
		assert.Equal(t, int64(50000), out.Total.IncurredClaims-prior.Total.IncurredClaims)
		assert.Equal(t, int64(3), out.Total.PolicyCount-prior.Total.PolicyCount)
		old := map[string]int64{}
		for _, r := range prior.Items {
			old[r.Key] = r.WrittenPremium
		}
		var premium, claims, count int64
		for _, r := range out.Items {
			premium += r.WrittenPremium
			claims += r.IncurredClaims
			count += r.PolicyCount
			if want, ok := expected[group][r.Key]; ok {
				assert.Equal(t, want, r.WrittenPremium-old[r.Key], group+"/"+r.Key)
			}
			assert.Equal(t, lossratio.LossRatioBP(r.IncurredClaims, r.WrittenPremium), r.LossRatioBP)
		}
		assert.Equal(t, out.Total.WrittenPremium, premium)
		assert.Equal(t, out.Total.IncurredClaims, claims)
		assert.Equal(t, out.Total.PolicyCount, count)
	}
	boundary := date("2025-08-01")
	july, err := report.ReadProduction(ctx, pool, report.Filter{From: &from, To: &boundary, GroupBy: "month"})
	require.NoError(t, err)
	prior := baseline["month"]
	var priorJuly int64
	for _, r := range prior.Items {
		if r.Key == "2025-07" {
			priorJuly = r.WrittenPremium
		}
	}
	assert.Equal(t, int64(1371738), july.Total.WrittenPremium-priorJuly)
	// Unlimited report must reconcile exactly to accounts, including all other fixtures.
	var ledgerPremium, ledgerClaims int64
	require.NoError(t, pool.QueryRow(ctx, `
  SELECT coalesce(sum(l.credit_kurus-l.debit_kurus) FILTER (WHERE a.code='4000'),0),
         coalesce(sum(l.debit_kurus-l.credit_kurus) FILTER (WHERE a.code='5000'),0)
    FROM journal_lines l JOIN accounts a ON a.id=l.account_id`).Scan(&ledgerPremium, &ledgerClaims))
	for _, group := range []string{"month", "product", "agent"} {
		out, e := report.ReadProduction(ctx, pool, report.Filter{GroupBy: group})
		require.NoError(t, e)
		assert.Equal(t, ledgerPremium, out.Total.WrittenPremium)
		assert.Equal(t, ledgerClaims, out.Total.IncurredClaims)
	}
	defaults, e := report.AgentPeriod(nil, time.Now())
	require.NoError(t, e)
	for _, period := range []report.AgentFilter{{}, defaults, {From: &from, To: &to}, {From: &from, To: &boundary}} {
		pane, e := report.ReadAgentPerformance(ctx, pool, period)
		require.NoError(t, e)
		legacy, e := report.ReadProduction(ctx, pool, report.Filter{From: period.From, To: period.To, GroupBy: "agent"})
		require.NoError(t, e)
		assert.Equal(t, legacy.Total, *pane.Total, "pano = ADR-18; zeyil/iptal/onay dahil")
	}

}

func TestProductionReportPeriodValidationAndRBAC(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	for _, query := range []string{"from=bad", "to=2026-02-30", "from=2026-10-25&to=2026-10-25", "from=2026-10-26&to=2026-10-25"} {
		s, b := get(t, account, "/api/v1/reports/production?"+query)
		assert.Equal(t, http.StatusUnprocessableEntity, s, b)
		assert.Equal(t, "INVALID_PERIOD", errCode(b))
	}
	for _, email := range []string{"muhasebe@guvenpolice.test", "underwriter@guvenpolice.test"} {
		s, b := get(t, tokenFor(t, email), "/api/v1/reports/production")
		assert.Equal(t, http.StatusOK, s, b)
		assert.Contains(t, b, "total")
	}
	for _, email := range []string{"musteri@guvenpolice.test", "acente@guvenpolice.test", "hasar@guvenpolice.test", "eksper@guvenpolice.test"} {
		s, b := get(t, tokenFor(t, email), "/api/v1/reports/production")
		assert.Equal(t, http.StatusForbidden, s, b)
	}
}
