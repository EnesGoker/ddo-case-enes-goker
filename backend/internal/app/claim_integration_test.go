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
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/jackc/pgx/v5"
)

// Geçmiş hasarlar için yalnızca fixture poliçesi: mevcut Issue yardımı kendi transaction'ında.
// Üretim API'sinin geriye dönük poliçe yasağı ve gelecekte hasar yasağı korunur.
func claimPolicy(t *testing.T, agent string) map[string]any {
	t.Helper()
	var tariffID, agentID int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM tariff_versions WHERE product='KASKO' AND version_no=2`).Scan(&tariffID))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM agents ORDER BY id LIMIT 1`).Scan(&agentID))
	start := caldate.Today(time.Now()).AddDate(0, 0, -30)
	var out policy.Issued
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		// Gerçek teklif kaydı oluşturulur; Issue'nın quote FK'si ve benzersizliği korunur.
		var qid int64
		uid := int64(0)
		if err := tx.QueryRow(context.Background(), `SELECT user_id FROM agents WHERE id=$1`, agentID).Scan(&uid); err != nil {
			return err
		}
		if err := tx.QueryRow(context.Background(), `INSERT INTO quotes (customer_id,agent_id,created_by,product,tariff_version_id,term_start,term_end,risk_data,coverage_selection,input_hash,total_premium_kurus,status) VALUES ($1,$2,$3,'KASKO',$4,$5,$6,'{}','{}','fixture',100000,'CONVERTED') RETURNING id`, customerID(t, ayse), agentID, uid, tariffID, start, caldate.AddYear(start)).Scan(&qid); err != nil {
			return err
		}
		var err error
		out, err = policy.Issue(context.Background(), tx, policy.IssueInput{QuoteID: qid, Product: pricing.Kasko, CustomerID: customerID(t, ayse), AgentID: &agentID, TariffVersionID: tariffID, StartDate: start, EndDate: caldate.AddYear(start), Plate: strptr("34ABC123"), RiskData: []byte(`{"driver_age":38,"ncd_level":2,"region_code":"34","vehicle_group":"C","plate":"34ABC123"}`), AnnualPremium: 100000, AnnualLimitKurus: 1_000_000, Coverages: []policy.CoverageTerm{{Code: "KASKO_ANA", LimitKurus: 2_000_000, DeductibleType: pricing.DeductibleNone}, {Code: "CAM", LimitKurus: 500_000, DeductibleType: pricing.DeductibleFixed, DeductibleValue: 50_000}}})
		return err
	})
	require.NoError(t, err)
	return map[string]any{"id": float64(out.ID), "version_id": float64(out.VersionID), "start_date": out.StartDate, "end_date": out.EndDate}
}
func strptr(s string) *string           { return &s }
func claimPath(c map[string]any) string { return fmt.Sprintf("/api/v1/claims/%v", c["id"]) }
func reportClaim(t *testing.T, token string, pol map[string]any, coverage, loss string) map[string]any {
	t.Helper()
	s, b, _ := postKey(t, token, "/api/v1/claims", map[string]any{"policy_id": pol["id"], "coverage_code": coverage, "loss_date": loss, "description": "Entegrasyon hasarı", "claimed_amount_kurus": 800000}, fmt.Sprintf("claim-%v-%s-%s-%d", pol["id"], coverage, loss, time.Now().UnixNano()))
	require.Equal(t, 201, s, b)
	return b
}
func transitionClaim(t *testing.T, token string, c map[string]any, to string) map[string]any {
	t.Helper()
	s, b := newClient(t).do(http.MethodPost, claimPath(c)+"/transition", token, map[string]any{"to_status": to, "expected_row_version": c["row_version"], "note": "test"})
	require.Equal(t, 200, s, b)
	return b
}
func adjustmentClaim(t *testing.T, pol map[string]any, adjuster, appraiser string, damage int64) map[string]any {
	t.Helper()
	c := reportClaim(t, adjuster, pol, "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
	c = transitionClaim(t, adjuster, c, "TRIAGED")
	c = transitionClaim(t, adjuster, c, "APPRAISAL")
	s, b := newClient(t).do(http.MethodPost, claimPath(c)+"/appraisals", appraiser, map[string]any{"damage_amount_kurus": damage, "is_total_loss": false, "notes": "tespit", "inspected_at": time.Now().UTC().Format(time.RFC3339)})
	require.Equal(t, 201, s, b)
	return transitionClaim(t, adjuster, c, "ADJUSTMENT")
}

func TestClaimLifecycleAndIdempotency(t *testing.T) {
	adjuster, appraiser, accounting := tokenFor(t, "hasar@guvenpolice.test"), tokenFor(t, "eksper@guvenpolice.test"), tokenFor(t, "muhasebe@guvenpolice.test")
	pol := claimPolicy(t, "")
	c := adjustmentClaim(t, pol, adjuster, appraiser, 800000)
	path := claimPath(c)
	s, preview := newClient(t).do(http.MethodPost, path+"/settlement/preview", adjuster, nil)
	require.Equal(t, 200, s, preview)
	assert.Equal(t, int64(800000), num(preview["payable_kurus"]))
	assert.Len(t, preview["calc_steps"], 6)
	s, approved, _ := postKey(t, adjuster, path+"/settlement/approve", nil, fmt.Sprintf("approve-%v", c["id"]))
	require.Equal(t, 201, s, approved)
	s, replayed, h := postKey(t, adjuster, path+"/settlement/approve", nil, fmt.Sprintf("approve-%v", c["id"]))
	require.Equal(t, 201, s, replayed)
	assert.Equal(t, "true", h.Get("Idempotent-Replayed"))
	assert.Equal(t, approved, replayed)
	s, paid, _ := postKey(t, accounting, path+"/pay", nil, fmt.Sprintf("pay-%v", c["id"]))
	require.Equal(t, 200, s, paid)
	s, again, _ := postKey(t, accounting, path+"/pay", nil, fmt.Sprintf("pay-%v", c["id"]))
	assert.Equal(t, 200, s)
	assert.Equal(t, paid, again)
	s, detail := get(t, adjuster, path)
	require.Equal(t, 200, s, detail)
	assert.Equal(t, "PAID", detail["status"])
	assert.Len(t, detail["history"], 6)
	assert.Len(t, detail["appraisals"], 1)
	settlement := detail["settlement"].(map[string]any)
	assert.Equal(t, "PAID", settlement["status"])
	assert.NotNil(t, settlement["payment_journal_entry_id"])
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM claim_settlements WHERE claim_id=$1`, num(c["id"])).Scan(&n))
	assert.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM journal_entries WHERE ref_id=$1 AND ref_type IN ('CLAIM_APPROVAL','CLAIM_PAYMENT')`, num(c["id"])).Scan(&n))
	assert.Equal(t, 2, n)
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM (SELECT e.id FROM journal_entries e LEFT JOIN journal_lines l ON l.entry_id=e.id GROUP BY e.id HAVING count(l.id)=0 OR sum(l.debit_kurus)<>sum(l.credit_kurus)) x`).Scan(&n))
	assert.Zero(t, n)
	var debit, credit int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT coalesce(sum(debit_kurus),0),coalesce(sum(credit_kurus),0) FROM journal_lines`).Scan(&debit, &credit))
	assert.Equal(t, debit, credit)
	transitionClaim(t, adjuster, detail, "CLOSED")
}

