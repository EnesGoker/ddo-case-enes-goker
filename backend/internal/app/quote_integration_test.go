//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

func tokenFor(t *testing.T, email string) string {
	t.Helper()
	status, body := newClient(t).login(email, demoPassword)
	require.Equal(t, 200, status, email)
	return body["access_token"].(string)
}

// customerID ulusal kimliği bilinen demo müşterinin id'si.
func customerID(t *testing.T, nationalID string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM customers WHERE national_id = $1`, nationalID).Scan(&id))
	return id
}

const (
	ayse = "10000000001" // 1988-04-12, bölge 34, müşteri hesabı var
	can  = "10000000005" // 2007-05-10, bölge 06 → genç sürücü
)

func futureStart() string {
	return caldate.Format(caldate.Today(time.Now()).AddDate(0, 0, 7))
}

func kaskoBody(t *testing.T, national, start string) map[string]any {
	return map[string]any{
		"customer_id": customerID(t, national), "product": "KASKO", "term_start": start,
		"ncd_level": 2, "vehicle_group": "C", "plate": "34 ABC 123", "coverages": []string{"KASKO_ANA", "CAM"},
	}
}

func num(v any) int64 {
	f, _ := v.(float64)
	return int64(f)
}

func sumLines(body map[string]any) int64 {
	var s int64
	for _, l := range body["lines"].([]any) {
		s += num(l.(map[string]any)["amount_kurus"])
	}
	return s
}

func createQuote(t *testing.T, token string, body map[string]any) map[string]any {
	t.Helper()
	status, q := newClient(t).do(http.MethodPost, "/api/v1/quotes", token, body)
	require.Equal(t, 201, status, q)
	return q
}

func TestTariffActiveByDate(t *testing.T) {
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")

	status, body := c.do(http.MethodGet, "/api/v1/tariffs/active?product=KASKO&date=2026-06-30", agent, nil)
	require.Equal(t, 200, status, body)
	assert.Equal(t, float64(1), body["version_no"])
	assert.Equal(t, "2026-07-01", body["valid_to"])

	status, body = c.do(http.MethodGet, "/api/v1/tariffs/active?product=KASKO&date=2026-07-01", agent, nil)
	require.Equal(t, 200, status)
	assert.Equal(t, float64(2), body["version_no"])
	assert.Len(t, body["coverages"], 3)
	assert.Len(t, body["short_period_rates"], 5)

	status, _ = c.do(http.MethodGet, "/api/v1/tariffs/active?product=KASKO&date=2024-01-01", agent, nil)
	assert.Equal(t, 404, status)
	status, _ = c.do(http.MethodGet, "/api/v1/tariffs/active?product=UZAY", agent, nil)
	assert.Equal(t, 400, status)
	status, _ = c.do(http.MethodGet, "/api/v1/tariffs/active?product=KASKO", tokenFor(t, "musteri@guvenpolice.test"), nil)
	assert.Equal(t, 403, status)
}

func TestQuoteDeterministicAndPersisted(t *testing.T) {
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	start := futureStart()
	req := kaskoBody(t, ayse, start)

	s1, p1 := c.do(http.MethodPost, "/api/v1/quotes/preview", agent, req)
	s2, p2 := c.do(http.MethodPost, "/api/v1/quotes/preview", agent, req)
	require.Equal(t, 200, s1, p1)
	require.Equal(t, 200, s2)
	assert.Equal(t, p1, p2, "aynı girdi → aynı sonuç (döküm, toplam, hash)")
	assert.Equal(t, num(p1["total_premium_kurus"]), sumLines(p1), "döküm toplamı = prim")

	// Veritabanından yüklenen tarife ile saf motor + örnek tarife aynı sonucu vermeli
	startDate, _ := caldate.Parse(start)
	var sample pricing.Tariff
	for _, tr := range pricing.SampleTariffs() {
		if tr.Product == pricing.Kasko && tr.ValidOn(startDate) {
			sample = tr
		}
	}
	age := caldate.AgeOn(time.Date(1988, 4, 12, 0, 0, 0, 0, time.UTC), startDate)
	ncd := 2
	want, err := pricing.Calculate(sample, pricing.Input{Product: pricing.Kasko,
		Risk:      pricing.Risk{DriverAge: &age, NCDLevel: &ncd, RegionCode: "34", VehicleGroup: "C"},
		Coverages: []string{"KASKO_ANA", "CAM"}})
	require.NoError(t, err)
	assert.Equal(t, want.TotalKurus, num(p1["total_premium_kurus"]))

	q := createQuote(t, agent, req)
	assert.Equal(t, "CALCULATED", q["status"])
	assert.Equal(t, p1["total_premium_kurus"], q["total_premium_kurus"])
	assert.Equal(t, p1["input_hash"], q["input_hash"])
	assert.Equal(t, p1["lines"], q["lines"], "kaydedilen döküm = önizleme dökümü")
	risk := q["risk"].(map[string]any)
	assert.Equal(t, "34ABC123", risk["plate"], "plaka normalize edilir")

	status, got := c.do(http.MethodGet, fmt.Sprintf("/api/v1/quotes/%v", q["id"]), agent, nil)
	require.Equal(t, 200, status)
	assert.Equal(t, q["lines"], got["lines"])
	assert.Equal(t, q["total_premium_kurus"], got["total_premium_kurus"])

	q2 := createQuote(t, agent, req)
	assert.NotEqual(t, q["id"], q2["id"])
	assert.Equal(t, q["input_hash"], q2["input_hash"])
	assert.Equal(t, q["total_premium_kurus"], q2["total_premium_kurus"])

	var audits int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'QUOTE_CREATED' AND entity_id = $1`, fmt.Sprint(q["id"])).Scan(&audits))
	assert.Equal(t, 1, audits)
}

