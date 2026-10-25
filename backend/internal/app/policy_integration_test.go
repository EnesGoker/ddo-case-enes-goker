//go:build integration

package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
)

// issuePolicy Ayşe için KASKO teklifi oluşturup poliçeye çevirir; poliçe yanıtını döner.
func issuePolicy(t *testing.T, agent string) map[string]any {
	t.Helper()
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	s, pol, _ := newClient(t).doH(http.MethodPost, fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"]), agent, nil,
		map[string]string{idempotency.Header: fmt.Sprintf("issue-%v", q["id"])})
	require.Equal(t, 201, s, pol)
	return pol
}

func pgCode(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

// Sürümler değişmez: yalnızca açık sürümün effective_to'su bir kez kapatılabilir (00003 tetikleyicisi).
func TestPolicyVersionsAreImmutable(t *testing.T) {
	ctx := context.Background()
	pol := issuePolicy(t, tokenFor(t, "acente@guvenpolice.test"))
	vid := num(pol["version_id"])

	_, err := pool.Exec(ctx, `UPDATE policy_versions SET annual_premium_kurus = 1 WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "açık sürümde prim değiştirilemez")
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = effective_from + 10, vehicle_plate = '06ZZ99' WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "kapatırken başka alan değiştirilemez")
	_, err = pool.Exec(ctx, `UPDATE policy_version_coverages SET limit_kurus = 1 WHERE policy_version_id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "sürüm teminatı değiştirilemez")
	_, err = pool.Exec(ctx, `DELETE FROM policy_version_coverages WHERE policy_version_id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "sürüm teminatı silinemez")
	_, err = pool.Exec(ctx, `DELETE FROM policy_versions WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "sürüm silinemez")

	// Tek izinli geçiş: NULL → tarih
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = effective_from + 10 WHERE id = $1`, vid)
	require.NoError(t, err)
	// Kapanmış sürüm bir daha değiştirilemez (tarih dahil)
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = effective_from + 20 WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "kapanmış sürüm yeniden kapatılamaz")
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = NULL WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "kapanmış sürüm yeniden açılamaz")
}

// --- A3 yardımcıları ---

func dateOf(t *testing.T, v any) time.Time {
	t.Helper()
	d, err := caldate.Parse(v.(string))
	require.NoError(t, err)
	return d
}

func day(t time.Time, n int) string { return caldate.Format(t.AddDate(0, 0, n)) }

func policyPath(pol map[string]any) string { return fmt.Sprintf("/api/v1/policies/%v", pol["id"]) }

func postKey(t *testing.T, token, path string, body any, key string) (int, map[string]any, http.Header) {
	t.Helper()
	return newClient(t).doH(http.MethodPost, path, token, body, map[string]string{idempotency.Header: key})
}

func get(t *testing.T, token, path string) (int, map[string]any) {
	t.Helper()
	return newClient(t).do(http.MethodGet, path, token, nil)
}

// expectedKaskoAnnual Ayşe'nin KASKO primini saf motor + örnek tarife ile bağımsız hesaplar.
func expectedKaskoAnnual(t *testing.T, start time.Time, group string) int64 {
	t.Helper()
	var tr pricing.Tariff
	for _, s := range pricing.SampleTariffs() {
		if s.Product == pricing.Kasko && s.ValidOn(start) {
			tr = s
		}
	}
	age := caldate.AgeOn(time.Date(1988, 4, 12, 0, 0, 0, 0, time.UTC), start)
	ncd := 2
	res, err := pricing.Calculate(tr, pricing.Input{Product: pricing.Kasko,
		Risk:      pricing.Risk{DriverAge: &age, NCDLevel: &ncd, RegionCode: "34", VehicleGroup: group},
		Coverages: []string{"KASKO_ANA", "CAM"}})
	require.NoError(t, err)
	return res.TotalKurus
}

func versionNoOn(t *testing.T, token string, pol map[string]any, date string) (int, string) {
	t.Helper()
	s, body := get(t, token, policyPath(pol)+"/coverage?date="+date)
	if s != 200 {
		return s, errCode(body)
	}
	return s, fmt.Sprint(body["version"].(map[string]any)["version_no"])
}

// assertVersionChain sürümlerin çakışmadan ve boşluksuz zincirlendiğini doğrular.
func assertVersionChain(t *testing.T, policyID int64) []map[string]any {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT version_no, effective_from, effective_to FROM policy_versions WHERE policy_id = $1 ORDER BY version_no`, policyID)
	require.NoError(t, err)
	var out []map[string]any
	var prevTo *time.Time
	for rows.Next() {
		var no int
		var from time.Time
		var to *time.Time
		require.NoError(t, rows.Scan(&no, &from, &to))
		if prevTo != nil {
			assert.Equal(t, *prevTo, from, "sürüm %d bir öncekinin bittiği gün başlamalı", no)
		}
		prevTo = to
		out = append(out, map[string]any{"no": no, "from": caldate.Format(from), "to": to})
	}
	require.NoError(t, rows.Err())
	return out
}

