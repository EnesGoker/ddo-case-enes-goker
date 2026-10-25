//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

func billingPolicy(t *testing.T, days, count int, premium int64) map[string]any {
	t.Helper()
	ctx := context.Background()
	var tariffID, agentID, actorID int64
	require.NoError(t, pool.QueryRow(ctx, `
  SELECT t.id, a.id, a.user_id FROM tariff_versions t CROSS JOIN agents a
   WHERE t.product = 'KASKO' AND t.version_no = 2 ORDER BY a.id LIMIT 1`).Scan(&tariffID, &agentID, &actorID))
	start := caldate.Today(time.Now()).AddDate(0, 0, -days)
	var out policy.Issued
	require.NoError(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var err error
		out, err = policy.Issue(ctx, tx, policy.IssueInput{
			PolicyNoOverride: fmt.Sprintf("TEST-BILL-%d", time.Now().UnixNano()), ActorID: actorID,
			Product: pricing.Kasko, CustomerID: customerID(t, ayse), AgentID: &agentID,
			TariffVersionID: tariffID, StartDate: start, EndDate: caldate.AddYear(start),
			Plate: strptr("34ABC123"), RiskData: []byte(`{"driver_age":38,"ncd_level":2,"region_code":"34","vehicle_group":"C","plate":"34ABC123"}`),
			AnnualPremium: premium, InstallmentCount: count, AnnualLimitKurus: 180000000,
			Coverages: []policy.CoverageTerm{{Code: "KASKO_ANA", LimitKurus: 150000000, DeductibleType: pricing.DeductibleNone, PremiumKurus: premium}},
		})
		return err
	}))
	return map[string]any{"id": float64(out.ID), "start_date": out.StartDate, "end_date": out.EndDate}
}