func TestQuoteUsesTariffValidOnTermStart(t *testing.T) {
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")

	_, old := c.do(http.MethodPost, "/api/v1/quotes/preview", agent, kaskoBody(t, ayse, "2026-06-30"))
	_, cur := c.do(http.MethodPost, "/api/v1/quotes/preview", agent, kaskoBody(t, ayse, "2026-07-01"))
	assert.Equal(t, float64(1), old["tariff"].(map[string]any)["version_no"])
	assert.Equal(t, float64(2), cur["tariff"].(map[string]any)["version_no"])
	assert.NotEqual(t, old["total_premium_kurus"], cur["total_premium_kurus"])
	assert.NotEqual(t, old["input_hash"], cur["input_hash"])

	// Geçmiş tarihli teklif kaydedilebilir ama poliçeye çevrilemez
	q := createQuote(t, agent, kaskoBody(t, ayse, "2026-06-30"))
	assert.Equal(t, float64(1), q["tariff"].(map[string]any)["version_no"])
	status, body, _ := c.doH(http.MethodPost, fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"]), agent, nil,
		map[string]string{idempotency.Header: "past-" + fmt.Sprint(q["id"])})
	assert.Equal(t, 422, status)
	assert.Equal(t, "TERM_START_IN_PAST", errCode(body))
}

func TestQuoteValidation(t *testing.T) {
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	base := kaskoBody(t, ayse, futureStart())
	cases := map[string]func(map[string]any){
		"plate":         func(b map[string]any) { b["plate"] = "ABC" },
		"term_start":    func(b map[string]any) { b["term_start"] = "2026-02-29" },
		"product":       func(b map[string]any) { b["product"] = "UZAY" },
		"coverages":     func(b map[string]any) { b["coverages"] = []string{"CAM"} },
		"ncd_level":     func(b map[string]any) { b["ncd_level"] = 9 },
		"vehicle_group": func(b map[string]any) { b["vehicle_group"] = "Z" },
	}
	for field, mutate := range cases {
		b := map[string]any{}
		for k, v := range base {
			b[k] = v
		}
		mutate(b)
		status, body := c.do(http.MethodPost, "/api/v1/quotes/preview", agent, b)
		assert.Equal(t, 400, status, field)
		details, _ := body["error"].(map[string]any)["details"].(map[string]any)
		assert.Contains(t, details, field, field)
	}
	b := kaskoBody(t, ayse, "2024-06-01")
	status, body := c.do(http.MethodPost, "/api/v1/quotes/preview", agent, b)
	assert.Equal(t, 422, status)
	assert.Equal(t, "NO_TARIFF", errCode(body))

	// KONUT: plaka/araç grubu gerekmez
	status, body = c.do(http.MethodPost, "/api/v1/quotes/preview", agent, map[string]any{
		"customer_id": customerID(t, ayse), "product": "KONUT", "term_start": futureStart(),
		"ncd_level": 1, "coverages": []string{"YANGIN"}})
	require.Equal(t, 200, status, body)
	assert.Len(t, body["lines"], 3, "taban + NCD + bölge")
}