func TestConcurrentClaimApprovals(t *testing.T) {
	adjuster, appraiser := tokenFor(t, "hasar@guvenpolice.test"), tokenFor(t, "eksper@guvenpolice.test")
	pol := claimPolicy(t, "")
	a := adjustmentClaim(t, pol, adjuster, appraiser, 800000)
	b := adjustmentClaim(t, pol, adjuster, appraiser, 800000)
	claims := []map[string]any{a, b}
	codes := runParallel(t, 2, func(i int) int {
		s, _, _ := postKey(t, adjuster, claimPath(claims[i])+"/settlement/approve", nil, fmt.Sprintf("race-%v", claims[i]["id"]))
		return s
	})
	assert.ElementsMatch(t, []int{201, 201}, codes)
	var used, limit, total int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT used_kurus,limit_kurus FROM policy_limit_balances WHERE policy_id=$1 AND coverage_code IS NULL`, num(pol["id"])).Scan(&used, &limit))
	assert.LessOrEqual(t, used, limit)
	assert.Equal(t, int64(1000000), used)
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT sum(s.payable_kurus) FROM claim_settlements s JOIN claims c ON c.id=s.claim_id WHERE c.policy_id=$1`, num(pol["id"])).Scan(&total))
	assert.Equal(t, used, total)
	rows, err := pool.Query(context.Background(), `SELECT payable_kurus FROM claim_settlements WHERE claim_id=ANY($1)`, []int64{num(a["id"]), num(b["id"])})
	require.NoError(t, err)
	amounts, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{800000, 200000}, amounts)
}