func assertBillingInvariant(t *testing.T) {
	t.Helper()
	b, err := ledger.ReadTrialBalance(context.Background(), pool)
	require.NoError(t, err)
	assert.True(t, b.Balanced, "%+v", b)
	assert.Zero(t, b.EmptyEntries)
	assert.Zero(t, b.UnbalancedEntries)
	assert.True(t, b.ReceivableReconciliation.Matches, "%+v", b.ReceivableReconciliation)
	var invalid int
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT count(*) FROM (
   SELECT pl.id FROM installment_plans pl JOIN installments i ON i.plan_id = pl.id
   GROUP BY pl.id HAVING pl.total_kurus <> sum(CASE WHEN i.status <> 'CANCELLED' THEN i.amount_kurus ELSE 0 END)
  ) bad`).Scan(&invalid))
	assert.Zero(t, invalid, "plan toplamları güncel")
}

func billingDetail(t *testing.T, token string, pol map[string]any) map[string]any {
	t.Helper()
	s, b := get(t, token, policyPath(pol)+"/billing")
	require.Equal(t, http.StatusOK, s, b)
	return b
}

func payBilling(t *testing.T, token string, pol map[string]any, amount int64, suffix string) map[string]any {
	t.Helper()
	s, b, _ := postKey(t, token, policyPath(pol)+"/payments", map[string]any{"amount_kurus": amount, "method": "TRANSFER"}, fmt.Sprintf("bill-%v-%s", pol["id"], suffix))
	require.Equal(t, http.StatusCreated, s, b)
	return b
}

func TestBillingIssueThreeInstallments(t *testing.T) {
	defer assertBillingInvariant(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	s, pol, _ := postKey(t, agent, fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"]), map[string]any{"installment_count": 3}, fmt.Sprintf("bill-convert-%v", q["id"]))
	require.Equal(t, http.StatusCreated, s, pol)
	b := billingDetail(t, agent, pol)
	rows := b["installments"].([]any)
	require.Len(t, rows, 3)
	total := num(pol["annual_premium_kurus"])
	var sum int64
	for i, r := range rows {
		row := r.(map[string]any)
		sum += num(row["amount_kurus"])
		assert.Equal(t, caldate.Format(caldate.AddMonths(dateOf(t, pol["start_date"]), i)), row["due_date"])
	}
	assert.Equal(t, total, sum)
	assert.Equal(t, total/3+total%3, num(rows[0].(map[string]any)["amount_kurus"]))
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT count(*) FROM journal_entries WHERE ref_type = 'POLICY_ISSUE' AND ref_id = $1`, num(pol["id"])).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestBillingInterestAllocationRefundAndDates(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	pol := billingPolicy(t, 20, 3, 100000)
	p := payBilling(t, account, pol, 40000, "partial")
	b := billingDetail(t, account, pol)
	rows := b["installments"].([]any)
	// 100000 / 3 = 33334,33333,33333; faiz 33334*1000*20/1e6=666.68→667.
	// 40000 = 667 faiz + 33334 ilk anapara + 5999 ikinci anapara.
	assert.Equal(t, int64(667), num(b["interest"].([]any)[0].(map[string]any)["amount_kurus"]))
	assert.Equal(t, "PAID", rows[0].(map[string]any)["status"])
	assert.Equal(t, int64(5999), num(rows[1].(map[string]any)["paid_kurus"]))
	s, r, _ := postKey(t, account, fmt.Sprintf("/api/v1/payments/%v/refund", p["id"]), map[string]any{"amount_kurus": 1000, "reason": "kısmi iade"}, fmt.Sprintf("refund-%v", p["id"]))
	require.Equal(t, http.StatusCreated, s, r)
	b = billingDetail(t, account, pol)
	assert.Equal(t, int64(4999), num(b["installments"].([]any)[1].(map[string]any)["paid_kurus"]))
	assert.Len(t, b["interest"], 1, "iade aynı gün ek faiz yaratmaz")
	s, r, _ = postKey(t, account, policyPath(pol)+"/payments", map[string]any{"amount_kurus": 1, "method": "CASH", "payment_date": day(caldate.Today(time.Now()), -1)}, fmt.Sprintf("date-%v", pol["id"]))
	assert.Equal(t, http.StatusUnprocessableEntity, s, r)
	assert.Equal(t, "PAYMENT_DATE_BEFORE_ACCRUAL", errCode(r))
	// Tüm anaparayı ödeyip faiz tahsisini geri aç: son tahsisten geriye gidilir.
	p2 := payBilling(t, account, pol, num(b["open_balance_kurus"]), "close")
	s, r, _ = postKey(t, account, fmt.Sprintf("/api/v1/payments/%v/refund", p2["id"]), map[string]any{"amount_kurus": 100, "reason": "yeniden aç"}, fmt.Sprintf("reopen-%v", p2["id"]))
	require.Equal(t, http.StatusCreated, s, r)
	assert.Equal(t, int64(100), num(billingDetail(t, account, pol)["open_balance_kurus"]))
}