func endorsementCount(t *testing.T, policyID int64) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM endorsements WHERE policy_id = $1`, policyID).Scan(&n))
	return n
}

// --- A3 testleri ---

func TestVehicleChangeEndorsementAndCoverageOnDate(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")
	pol := issuePolicy(t, agent)
	path := policyPath(pol)
	start, end := dateOf(t, pol["start_date"]), dateOf(t, pol["end_date"])
	zeyil := start.AddDate(0, 0, 100)
	body := map[string]any{"effective_date": caldate.Format(zeyil), "vehicle_group": "D", "plate": "06 abc 42", "base_version_no": 1}

	s, prev := newClient(t).do(http.MethodPost, path+"/endorsements/preview", agent, body)
	require.Equal(t, 200, s, prev)
	oldAnnual, newAnnual := num(prev["old_annual_premium_kurus"]), num(prev["new_annual_premium_kurus"])
	assert.Equal(t, expectedKaskoAnnual(t, start, "C"), oldAnnual)
	assert.Equal(t, expectedKaskoAnnual(t, start, "D"), newAnnual, "yeni prim orijinal tarifeyle")
	periodDays := proration.DaysBetween(start, end)
	remaining := proration.DaysBetween(zeyil, end)
	wantDelta := proration.RoundDiv((newAnnual-oldAnnual)*remaining, periodDays)
	assert.Equal(t, wantDelta, num(prev["premium_delta_kurus"]))
	assert.Positive(t, wantDelta, "C→D prim artışı: ek prim")
	calc := prev["calc_detail"].(map[string]any)
	assert.Equal(t, float64(periodDays), calc["period_days"])
	assert.Equal(t, float64(remaining), calc["remaining_days"])
	assert.Equal(t, "06ABC42", prev["vehicle_plate"])
	assert.NotEmpty(t, calc["steps"])

	key := fmt.Sprintf("veh-%v", pol["id"])
	s, res, h1 := postKey(t, agent, path+"/endorsements", body, key)
	require.Equal(t, 201, s, res)
	assert.Empty(t, h1.Get(idempotency.ReplayedHeader))
	e := res["endorsement"].(map[string]any)
	v := res["version"].(map[string]any)
	assert.Equal(t, float64(1), e["endorsement_no"])
	assert.Equal(t, "VEHICLE_CHANGE", e["type"])
	assert.Equal(t, prev["premium_delta_kurus"], e["premium_delta_kurus"], "kayıt = önizleme")
	assert.Equal(t, float64(2), v["version_no"])
	assert.Equal(t, caldate.Format(zeyil), v["effective_from"])
	assert.Nil(t, v["effective_to"])
	assert.Equal(t, float64(newAnnual), v["annual_premium_kurus"])

	// Aynı anahtar → aynı yanıt, tek zeyil
	s, again, h2 := postKey(t, agent, path+"/endorsements", body, key)
	assert.Equal(t, 201, s)
	assert.Equal(t, "true", h2.Get(idempotency.ReplayedHeader))
	assert.Equal(t, res, again)
	assert.Equal(t, 1, endorsementCount(t, num(pol["id"])))

	// Tarihte teminat: zeyil gününde yeni sürüm, bir gün önce eski
	for date, want := range map[string]string{
		caldate.Format(start): "1", day(zeyil, -1): "1", caldate.Format(zeyil): "2", day(end, -1): "2",
	} {
		s, got := versionNoOn(t, agent, pol, date)
		assert.Equal(t, 200, s, date)
		assert.Equal(t, want, got, date)
	}
	for _, date := range []string{day(start, -1), caldate.Format(end)} {
		s, code := versionNoOn(t, agent, pol, date)
		assert.Equal(t, 404, s, date)
		assert.Equal(t, "NO_COVERAGE_ON_DATE", code, date)
	}

	s, det := get(t, agent, path)
	require.Equal(t, 200, s)
	versions := det["versions"].([]any)
	require.Len(t, versions, 2)
	assert.Equal(t, caldate.Format(zeyil), versions[0].(map[string]any)["effective_to"])
	assert.Equal(t, float64(1), versions[1].(map[string]any)["endorsement_no"])
	assert.Len(t, det["endorsements"], 1)
	prem := det["premium"].(map[string]any)
	assert.Equal(t, float64(oldAnnual), prem["initial_premium_kurus"])
	assert.Equal(t, float64(oldAnnual+wantDelta), prem["written_premium_kurus"])
	assertVersionChain(t, num(pol["id"]))

	var audits int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'POLICY_ENDORSED' AND entity_id = $1`, fmt.Sprint(pol["id"])).Scan(&audits))
	assert.Equal(t, 1, audits)
}