func TestClaimVersionSelectionAndCancellation(t *testing.T) {
	agent, adjuster := tokenFor(t, "acente@guvenpolice.test"), tokenFor(t, "hasar@guvenpolice.test")
	pol := claimPolicy(t, "")
	start := dateOf(t, pol["start_date"])
	zdate := day(start, 10)
	s, z, _ := postKey(t, agent, policyPath(pol)+"/endorsements", map[string]any{"effective_date": zdate, "plate": "06ABC42", "vehicle_group": "D"}, fmt.Sprintf("claims-z-%v", pol["id"]))
	require.Equal(t, 201, s, z)
	old := reportClaim(t, adjuster, pol, "KASKO_ANA", day(start, 9))
	newer := reportClaim(t, adjuster, pol, "KASKO_ANA", zdate)
	assert.Equal(t, pol["version_id"], old["policy_version_id"])
	assert.Equal(t, z["version"].(map[string]any)["id"], newer["policy_version_id"])
	assert.Equal(t, "34ABC123", old["plate"])
	assert.Equal(t, "06ABC42", newer["plate"])
	cancel := caldate.Format(caldate.Today(time.Now()).AddDate(0, 0, -1))
	s, b, _ := postKey(t, agent, policyPath(pol)+"/cancel", map[string]any{"effective_date": cancel, "reason": "CUSTOMER_REQUEST"}, fmt.Sprintf("claims-c-%v", pol["id"]))
	require.Equal(t, 201, s, b)
	for _, loss := range []string{cancel, caldate.Format(caldate.Today(time.Now()))} {
		s, b, _ = postKey(t, adjuster, "/api/v1/claims", map[string]any{"policy_id": pol["id"], "coverage_code": "KASKO_ANA", "loss_date": loss, "description": "iptal sonrası", "claimed_amount_kurus": 1}, fmt.Sprintf("after-%v-%s", pol["id"], loss))
		assert.Equal(t, 422, s, b)
		assert.Equal(t, "NO_COVERAGE_ON_DATE", errCode(b))
	}
	pol2 := claimPolicy(t, "")
	s, b, _ = postKey(t, agent, policyPath(pol2)+"/cancel", map[string]any{"effective_date": pol2["start_date"], "reason": "CUSTOMER_REQUEST"}, fmt.Sprintf("start-c-%v", pol2["id"]))
	require.Equal(t, 201, s, b)
	s, b, _ = postKey(t, adjuster, "/api/v1/claims", map[string]any{"policy_id": pol2["id"], "coverage_code": "KASKO_ANA", "loss_date": pol2["start_date"], "description": "başlangıçta iptal", "claimed_amount_kurus": 1}, fmt.Sprintf("start-claim-%v", pol2["id"]))
	assert.Equal(t, 422, s, b)
	assert.Equal(t, "NO_COVERAGE_ON_DATE", errCode(b))
	pol3 := claimPolicy(t, "")
	end := day(dateOf(t, pol3["start_date"]), 20)
	_, err := pool.Exec(context.Background(), `UPDATE policies SET end_date=$2::date WHERE id=$1`, num(pol3["id"]), end)
	require.NoError(t, err)
	s, b, _ = postKey(t, adjuster, "/api/v1/claims", map[string]any{"policy_id": pol3["id"], "coverage_code": "KASKO_ANA", "loss_date": end, "description": "dönem sonu", "claimed_amount_kurus": 1}, fmt.Sprintf("end-claim-%v", pol3["id"]))
	assert.Equal(t, 422, s, b)
	assert.Equal(t, "NO_COVERAGE_ON_DATE", errCode(b))
}