func TestUnderwriterReviewFlow(t *testing.T) {
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	uw := tokenFor(t, "underwriter@guvenpolice.test")

	q := createQuote(t, agent, kaskoBody(t, can, futureStart()))
	require.Equal(t, "UW_REVIEW", q["status"])
	assert.Contains(t, q["risk"].(map[string]any)["review_reasons"], pricing.ReasonYoungDriver)
	path := fmt.Sprintf("/api/v1/quotes/%v", q["id"])

	status, body, _ := c.doH(http.MethodPost, path+"/convert", agent, nil, map[string]string{idempotency.Header: "uw-early-" + fmt.Sprint(q["id"])})
	assert.Equal(t, 409, status)
	assert.Equal(t, "QUOTE_NOT_CONVERTIBLE", errCode(body))

	status, _ = c.do(http.MethodPost, path+"/review", agent, map[string]any{"decision": "APPROVE"})
	assert.Equal(t, 403, status, "acente onaylayamaz")

	status, body = c.do(http.MethodPost, path+"/review", uw, map[string]any{"decision": "REJECT"})
	assert.Equal(t, 400, status, "red gerekçesiz olmaz")

	status, body = c.do(http.MethodGet, "/api/v1/quotes?status=UW_REVIEW", uw, nil)
	require.Equal(t, 200, status)
	assert.NotEmpty(t, body["items"])

	status, body = c.do(http.MethodPost, path+"/review", uw, map[string]any{"decision": "APPROVE", "note": "ehliyet 2 yıllık, uygun"})
	require.Equal(t, 200, status, body)
	assert.Equal(t, "APPROVED", body["status"])
	assert.Equal(t, "ehliyet 2 yıllık, uygun", body["underwriter_note"])

	status, body = c.do(http.MethodPost, path+"/review", uw, map[string]any{"decision": "REJECT", "note": "x"})
	assert.Equal(t, 409, status)
	assert.Equal(t, "QUOTE_INVALID_STATE", errCode(body))

	status, body, _ = c.doH(http.MethodPost, path+"/convert", agent, nil, map[string]string{idempotency.Header: "uw-ok-" + fmt.Sprint(q["id"])})
	require.Equal(t, 201, status, body)

	var audits int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'QUOTE_REVIEWED' AND entity_id = $1 AND actor_role = 'UNDERWRITER'`,
		fmt.Sprint(q["id"])).Scan(&audits))
	assert.Equal(t, 1, audits)
}

func TestConvertCreatesPolicyAndIsIdempotent(t *testing.T) {
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	path := fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"])
	key := map[string]string{idempotency.Header: "conv-" + fmt.Sprint(q["id"])}

	status, body, _ := c.doH(http.MethodPost, path, agent, nil, nil)
	assert.Equal(t, 400, status)
	assert.Equal(t, "IDEMPOTENCY_KEY_REQUIRED", errCode(body))

	status, first, h1 := c.doH(http.MethodPost, path, agent, nil, key)
	require.Equal(t, 201, status, first)
	assert.Empty(t, h1.Get(idempotency.ReplayedHeader))
	assert.Regexp(t, `^GP-KS-\d{4}-\d{8}$`, first["policy_no"])
	assert.Equal(t, q["total_premium_kurus"], first["annual_premium_kurus"])
	assert.Len(t, first["coverages"], 2)
	assert.Len(t, first["limit_balances"], 3, "2 teminat + yıllık toplam")

	status, again, h2 := c.doH(http.MethodPost, path, agent, nil, key)
	assert.Equal(t, 201, status)
	assert.Equal(t, "true", h2.Get(idempotency.ReplayedHeader))
	assert.Equal(t, first, again, "aynı anahtar → aynı yanıt")

	// Aynı anahtar başka bir istekte → 422
	other := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	status, body, _ = c.doH(http.MethodPost, fmt.Sprintf("/api/v1/quotes/%v/convert", other["id"]), agent, nil, key)
	assert.Equal(t, 422, status)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", errCode(body))

	// Farklı anahtarla tekrar → iş kuralı: zaten çevrilmiş
	status, body, _ = c.doH(http.MethodPost, path, agent, nil, map[string]string{idempotency.Header: "conv2-" + fmt.Sprint(q["id"])})
	assert.Equal(t, 409, status)
	assert.Equal(t, "QUOTE_NOT_CONVERTIBLE", errCode(body))

	ctx := context.Background()
	var policies, versions, coverages, balances int
	var quoteStatus string
	var annualLimit, annualUsed int64
	pid := num(first["id"])
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM policies WHERE quote_id = $1`, num(q["id"])).Scan(&policies))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM policy_versions WHERE policy_id = $1 AND version_no = 1 AND effective_to IS NULL`, pid).Scan(&versions))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM policy_version_coverages pvc JOIN policy_versions v ON v.id = pvc.policy_version_id WHERE v.policy_id = $1`, pid).Scan(&coverages))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM policy_limit_balances WHERE policy_id = $1`, pid).Scan(&balances))
	require.NoError(t, pool.QueryRow(ctx, `SELECT limit_kurus, used_kurus FROM policy_limit_balances WHERE policy_id = $1 AND coverage_code IS NULL`, pid).Scan(&annualLimit, &annualUsed))
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM quotes WHERE id = $1`, num(q["id"])).Scan(&quoteStatus))
	assert.Equal(t, 1, policies)
	assert.Equal(t, 1, versions)
	assert.Equal(t, 2, coverages)
	assert.Equal(t, 3, balances)
	assert.Equal(t, pricing.Catalog[pricing.Kasko].AnnualLimitKurus, annualLimit)
	assert.Zero(t, annualUsed)
	assert.Equal(t, "CONVERTED", quoteStatus)

	var issued int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'POLICY_ISSUED' AND entity_id = $1`, fmt.Sprint(pid)).Scan(&issued))
	assert.Equal(t, 1, issued)

	// Teklif artık poliçeye bağlı görünür
	status, got := c.do(http.MethodGet, fmt.Sprintf("/api/v1/quotes/%v", q["id"]), agent, nil)
	require.Equal(t, 200, status)
	assert.Equal(t, first["id"], got["policy_id"])
}

// Farklı Idempotency-Key'lerle eşzamanlı çevirme: teklif satırı kilidi tek poliçeye izin verir.
func TestConcurrentConvertDifferentKeys(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	path := fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"])

	const n = 5
	codes := runParallel(t, n, func(i int) int {
		s, _, _ := newClient(t).doH(http.MethodPost, path, agent, nil,
			map[string]string{idempotency.Header: fmt.Sprintf("par-%v-%d", q["id"], i)})
		return s
	})
	assert.Equal(t, 1, count(codes, 201), codes)
	assert.Equal(t, n-1, count(codes, 409), codes)
	assertPolicyCount(t, num(q["id"]), 1)
}

// Aynı anahtarla eşzamanlı 5 istek → tek iş etkisi.
func TestConcurrentConvertSameKey(t *testing.T) {
	agent := tokenFor(t, "acente@guvenpolice.test")
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	path := fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"])
	key := fmt.Sprintf("same-%v", q["id"])

	codes := runParallel(t, 5, func(int) int {
		s, _, _ := newClient(t).doH(http.MethodPost, path, agent, nil, map[string]string{idempotency.Header: key})
		return s
	})
	assert.GreaterOrEqual(t, count(codes, 201), 1, codes)
	assert.Equal(t, 5, count(codes, 201)+count(codes, 409), codes)
	assertPolicyCount(t, num(q["id"]), 1)

	// İşlem bittikten sonra aynı anahtar her zaman kayıtlı yanıtı döner
	s, _, h := newClient(t).doH(http.MethodPost, path, agent, nil, map[string]string{idempotency.Header: key})
	assert.Equal(t, 201, s)
	assert.Equal(t, "true", h.Get(idempotency.ReplayedHeader))
}

// Kilit atlanmış bir yarışı simüle eder: teklif hâlâ CALCULATED iken poliçesi zaten var.
// policies.quote_id UNIQUE ihlali 500 değil 409 QUOTE_ALREADY_CONVERTED olmalı.
func TestConvertUniqueViolationIsConflict(t *testing.T) {
	ctx := context.Background()
	agent := tokenFor(t, "acente@guvenpolice.test")
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	qid := num(q["id"])
	_, err := pool.Exec(ctx, `
		INSERT INTO policies (policy_no, product, customer_id, agent_id, quote_id, start_date, end_date)
		SELECT 'GP-RACE-' || id, product, customer_id, agent_id, id, term_start, term_end FROM quotes WHERE id = $1`, qid)
	require.NoError(t, err)

	s, body, _ := newClient(t).doH(http.MethodPost, fmt.Sprintf("/api/v1/quotes/%d/convert", qid), agent, nil,
		map[string]string{idempotency.Header: fmt.Sprintf("race-%d", qid)})
	assert.Equal(t, 409, s, body)
	assert.Equal(t, "QUOTE_ALREADY_CONVERTED", errCode(body))
	assertPolicyCount(t, qid, 1)
}

func TestIdempotencyStaleInProgressIsTakenOver(t *testing.T) {
	ctx := context.Background()
	agent := tokenFor(t, "acente@guvenpolice.test")
	var agentUserID int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM users WHERE email = 'acente@guvenpolice.test'`).Scan(&agentUserID))

	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	path := fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"])
	hash := idempotency.RequestHash(http.MethodPost, path, nil)

	// Taze IN_PROGRESS → 409
	_, err := pool.Exec(ctx, `INSERT INTO idempotency_keys (user_id, idem_key, method, path, request_hash, status)
		VALUES ($1, 'fresh-'||$2, 'POST', $3, $4, 'IN_PROGRESS')`, agentUserID, fmt.Sprint(q["id"]), path, hash)
	require.NoError(t, err)
	s, body, _ := newClient(t).doH(http.MethodPost, path, agent, nil, map[string]string{idempotency.Header: "fresh-" + fmt.Sprint(q["id"])})
	assert.Equal(t, 409, s)
	assert.Equal(t, "IDEMPOTENCY_IN_PROGRESS", errCode(body))

	// Süresi geçmiş (çökmüş istek) IN_PROGRESS → devralınır ve işlenir
	_, err = pool.Exec(ctx, `INSERT INTO idempotency_keys (user_id, idem_key, method, path, request_hash, status, created_at)
		VALUES ($1, 'stale-'||$2, 'POST', $3, $4, 'IN_PROGRESS', now() - interval '5 minutes')`,
		agentUserID, fmt.Sprint(q["id"]), path, hash)
	require.NoError(t, err)
	s, body, _ = newClient(t).doH(http.MethodPost, path, agent, nil, map[string]string{idempotency.Header: "stale-" + fmt.Sprint(q["id"])})
	assert.Equal(t, 201, s, body)
}