func TestShortPeriodCancellationAfterEndorsement(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")
	pol := issuePolicy(t, agent)
	path := policyPath(pol)
	start, end := dateOf(t, pol["start_date"]), dateOf(t, pol["end_date"])
	s, z, _ := postKey(t, agent, path+"/endorsements",
		map[string]any{"effective_date": day(start, 100), "vehicle_group": "D"}, fmt.Sprintf("sp-z-%v", pol["id"]))
	require.Equal(t, 201, s, z)
	delta := num(z["endorsement"].(map[string]any)["premium_delta_kurus"])
	newAnnual := num(z["version"].(map[string]any)["annual_premium_kurus"])
	written := num(pol["annual_premium_kurus"]) + delta

	cancelAt := start.AddDate(0, 0, 200)
	body := map[string]any{"effective_date": caldate.Format(cancelAt), "reason": "CUSTOMER_REQUEST", "note": "müşteri aracı sattı", "base_version_no": 2}
	s, prev := newClient(t).do(http.MethodPost, path+"/cancel/preview", agent, body)
	require.Equal(t, 200, s, prev)
	band, err := proration.ShortPeriodRate(proration.SampleShortPeriodTable(), 200)
	require.NoError(t, err)
	earned := proration.RoundDiv(newAnnual*int64(band.RateBP), 10_000)
	assert.Equal(t, "SHORT_PERIOD", prev["method"])
	assert.Equal(t, float64(written), prev["written_premium_kurus"])
	assert.Equal(t, float64(written-earned), prev["refund_kurus"])
	assert.Equal(t, true, prev["closes_version"])

	s, res, _ := postKey(t, agent, path+"/cancel", body, fmt.Sprintf("sp-c-%v", pol["id"]))
	require.Equal(t, 201, s, res)
	assert.Equal(t, prev["refund_kurus"], res["refund_kurus"])
	p := res["policy"].(map[string]any)
	assert.Equal(t, "CANCELLED", p["status"])
	assert.Equal(t, caldate.Format(cancelAt), p["cancel_effective_date"])
	assert.Equal(t, "CUSTOMER_REQUEST", p["cancel_reason"])
	e := res["endorsement"].(map[string]any)
	assert.Equal(t, "CANCELLATION", e["type"])
	assert.Equal(t, float64(-(written - earned)), e["premium_delta_kurus"])
	assert.Nil(t, e["to_version_id"])

	// İptal tarihinde ve sonrasında teminat yok; bir gün önce son sürüm
	s, got := versionNoOn(t, agent, pol, day(cancelAt, -1))
	assert.Equal(t, 200, s)
	assert.Equal(t, "2", got)
	for _, date := range []string{caldate.Format(cancelAt), day(cancelAt, 1), day(end, -1)} {
		s, code := versionNoOn(t, agent, pol, date)
		assert.Equal(t, 404, s, date)
		assert.Equal(t, "NO_COVERAGE_ON_DATE", code)
	}
	chain := assertVersionChain(t, num(pol["id"]))
	require.Len(t, chain, 2)
	assert.NotNil(t, chain[1]["to"], "iptalde açık sürüm kapanır")

	// İptal edilmiş poliçeye zeyil/iptal yok
	s, body2, _ := postKey(t, agent, path+"/endorsements", map[string]any{"effective_date": day(start, 150), "vehicle_group": "A"}, "after-cancel")
	assert.Equal(t, 422, s)
	assert.Equal(t, "POLICY_CANCELLED", errCode(body2))
	s, body2, _ = postKey(t, agent, path+"/cancel", body, fmt.Sprintf("sp-c2-%v", pol["id"]))
	assert.Equal(t, 422, s)
	assert.Equal(t, "POLICY_CANCELLED", errCode(body2))

	s, det := get(t, tokenFor(t, "musteri@guvenpolice.test"), path)
	require.Equal(t, 200, s, "müşteri kendi poliçesini salt okunur görür")
	prem := det["premium"].(map[string]any)
	assert.Equal(t, float64(written-earned), prem["cancellation_refund_kurus"])
	assert.Equal(t, float64(earned), prem["net_premium_kurus"])
}