func TestClaimsBlockBackdatedPolicyChanges(t *testing.T) {
	agent, adjuster := tokenFor(t, "acente@guvenpolice.test"), tokenFor(t, "hasar@guvenpolice.test")
	pol := claimPolicy(t, "")
	today := caldate.Today(time.Now())
	c := reportClaim(t, adjuster, pol, "KASKO_ANA", caldate.Format(today))
	for i, date := range []string{day(today, -1), day(today, 0)} {
		for _, endpoint := range []string{"endorsements", "cancel"} {
			body := map[string]any{"effective_date": date}
			if endpoint == "cancel" {
				body["reason"] = "CUSTOMER_REQUEST"
			} else {
				body["vehicle_group"] = "D"
			}
			for _, suffix := range []string{"/preview", ""} {
				s, b, _ := postKey(t, agent, policyPath(pol)+"/"+endpoint+suffix, body, fmt.Sprintf("guard-%v-%s-%d-%s", pol["id"], endpoint, i, suffix))
				assert.Equal(t, 422, s, b)
				assert.Equal(t, "CLAIMS_AFTER_EFFECTIVE_DATE", errCode(b))
			}
		}
	}
	transitionClaim(t, adjuster, c, "REJECTED")
	s, b, _ := postKey(t, agent, policyPath(pol)+"/endorsements", map[string]any{"effective_date": day(today, -1), "vehicle_group": "D"}, fmt.Sprintf("rejected-z-%v", pol["id"]))
	assert.Equal(t, 201, s, b)
}