func TestBillingPaymentIdempotency(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	pol := billingPolicy(t, 0, 3, 30000)
	path := policyPath(pol) + "/payments"
	key := fmt.Sprintf("five-%v", pol["id"])
	body := map[string]any{"amount_kurus": 5000, "method": "CARD"}
	codes := runParallel(t, 5, func(i int) int { s, _, _ := postKey(t, account, path, body, key); return s })
	assert.Equal(t, 5, count(codes, http.StatusCreated)+count(codes, http.StatusConflict), codes)
	assert.GreaterOrEqual(t, count(codes, http.StatusCreated), 1, codes)
	s, p, h := postKey(t, account, path, body, key)
	require.Equal(t, http.StatusCreated, s, p)
	assert.Equal(t, "true", h.Get("Idempotent-Replayed"))
	s, again, _ := postKey(t, account, path, body, key)
	require.Equal(t, http.StatusCreated, s)
	assert.Equal(t, p, again)
	b := billingDetail(t, account, pol)
	assert.Len(t, b["payments"], 1)
	assert.Equal(t, int64(25000), num(b["open_balance_kurus"]))
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT count(*) FROM journal_entries WHERE ref_type = 'PAYMENT' AND ref_id = $1`, num(p["id"])).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestConcurrentPayments(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	pol := billingPolicy(t, 0, 1, 30000)
	gate, err := pool.Begin(context.Background())
	require.NoError(t, err)
	defer gate.Rollback(context.Background())
	rows, err := gate.Query(context.Background(), `
  SELECT i.id FROM installments i JOIN installment_plans pl ON pl.id = i.plan_id
   WHERE pl.policy_id = $1 FOR UPDATE OF i`, num(pol["id"]))
	require.NoError(t, err)
	_, err = pgx.CollectRows(rows, pgx.RowTo[int64])
	require.NoError(t, err)
	result := make(chan []int, 1)
	go func() {
		result <- runParallel(t, 2, func(i int) int {
			s, b, _ := postKey(t, account, policyPath(pol)+"/payments", map[string]any{"amount_kurus": 20000, "method": "CARD"}, fmt.Sprintf("race-%v-%d", pol["id"], i))
			t.Logf("ödeme %d: HTTP %d %v", i, s, b)
			if s == http.StatusUnprocessableEntity {
				assert.Equal(t, "OVERPAYMENT", errCode(b))
			}
			return s
		})
	}()
	// Normalde biri anapara UPDATE'inde, diğeri poliçe kilidinde bekler.
	// Poliçe kilidi mutasyonunda ikisi de eski bakiye ile anapara UPDATE'ine ulaşır.
	require.Eventually(t, func() bool {
		var waiting int
		err := pool.QueryRow(context.Background(), `
   SELECT count(*) FROM pg_stat_activity
    WHERE datname = current_database() AND wait_event_type = 'Lock'
      AND (query LIKE '%FROM policies p%' OR query LIKE '%UPDATE installments SET%')`).Scan(&waiting)
		return err == nil && waiting >= 2
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, gate.Commit(context.Background()))
	assert.ElementsMatch(t, []int{http.StatusCreated, http.StatusUnprocessableEntity}, <-result)
	assert.Equal(t, int64(10000), num(billingDetail(t, account, pol)["open_balance_kurus"]))
	var bad int
	require.NoError(t, pool.QueryRow(context.Background(), `
  SELECT count(*) FROM installments WHERE paid_kurus > amount_kurus`).Scan(&bad))
	assert.Zero(t, bad)
}

func TestConcurrentRefunds(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	pol := billingPolicy(t, 0, 1, 30000)
	p := payBilling(t, account, pol, 20000, "pay")
	codes := runParallel(t, 2, func(i int) int {
		s, b, _ := postKey(t, account, fmt.Sprintf("/api/v1/payments/%v/refund", p["id"]), map[string]any{"amount_kurus": 15000, "reason": "yarış"}, fmt.Sprintf("refund-race-%v-%d", p["id"], i))
		if s == http.StatusUnprocessableEntity {
			assert.Equal(t, "REFUND_EXCEEDS_PAYMENT", errCode(b))
		}
		return s
	})
	assert.ElementsMatch(t, []int{http.StatusCreated, http.StatusUnprocessableEntity}, codes)
	b := billingDetail(t, account, pol)
	assert.Equal(t, int64(25000), num(b["open_balance_kurus"]))
	assert.Len(t, b["payments"], 2)
}

func TestBillingRunSeedAndBoundary(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	p29, p30 := billingPolicy(t, 29, 3, 300000), billingPolicy(t, 30, 3, 300000)
	s, run := newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, nil)
	require.Equal(t, http.StatusOK, s, run)
	assert.Equal(t, "ACTIVE", billingDetail(t, account, p29)["policy_status"])
	assert.Equal(t, "CANCELLED", billingDetail(t, account, p30)["policy_status"])
	var aid, bid int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM policies WHERE policy_no = 'GP-DEMO-OVERDUE-A'`).Scan(&aid))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM policies WHERE policy_no = 'GP-DEMO-OVERDUE-B'`).Scan(&bid))
	_, a := get(t, account, fmt.Sprintf("/api/v1/policies/%d", aid))
	assert.Equal(t, "CANCELLED", a["status"])
	assert.Equal(t, "NON_PAYMENT", a["cancel_reason"])
	b := billingDetail(t, account, map[string]any{"id": float64(bid)})
	assert.Equal(t, "ACTIVE", b["policy_status"])
	interest := b["interest"].([]any)
	require.Len(t, interest, 1)
	assert.Equal(t, int64(2000), num(interest[0].(map[string]any)["amount_kurus"]), "100000×1000×20/1e6")
	s, again := newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, nil)
	require.Equal(t, http.StatusOK, s, again)
	assert.Zero(t, num(again["accrual_count"]))
	assert.Empty(t, again["cancelled"])
	s, bad := newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, map[string]any{"as_of": day(caldate.Today(time.Now()), 1)})
	assert.Equal(t, http.StatusUnprocessableEntity, s, bad)
}

func TestBillingRunSkipsLockedAndClaimGuard(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	pol := billingPolicy(t, 31, 3, 300000)
	gate, err := pool.Begin(context.Background())
	require.NoError(t, err)
	defer gate.Rollback(context.Background())
	_, err = gate.Exec(context.Background(), `SELECT id FROM policies WHERE id = $1 FOR UPDATE`, num(pol["id"]))
	require.NoError(t, err)
	s, b := newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, nil)
	require.Equal(t, http.StatusOK, s, b)
	assert.Contains(t, b["skipped"], map[string]any{"policy_id": pol["id"], "reason": "LOCKED"})
	require.NoError(t, gate.Commit(context.Background()))
	// Bugünkü hasar, dünkü sistem iptalini A4 korumasıyla engeller.
	pol = billingPolicy(t, 31, 3, 300000)
	reportClaim(t, tokenFor(t, "hasar@guvenpolice.test"), pol, "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
	s, b = newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, map[string]any{"as_of": day(caldate.Today(time.Now()), -1)})
	require.Equal(t, http.StatusOK, s, b)
	assert.Contains(t, b["skipped"], map[string]any{"policy_id": pol["id"], "reason": "CLAIMS_AFTER_EFFECTIVE_DATE"})
	assert.Equal(t, "ACTIVE", billingDetail(t, account, pol)["policy_status"])
}

func TestBillingPremiumAdjustmentsAndCashRefund(t *testing.T) {
	defer assertBillingInvariant(t)
	account, agent, uw := tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "acente@guvenpolice.test"), tokenFor(t, "underwriter@guvenpolice.test")
	pol := issuePolicy(t, agent)
	total := num(pol["annual_premium_kurus"])
	payBilling(t, account, pol, total, "full")
	start := dateOf(t, pol["start_date"])
	// Tam ödenmiş planda + zeyil yeni taksit açar; - zeyil en son açıktan düşer.
	body := map[string]any{"effective_date": day(start, 10), "vehicle_group": "D", "plate": "34ZZ999"}
	s, plus, _ := postKey(t, agent, policyPath(pol)+"/endorsements", body, fmt.Sprintf("bill-plus-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, plus)
	assert.Positive(t, num(plus["endorsement"].(map[string]any)["premium_delta_kurus"]))
	b := billingDetail(t, account, pol)
	assert.Len(t, b["installments"], 2)
	assert.Equal(t, int64(1), num(b["installment_count"]))
	body = map[string]any{"effective_date": day(start, 20), "vehicle_group": "A", "plate": "34ZZ998"}
	s, minus, _ := postKey(t, agent, policyPath(pol)+"/endorsements", body, fmt.Sprintf("bill-minus-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, minus)
	assert.Negative(t, num(minus["endorsement"].(map[string]any)["premium_delta_kurus"]))
	assertBillingInvariant(t)
	// Ayrı poliçede peşin ödeme: başlangıçta UW iptalinde primin tamamı nakit iade.
	pol = issuePolicy(t, agent)
	total = num(pol["annual_premium_kurus"])
	payment := payBilling(t, account, pol, total, "full")
	cancel := map[string]any{"effective_date": pol["start_date"], "reason": "UNDERWRITER"}
	s, preview := newClient(t).do(http.MethodPost, policyPath(pol)+"/cancel/preview", uw, cancel)
	require.Equal(t, http.StatusOK, s, preview)
	assert.Equal(t, total, num(preview["cash_refund_kurus"]))
	s, result, _ := postKey(t, uw, policyPath(pol)+"/cancel", cancel, fmt.Sprintf("bill-cancel-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, result)
	assert.Equal(t, total, num(result["cash_refund_kurus"]))
	b = billingDetail(t, account, pol)
	assert.Zero(t, num(b["open_balance_kurus"]))
	assert.Len(t, b["payments"], 2)
	assert.Len(t, b["allocations"], 1, "otomatik nakit iadenin tahsisi yok")
	s, result, _ = postKey(t, account, fmt.Sprintf("/api/v1/payments/%v/refund", payment["id"]), map[string]any{"amount_kurus": 1, "reason": "iptal sonrası"}, fmt.Sprintf("inactive-refund-%v", payment["id"]))
	assert.Equal(t, http.StatusConflict, s, result)
	assert.Equal(t, "REFUND_REQUIRES_ACTIVE_POLICY", errCode(result))
	// Hiç ödenmemiş başlangıç iptalinde plan toplamı 0: 00005 kısıtı gerçekten kullanılır.
	pol = issuePolicy(t, agent)
	cancel = map[string]any{"effective_date": pol["start_date"], "reason": "UNDERWRITER"}
	s, result, _ = postKey(t, uw, policyPath(pol)+"/cancel", cancel, fmt.Sprintf("zero-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, result)
	assert.Zero(t, num(billingDetail(t, account, pol)["total_kurus"]))
}

func TestBillingRBACAndLedger(t *testing.T) {
	defer assertBillingInvariant(t)
	account, agent, customer := tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "acente@guvenpolice.test"), tokenFor(t, "musteri@guvenpolice.test")
	pol := billingPolicy(t, 0, 1, 30000)
	s, b, _ := postKey(t, agent, policyPath(pol)+"/payments", map[string]any{"amount_kurus": 1, "method": "CARD"}, fmt.Sprintf("agent-pay-%v", pol["id"]))
	assert.Equal(t, http.StatusForbidden, s, b)
	p := payBilling(t, customer, pol, 1000, "customer")
	s, b, _ = postKey(t, customer, fmt.Sprintf("/api/v1/payments/%v/refund", p["id"]), map[string]any{"amount_kurus": 1, "reason": "yetki"}, fmt.Sprintf("cust-refund-%v", p["id"]))
	assert.Equal(t, http.StatusForbidden, s, b)
	s, b = newClient(t).do(http.MethodPost, "/api/v1/billing/run", customer, nil)
	assert.Equal(t, http.StatusForbidden, s, b)
	var otherID int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM customers WHERE national_id = '10000000002'`).Scan(&otherID))
	_, err := pool.Exec(context.Background(), `UPDATE policies SET customer_id = $2 WHERE id = $1`, num(pol["id"]), otherID)
	require.NoError(t, err)
	s, b, _ = postKey(t, customer, policyPath(pol)+"/payments", map[string]any{"amount_kurus": 1, "method": "CARD"}, fmt.Sprintf("other-pay-%v", pol["id"]))
	assert.Equal(t, http.StatusNotFound, s, b)
	s, b = get(t, account, "/api/v1/ledger/trial-balance")
	require.Equal(t, http.StatusOK, s, b)
	assert.Equal(t, true, b["balanced"])
	assert.Equal(t, true, b["receivable_reconciliation"].(map[string]any)["matches"])
	s, b = get(t, account, "/api/v1/journal-entries?limit=1")
	require.Equal(t, http.StatusOK, s, b)
	require.Len(t, b["items"], 1)
	assert.Len(t, b["items"].([]any)[0].(map[string]any)["lines"], 2)
	cursor := b["next_cursor"].(string)
	require.NotEmpty(t, cursor)
	s, b = get(t, account, "/api/v1/journal-entries?limit=1&cursor="+cursor)
	require.Equal(t, http.StatusOK, s, b)
	s, b = get(t, agent, "/api/v1/ledger/trial-balance")
	assert.Equal(t, http.StatusForbidden, s, b)
}