func TestQuoteRBACAndOwnership(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	qPath := fmt.Sprintf("/api/v1/quotes/%v", q["id"])

	// Underwriter teklif oluşturamaz
	status, _ := c.do(http.MethodPost, "/api/v1/quotes", tokenFor(t, "underwriter@guvenpolice.test"), kaskoBody(t, ayse, futureStart()))
	assert.Equal(t, 403, status)
	for _, email := range []string{"eksper@guvenpolice.test", "hasar@guvenpolice.test", "muhasebe@guvenpolice.test"} {
		status, _ = c.do(http.MethodGet, "/api/v1/quotes", tokenFor(t, email), nil)
		assert.Equal(t, 403, status, email)
	}

	// Başka acente: kendi acentesine ait olmayan teklifi göremez/çeviremez (404), müşteriye teklif veremez
	hash, err := bcrypt.GenerateFromPassword([]byte(demoPassword), bcrypt.MinCost)
	require.NoError(t, err)
	var otherUser int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, full_name, role)
		VALUES ('acente2@guvenpolice.test', $1, 'Diğer Acente', 'AGENT')
		ON CONFLICT ((lower(email))) DO UPDATE SET full_name = EXCLUDED.full_name RETURNING id`, string(hash)).Scan(&otherUser))
	_, err = pool.Exec(ctx, `INSERT INTO agents (user_id, agency_name) VALUES ($1, 'Diğer Acente') ON CONFLICT (user_id) DO NOTHING`, otherUser)
	require.NoError(t, err)
	other := tokenFor(t, "acente2@guvenpolice.test")

	status, body := c.do(http.MethodGet, qPath, other, nil)
	assert.Equal(t, 404, status)
	assert.Equal(t, "NOT_FOUND", errCode(body))
	status, _, _ = c.doH(http.MethodPost, qPath+"/convert", other, nil, map[string]string{idempotency.Header: "other-" + fmt.Sprint(q["id"])})
	assert.Equal(t, 404, status)
	status, body = c.do(http.MethodGet, "/api/v1/quotes", other, nil)
	require.Equal(t, 200, status)
	assert.Empty(t, body["items"])
	status, _ = c.do(http.MethodPost, "/api/v1/quotes/preview", other, kaskoBody(t, ayse, futureStart()))
	assert.Equal(t, 404, status, "başka acentenin müşterisi")

	// Müşteri: yalnızca kendi teklifleri
	customer := tokenFor(t, "musteri@guvenpolice.test")
	status, _ = c.do(http.MethodGet, qPath, customer, nil)
	assert.Equal(t, 200, status, "Ayşe kendi teklifini görür")
	canQuote := createQuote(t, agent, kaskoBody(t, can, futureStart()))
	status, _ = c.do(http.MethodGet, fmt.Sprintf("/api/v1/quotes/%v", canQuote["id"]), customer, nil)
	assert.Equal(t, 404, status, "başka müşterinin teklifi")
	status, body = c.do(http.MethodGet, "/api/v1/quotes?limit=100", customer, nil)
	require.Equal(t, 200, status)
	for _, it := range body["items"].([]any) {
		assert.Equal(t, float64(customerID(t, ayse)), it.(map[string]any)["customer_id"])
	}
	status, _ = c.do(http.MethodPost, "/api/v1/quotes/preview", customer, kaskoBody(t, ayse, futureStart()))
	assert.Equal(t, 403, status)
}

func TestPolicyListScope(t *testing.T) {
	c := newClient(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	status, pol, _ := c.doH(http.MethodPost, fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"]), agent, nil,
		map[string]string{idempotency.Header: "list-" + fmt.Sprint(q["id"])})
	require.Equal(t, 201, status, pol)

	status, body := c.do(http.MethodGet, "/api/v1/policies?limit=100", tokenFor(t, "musteri@guvenpolice.test"), nil)
	require.Equal(t, 200, status)
	found := false
	for _, it := range body["items"].([]any) {
		m := it.(map[string]any)
		assert.Equal(t, float64(customerID(t, ayse)), m["customer_id"])
		if m["policy_no"] == pol["policy_no"] {
			found = true
			assert.Equal(t, "34ABC123", m["vehicle_plate"])
		}
	}
	assert.True(t, found)

	for _, email := range []string{"underwriter@guvenpolice.test", "hasar@guvenpolice.test", "muhasebe@guvenpolice.test"} {
		status, _ = c.do(http.MethodGet, "/api/v1/policies", tokenFor(t, email), nil)
		assert.Equal(t, 200, status, email)
	}
	status, _ = c.do(http.MethodGet, "/api/v1/policies", tokenFor(t, "eksper@guvenpolice.test"), nil)
	assert.Equal(t, 403, status)

	// Keyset: limit=1 ile iki sayfa farklı kayıt döner
	uw := tokenFor(t, "underwriter@guvenpolice.test")
	_, p1 := c.do(http.MethodGet, "/api/v1/policies?limit=1", uw, nil)
	cursor, _ := p1["next_cursor"].(string)
	require.NotEmpty(t, cursor)
	_, p2 := c.do(http.MethodGet, "/api/v1/policies?limit=1&cursor="+cursor, uw, nil)
	require.Len(t, p2["items"], 1)
	assert.NotEqual(t, p1["items"].([]any)[0].(map[string]any)["id"], p2["items"].([]any)[0].(map[string]any)["id"])
	status, _ = c.do(http.MethodGet, "/api/v1/policies?cursor=bozuk", uw, nil)
	assert.Equal(t, 400, status)
}

func runParallel(t *testing.T, n int, f func(i int) int) []int {
	t.Helper()
	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i] = f(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return codes
}

func count(codes []int, v int) int {
	n := 0
	for _, c := range codes {
		if c == v {
			n++
		}
	}
	return n
}

func assertPolicyCount(t *testing.T, quoteID int64, want int) {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM policies WHERE quote_id = $1`, quoteID).Scan(&n))
	assert.Equal(t, want, n)
}