func TestClaimTransitionsValidationAndRBAC(t *testing.T) {
	adjuster, appraiser, accounting, customer := tokenFor(t, "hasar@guvenpolice.test"), tokenFor(t, "eksper@guvenpolice.test"), tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "musteri@guvenpolice.test")
	pol := claimPolicy(t, "")
	today := caldate.Format(caldate.Today(time.Now()))
	c := reportClaim(t, customer, pol, "CAM", today)
	path := claimPath(c)
	s, b := get(t, appraiser, path)
	assert.Equal(t, 404, s, b)
	s, b = newClient(t).do(http.MethodPost, path+"/transition", adjuster, map[string]any{"to_status": "ADJUSTMENT", "expected_row_version": 1})
	assert.Equal(t, 409, s)
	assert.Equal(t, "INVALID_TRANSITION", errCode(b))
	s, b = newClient(t).do(http.MethodPost, path+"/transition", adjuster, map[string]any{"to_status": "REJECTED", "expected_row_version": 1})
	assert.Equal(t, 422, s)
	assert.Equal(t, "REJECTION_REASON_REQUIRED", errCode(b))
	c = transitionClaim(t, adjuster, c, "TRIAGED")
	s, b = newClient(t).do(http.MethodPost, path+"/transition", adjuster, map[string]any{"to_status": "APPRAISAL", "expected_row_version": 1})
	assert.Equal(t, 409, s)
	assert.Equal(t, "STALE_CLAIM", errCode(b))
	for _, test := range []struct {
		token, path string
		body        any
	}{{appraiser, path + "/settlement/approve", nil}, {adjuster, path + "/pay", nil}, {accounting, "/api/v1/claims", map[string]any{"policy_id": pol["id"]}}} {
		s, b, _ = postKey(t, test.token, test.path, test.body, fmt.Sprintf("rbac-%d", time.Now().UnixNano()))
		assert.Equal(t, 403, s, b)
	}
	// Aynı müşteri hesabı başka müşterinin poliçesini göremez / hasar açamaz.
	var otherCustomer int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM customers WHERE national_id='10000000002'`).Scan(&otherCustomer))
	other := claimPolicy(t, "")
	_, err := pool.Exec(context.Background(), `UPDATE policies SET customer_id=$2 WHERE id=$1`, num(other["id"]), otherCustomer)
	require.NoError(t, err)
	s, b, _ = postKey(t, customer, "/api/v1/claims", map[string]any{"policy_id": other["id"], "coverage_code": "CAM", "loss_date": today, "description": "kapsam dışı", "claimed_amount_kurus": 1}, fmt.Sprintf("other-%v", other["id"]))
	assert.Equal(t, 404, s, b)
	// Eksper yalnızca APPRAISAL ve kendi kayıtlarını görür; son girilen ekspertiz kullanılır.
	c = transitionClaim(t, adjuster, c, "APPRAISAL")
	s, b = get(t, appraiser, path)
	assert.Equal(t, 200, s, b)
	for _, amount := range []int64{100000, 200000} {
		s, b = newClient(t).do(http.MethodPost, path+"/appraisals", appraiser, map[string]any{"damage_amount_kurus": amount, "inspected_at": time.Now().UTC().Format(time.RFC3339)})
		require.Equal(t, 201, s, b)
	}
	c = transitionClaim(t, adjuster, c, "ADJUSTMENT")
	s, b = get(t, appraiser, path)
	assert.Equal(t, 200, s, b)
	s, b = newClient(t).do(http.MethodPost, path+"/settlement/preview", adjuster, nil)
	require.Equal(t, 200, s, b)
	assert.Equal(t, int64(200000), num(b["gross_damage_kurus"]))
	assert.Equal(t, int64(150000), num(b["payable_kurus"]))
	s, b = newClient(t).do(http.MethodPost, path+"/appraisals", appraiser, map[string]any{"damage_amount_kurus": 1, "inspected_at": time.Now().UTC().Format(time.RFC3339)})
	assert.Equal(t, 409, s, b)
	// Keyset ve filtre.
	s, b = get(t, customer, "/api/v1/claims?status=ADJUSTMENT&limit=1")
	require.Equal(t, 200, s, b)
	assert.Len(t, b["items"], 1)
	assert.NotNil(t, b["next_cursor"])
	s, b = get(t, customer, "/api/v1/claims?status=BAD")
	assert.Equal(t, 400, s, b)
	for _, tc := range []struct{ coverage, loss, code string }{{"YOK", today, "COVERAGE_NOT_IN_VERSION"}, {"CAM", day(caldate.Today(time.Now()), 1), "LOSS_DATE_IN_FUTURE"}} {
		s, b, _ = postKey(t, customer, "/api/v1/claims", map[string]any{"policy_id": pol["id"], "coverage_code": tc.coverage, "loss_date": tc.loss, "description": "doğrulama", "claimed_amount_kurus": 1}, fmt.Sprintf("validation-%v-%s", pol["id"], tc.code))
		assert.Equal(t, 422, s, b)
		assert.Equal(t, tc.code, errCode(b))
	}
}

func TestZeroSettlementDoesNotConsumeLimit(t *testing.T) {
	adjuster, appraiser := tokenFor(t, "hasar@guvenpolice.test"), tokenFor(t, "eksper@guvenpolice.test")
	pol := claimPolicy(t, "")
	c := adjustmentClaim(t, pol, adjuster, appraiser, 0)
	s, b, _ := postKey(t, adjuster, claimPath(c)+"/settlement/approve", nil, fmt.Sprintf("zero-%v", c["id"]))
	assert.Equal(t, 422, s, b)
	assert.Equal(t, "LIMIT_EXHAUSTED", errCode(b))
	_, b = get(t, adjuster, claimPath(c))
	assert.Equal(t, "ADJUSTMENT", b["status"])
	assert.Nil(t, b["settlement"])
	var used int64
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT sum(used_kurus) FROM policy_limit_balances WHERE policy_id=$1`, num(pol["id"])).Scan(&used))
	assert.Zero(t, used)
}