func TestRefundReopenedPrincipalAccruesFromRefundDay(t *testing.T) {
	defer assertBillingInvariant(t)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	pol := billingPolicy(t, 20, 1, 100000)
	s, p, _ := postKey(t, account, policyPath(pol)+"/payments", map[string]any{"amount_kurus": 100000, "method": "TRANSFER", "payment_date": pol["start_date"]}, fmt.Sprintf("early-full-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, p)
	s, r, _ := postKey(t, account, fmt.Sprintf("/api/v1/payments/%v/refund", p["id"]), map[string]any{"amount_kurus": 1000, "reason": "geri aç"}, fmt.Sprintf("early-refund-%v", p["id"]))
	require.Equal(t, http.StatusCreated, s, r)
	b := billingDetail(t, account, pol)
	assert.Empty(t, b["interest"], "tam ödenmiş anapara için geriye dönük faiz yok")
	var actor int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = 'muhasebe@guvenpolice.test'`).Scan(&actor))
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{UserID: actor, Role: auth.RoleAccounting})
	tomorrow := caldate.Today(time.Now()).AddDate(0, 0, 1)
	svc := payment.NewService(pool, func() time.Time { return tomorrow }, 1000)
	amount := int64(1)
	_, err := svc.Pay(ctx, num(pol["id"]), payment.PayRequest{AmountKurus: &amount, Method: "CARD"})
	require.NoError(t, err)
	b = billingDetail(t, account, pol)
	charges := b["interest"].([]any)
	require.Len(t, charges, 1)
	c := charges[0].(map[string]any)
	assert.Equal(t, int64(1), num(c["amount_kurus"]), "1000 × 1000 × 1 / 1e6 = 1")
	assert.Equal(t, caldate.Format(caldate.Today(time.Now())), c["accrued_from"])
	assert.Equal(t, int64(1), num(c["paid_kurus"]))
}

func TestCancelledPolicyAcceptsRemainingBalance(t *testing.T) {
	defer assertBillingInvariant(t)
	account, uw := tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "underwriter@guvenpolice.test")
	pol := billingPolicy(t, 20, 3, 300000)
	body := map[string]any{"effective_date": day(caldate.Today(time.Now()), -5), "reason": "UNDERWRITER"}
	s, r, _ := postKey(t, uw, policyPath(pol)+"/cancel", body, fmt.Sprintf("cancel-open-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, r)
	b := billingDetail(t, account, pol)
	// İptalden önceki 15 gün: azaltılmış açık anapara üzerinde faiz; iptal tarihiyle kırpılır.
	payBilling(t, account, pol, 1, "cancelled-pay")
	b = billingDetail(t, account, pol)
	for _, v := range b["interest"].([]any) {
		assert.Equal(t, body["effective_date"], v.(map[string]any)["accrued_to"])
	}
	payBilling(t, account, pol, num(b["open_balance_kurus"]), "cancelled-close")
	assert.Zero(t, num(billingDetail(t, account, pol)["open_balance_kurus"]))
}