func TestProRataCancellationOnStartDay(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")
	uw := tokenFor(t, "underwriter@guvenpolice.test")
	pol := issuePolicy(t, agent)
	path := policyPath(pol)
	start := dateOf(t, pol["start_date"])
	body := map[string]any{"effective_date": caldate.Format(start), "reason": "UNDERWRITER"}

	s, b, _ := postKey(t, agent, path+"/cancel", body, fmt.Sprintf("ag-uw-%v", pol["id"]))
	assert.Equal(t, 403, s, "acente UNDERWRITER gerekçesiyle iptal edemez")
	assert.Equal(t, "FORBIDDEN", errCode(b))
	s, _, _ = postKey(t, agent, path+"/cancel", map[string]any{"effective_date": caldate.Format(start), "reason": "NON_PAYMENT"}, fmt.Sprintf("ag-np-%v", pol["id"]))
	assert.Equal(t, 403, s, "NON_PAYMENT yalnızca underwriter")
	customer := tokenFor(t, "musteri@guvenpolice.test")
	s, _, _ = postKey(t, customer, path+"/cancel", map[string]any{"effective_date": caldate.Format(start), "reason": "CUSTOMER_REQUEST"}, "cust-c")
	assert.Equal(t, 403, s, "müşteri iptal edemez")
	s, _, _ = postKey(t, customer, path+"/endorsements", map[string]any{"effective_date": day(start, 10), "vehicle_group": "A"}, "cust-z")
	assert.Equal(t, 403, s, "müşteri zeyil yapamaz")
	s, _ = get(t, tokenFor(t, "eksper@guvenpolice.test"), path)
	assert.Equal(t, 403, s)

	// Başlangıç gününde gün esaslı iptal: kalan = dönem, iade = yazılmış primin tamamı
	s, res, _ := postKey(t, uw, path+"/cancel", body, fmt.Sprintf("uw-start-%v", pol["id"]))
	require.Equal(t, 201, s, res)
	assert.Equal(t, pol["annual_premium_kurus"], res["refund_kurus"])
	calc := res["endorsement"].(map[string]any)["calc_detail"].(map[string]any)
	assert.Equal(t, "PRO_RATA", calc["method"])
	assert.Equal(t, float64(0), calc["elapsed_days"])
	assert.Equal(t, false, calc["version_closed"], "sıfır uzunlukta aralık açılmaz")
	s, code := versionNoOn(t, agent, pol, caldate.Format(start))
	assert.Equal(t, 404, s, "başlangıçtan iptal: hiçbir gün teminat yok")
	assert.Equal(t, "NO_COVERAGE_ON_DATE", code)
}

func TestEndorsementValidation(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")
	pol := issuePolicy(t, agent)
	path := policyPath(pol) + "/endorsements/preview"
	start, end := dateOf(t, pol["start_date"]), dateOf(t, pol["end_date"])
	c := newClient(t)
	check := func(name string, body map[string]any, status int, code string) {
		t.Helper()
		s, b := c.do(http.MethodPost, path, agent, body)
		assert.Equal(t, status, s, name)
		assert.Equal(t, code, errCode(b), name)
	}
	check("başlangıç günü", map[string]any{"effective_date": caldate.Format(start), "vehicle_group": "D"}, 422, "INVALID_EFFECTIVE_DATE")
	check("bitiş günü", map[string]any{"effective_date": caldate.Format(end), "vehicle_group": "D"}, 422, "INVALID_EFFECTIVE_DATE")
	check("bozuk tarih", map[string]any{"effective_date": "2027-02-30", "vehicle_group": "D"}, 400, "VALIDATION_ERROR")
	check("değişiklik yok", map[string]any{"effective_date": day(start, 10), "vehicle_group": "C", "plate": "34ABC123"}, 422, "NO_CHANGE")
	check("bozuk plaka", map[string]any{"effective_date": day(start, 10), "plate": "XYZ"}, 400, "VALIDATION_ERROR")
	check("bilinmeyen grup", map[string]any{"effective_date": day(start, 10), "vehicle_group": "Z"}, 400, "VALIDATION_ERROR")
	check("katalog üstü limit", map[string]any{"effective_date": day(start, 10), "coverage_limits": map[string]int64{"KASKO_ANA": 999_000_000_000}}, 400, "VALIDATION_ERROR")
	check("olmayan teminat", map[string]any{"effective_date": day(start, 10), "coverage_limits": map[string]int64{"IMM": 100}}, 400, "VALIDATION_ERROR")
	check("eski sürüm", map[string]any{"effective_date": day(start, 10), "vehicle_group": "D", "base_version_no": 7}, 409, "STALE_VERSION")

	// Geriye dönük zincir yok: son zeyilden önceki bir tarihe zeyil yapılamaz
	s, z, _ := postKey(t, agent, policyPath(pol)+"/endorsements", map[string]any{"effective_date": day(start, 100), "vehicle_group": "D"}, fmt.Sprintf("chain-%v", pol["id"]))
	require.Equal(t, 201, s, z)
	check("zeyil öncesine zeyil", map[string]any{"effective_date": day(start, 50), "vehicle_group": "A"}, 422, "INVALID_EFFECTIVE_DATE")
	check("zeyil günü", map[string]any{"effective_date": day(start, 100), "vehicle_group": "A"}, 422, "INVALID_EFFECTIVE_DATE")
	s, b := c.do(http.MethodPost, path, agent, map[string]any{"effective_date": day(start, 101), "vehicle_group": "A"})
	assert.Equal(t, 200, s, b)
	assert.Negative(t, num(b["premium_delta_kurus"]), "D→A prim düşüşü: iade")

	// İptal tarihi açık sürümün başlangıcından önce olamaz
	s, b = c.do(http.MethodPost, policyPath(pol)+"/cancel/preview", agent, map[string]any{"effective_date": day(start, 99), "reason": "CUSTOMER_REQUEST"})
	assert.Equal(t, 422, s)
	assert.Equal(t, "INVALID_EFFECTIVE_DATE", errCode(b))
	s, b = c.do(http.MethodPost, policyPath(pol)+"/cancel/preview", agent, map[string]any{"effective_date": day(start, 120), "reason": "KEYFI"})
	assert.Equal(t, 400, s)
	assert.Equal(t, "VALIDATION_ERROR", errCode(b))

	// KONUT poliçesinde araç değişikliği yok
	q := createQuote(t, agent, map[string]any{"customer_id": customerID(t, ayse), "product": "KONUT",
		"term_start": futureStart(), "ncd_level": 1, "coverages": []string{"YANGIN"}})
	s, konut, _ := postKey(t, agent, fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"]), nil, fmt.Sprintf("konut-%v", q["id"]))
	require.Equal(t, 201, s, konut)
	s, b = c.do(http.MethodPost, policyPath(konut)+"/endorsements/preview", agent,
		map[string]any{"effective_date": day(dateOf(t, konut["start_date"]), 10), "vehicle_group": "A"})
	assert.Equal(t, 422, s)
	assert.Equal(t, "NOT_A_VEHICLE_POLICY", errCode(b))

	// Başka müşterinin poliçesi müşteriye 404
	s, b = get(t, tokenFor(t, "musteri@guvenpolice.test"), "/api/v1/policies/999999")
	assert.Equal(t, 404, s)
	assert.Equal(t, "NOT_FOUND", errCode(b))
	s, _ = get(t, agent, policyPath(pol)+"/coverage")
	assert.Equal(t, 400, s, "date zorunlu")
}

func TestLimitCannotDropBelowUsage(t *testing.T) {
	ctx := context.Background()
	agent := tokenFor(t, "acente@guvenpolice.test")
	pol := issuePolicy(t, agent)
	pid := num(pol["id"])
	start := dateOf(t, pol["start_date"])
	// Hasar A4'te: kullanımı doğrudan bakiyeye yazarak simüle et
	_, err := pool.Exec(ctx, `UPDATE policy_limit_balances SET used_kurus = 100000000 WHERE policy_id = $1 AND coverage_code = 'KASKO_ANA'`, pid)
	require.NoError(t, err)

	s, b, _ := postKey(t, agent, policyPath(pol)+"/endorsements",
		map[string]any{"effective_date": day(start, 30), "coverage_limits": map[string]int64{"KASKO_ANA": 50_000_000}}, fmt.Sprintf("lim-low-%d", pid))
	assert.Equal(t, 422, s)
	assert.Equal(t, "LIMIT_BELOW_USAGE", errCode(b))
	assert.Equal(t, 0, endorsementCount(t, pid), "başarısız zeyil iz bırakmaz")

	s, b, _ = postKey(t, agent, policyPath(pol)+"/endorsements",
		map[string]any{"effective_date": day(start, 30), "coverage_limits": map[string]int64{"KASKO_ANA": 120_000_000}}, fmt.Sprintf("lim-ok-%d", pid))
	require.Equal(t, 201, s, b)
	assert.Equal(t, float64(0), b["endorsement"].(map[string]any)["premium_delta_kurus"], "limit primi etkilemez (örnek tarife)")
	var limit, used int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT limit_kurus, used_kurus FROM policy_limit_balances WHERE policy_id = $1 AND coverage_code = 'KASKO_ANA'`, pid).Scan(&limit, &used))
	assert.Equal(t, int64(120_000_000), limit)
	assert.Equal(t, int64(100_000_000), used, "kullanım korunur")

	var oldLimit, newLimit int64
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT (SELECT c.limit_kurus FROM policy_version_coverages c JOIN policy_versions v ON v.id = c.policy_version_id WHERE v.policy_id = $1 AND v.version_no = 1 AND c.coverage_code = 'KASKO_ANA'),
		       (SELECT c.limit_kurus FROM policy_version_coverages c JOIN policy_versions v ON v.id = c.policy_version_id WHERE v.policy_id = $1 AND v.version_no = 2 AND c.coverage_code = 'KASKO_ANA')`, pid).
		Scan(&oldLimit, &newLimit))
	assert.Equal(t, int64(150_000_000), oldLimit, "eski sürüm geçmişi korunur")
	assert.Equal(t, int64(120_000_000), newLimit)
}

// Aynı poliçeye farklı anahtarlarla eşzamanlı zeyiller: poliçe kilidi serileştirir.
func TestConcurrentEndorsements(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")

	// Aynı temel sürüme (base_version_no=1) karşı 5 eşzamanlı zeyil → 1 başarı, 4 × 409 STALE_VERSION
	pol := issuePolicy(t, agent)
	start := dateOf(t, pol["start_date"])
	body := map[string]any{"effective_date": day(start, 60), "vehicle_group": "D", "base_version_no": 1}
	codes := runParallel(t, 5, func(i int) int {
		s, _, _ := postKey(t, agent, policyPath(pol)+"/endorsements", body, fmt.Sprintf("cc-%v-%d", pol["id"], i))
		return s
	})
	assert.Equal(t, 1, count(codes, 201), codes)
	assert.Equal(t, 4, count(codes, 409), codes)
	assert.Equal(t, 1, endorsementCount(t, num(pol["id"])))
	assert.Len(t, assertVersionChain(t, num(pol["id"])), 2)

	// Temel sürüm verilmeden aynı tarihli 2 zeyil → biri 201, diğeri 422 (tarih yeni açık sürümün başlangıcı)
	pol2 := issuePolicy(t, agent)
	body2 := map[string]any{"effective_date": day(dateOf(t, pol2["start_date"]), 60), "vehicle_group": "D"}
	codes = runParallel(t, 2, func(i int) int {
		s, _, _ := postKey(t, agent, policyPath(pol2)+"/endorsements", body2, fmt.Sprintf("cc2-%v-%d", pol2["id"], i))
		return s
	})
	assert.ElementsMatch(t, []int{201, 422}, codes)
	assert.Equal(t, 1, endorsementCount(t, num(pol2["id"])))
	var dup int
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT count(*) FROM (SELECT endorsement_no FROM endorsements WHERE policy_id = ANY($1) GROUP BY policy_id, endorsement_no HAVING count(*) > 1) x`,
		[]int64{num(pol["id"]), num(pol2["id"])}).Scan(&dup))
	assert.Zero(t, dup, "endorsement_no poliçe içinde tekil")
}

// Eşzamanlı zeyil + iptal: hangi sırada işlenirse işlensin son durum tutarlı.
func TestConcurrentEndorsementAndCancellation(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")
	uw := tokenFor(t, "underwriter@guvenpolice.test")
	for i := 0; i < 4; i++ {
		pol := issuePolicy(t, agent)
		pid := num(pol["id"])
		start, end := dateOf(t, pol["start_date"]), dateOf(t, pol["end_date"])
		var zeyilStatus, cancelStatus int
		var cancelRes map[string]any
		runParallel(t, 2, func(j int) int {
			if j == 0 {
				zeyilStatus, _, _ = postKey(t, agent, policyPath(pol)+"/endorsements",
					map[string]any{"effective_date": day(start, 30), "vehicle_group": "D"}, fmt.Sprintf("zc-z-%d", pid))
				return zeyilStatus
			}
			cancelStatus, cancelRes, _ = postKey(t, uw, policyPath(pol)+"/cancel",
				map[string]any{"effective_date": day(start, 60), "reason": "UNDERWRITER"}, fmt.Sprintf("zc-c-%d", pid))
			return cancelStatus
		})
		require.Equal(t, 201, cancelStatus, cancelRes)
		require.Contains(t, []int{201, 422}, zeyilStatus)

		_, det := get(t, uw, policyPath(pol))
		assert.Equal(t, "CANCELLED", det["status"])
		chain := assertVersionChain(t, pid)
		wantVersions, wantEndorsements := 1, 1
		if zeyilStatus == 201 {
			wantVersions, wantEndorsements = 2, 2 // zeyil önce işlendi
		}
		require.Len(t, chain, wantVersions)
		assert.Equal(t, wantEndorsements, endorsementCount(t, pid))
		for _, v := range chain {
			assert.NotNil(t, v["to"], "iptal sonrası açık sürüm kalmaz")
		}
		// İade, iptal anındaki açık sürümün primiyle gün esaslı hesaplanmış olmalı
		versions := det["versions"].([]any)
		lastAnnual := num(versions[len(versions)-1].(map[string]any)["annual_premium_kurus"])
		want := proration.RoundDiv(lastAnnual*proration.DaysBetween(start.AddDate(0, 0, 60), end), proration.DaysBetween(start, end))
		assert.Equal(t, float64(want), cancelRes["refund_kurus"])
	}
}
