// Package quote teklif hesaplama/kaydetme, underwriter onayı ve poliçeye çevirme işlemleridir (F-1, T-1).
package quote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
)

// Teklif durumları (quotes.status CHECK ile aynı).
const (
	StatusCalculated = "CALCULATED"
	StatusUWReview   = "UW_REVIEW"
	StatusApproved   = "APPROVED"
	StatusRejected   = "REJECTED"
	StatusConverted  = "CONVERTED"
	StatusExpired    = "EXPIRED"
)

var validStatuses = map[string]bool{StatusCalculated: true, StatusUWReview: true, StatusApproved: true,
	StatusRejected: true, StatusConverted: true, StatusExpired: true}

// QuoteValidity teklifin geçerlilik süresi (varsayım).
const QuoteValidity = 30 * 24 * time.Hour

type Request struct {
	CustomerID   int64    `json:"customer_id"`
	Product      string   `json:"product"`
	TermStart    string   `json:"term_start"`
	NCDLevel     *int     `json:"ncd_level"`
	VehicleGroup string   `json:"vehicle_group"`
	Plate        string   `json:"plate"`
	Coverages    []string `json:"coverages"`
}

// RiskData quotes.risk_data içeriği (poliçe sürümüyle aynı biçim).
type RiskData = policy.RiskData

// Selection quotes.coverage_selection içeriği: seçilen teminatların limit/muafiyet/prim bilgisi.
type Selection struct {
	Coverages        []policy.CoverageTerm `json:"coverages"`
	AnnualLimitKurus int64                 `json:"annual_limit_kurus"`
}

type TariffRef struct {
	ID        int64  `json:"id"`
	VersionNo int    `json:"version_no"`
	ValidFrom string `json:"valid_from"`
}

// Calculation preview ve kayıt öncesi hesaplanan teklif.
type Calculation struct {
	CustomerID     int64          `json:"customer_id"`
	AgentID        *int64         `json:"agent_id"`
	Product        string         `json:"product"`
	Tariff         TariffRef      `json:"tariff"`
	TermStart      string         `json:"term_start"`
	TermEnd        string         `json:"term_end"`
	Risk           RiskData       `json:"risk"`
	Selection      Selection      `json:"selection"`
	Lines          []pricing.Line `json:"lines"`
	TotalPremium   int64          `json:"total_premium_kurus"`
	InputHash      string         `json:"input_hash"`
	ReviewRequired bool           `json:"review_required"`
	ReviewReasons  []string       `json:"review_reasons"`

	termStart, termEnd time.Time
}

type Quote struct {
	ID              int64           `json:"id"`
	CustomerID      int64           `json:"customer_id"`
	CustomerName    string          `json:"customer_name"`
	AgentID         *int64          `json:"agent_id"`
	Product         string          `json:"product"`
	Tariff          TariffRef       `json:"tariff"`
	TermStart       string          `json:"term_start"`
	TermEnd         string          `json:"term_end"`
	Risk            json.RawMessage `json:"risk"`
	Selection       json.RawMessage `json:"selection"`
	InputHash       string          `json:"input_hash"`
	TotalPremium    int64           `json:"total_premium_kurus"`
	Status          string          `json:"status"`
	UnderwriterID   *int64          `json:"underwriter_id"`
	UnderwriterNote *string         `json:"underwriter_note"`
	ValidUntil      *time.Time      `json:"valid_until"`
	CreatedAt       time.Time       `json:"created_at"`
	PolicyID        *int64          `json:"policy_id"`
	Lines           []pricing.Line  `json:"lines,omitempty"`
}

type Service struct {
	pool          *pgxpool.Pool
	now           func() time.Time
	uwThresholdKr int64
}

func NewService(pool *pgxpool.Pool, now func() time.Time, uwThresholdKurus int64) *Service {
	return &Service{pool: pool, now: now, uwThresholdKr: uwThresholdKurus}
}

func (s *Service) scope(ctx context.Context) (access.Scope, auth.Principal, error) {
	p, _ := auth.FromContext(ctx)
	sc, err := access.Resolve(ctx, s.pool, p)
	return sc, p, access.AsHTTP(err)
}

// Calculate isteği doğrular, müşteriyi ve başlangıç tarihine göre tarifeyi yükler ve primi hesaplar.
func (s *Service) Calculate(ctx context.Context, req Request) (Calculation, error) {
	sc, _, err := s.scope(ctx)
	if err != nil {
		return Calculation{}, err
	}
	product := pricing.Product(strings.ToUpper(strings.TrimSpace(req.Product)))
	details := map[string]string{}
	if req.CustomerID <= 0 {
		details["customer_id"] = "zorunlu"
	}
	if !product.Valid() {
		details["product"] = "KASKO, TRAFIK veya KONUT olmalı"
	}
	start, err := caldate.Parse(req.TermStart)
	if err != nil {
		details["term_start"] = err.Error()
	}
	plate, plateOK := policy.NormalizePlate(req.Plate)
	if product.IsVehicle() && !plateOK {
		details["plate"] = "geçerli bir plaka olmalı (ör. 34ABC123)"
	}
	if !product.IsVehicle() {
		plate = ""
	}
	if len(details) > 0 {
		return Calculation{}, httpx.Invalid(details)
	}

	var birth time.Time
	var region string
	var agentID *int64
	err = s.pool.QueryRow(ctx, `SELECT birth_date, region_code, agent_id FROM customers WHERE id = $1`, req.CustomerID).
		Scan(&birth, &region, &agentID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !sc.Allows(agentID, req.CustomerID)) {
		return Calculation{}, httpx.NotFound("müşteri")
	}
	if err != nil {
		return Calculation{}, err
	}

	t, err := tariff.Select(ctx, s.pool, product, start)
	if errors.Is(err, pricing.ErrNoTariff) {
		return Calculation{}, httpx.NewError(http.StatusUnprocessableEntity, "NO_TARIFF",
			fmt.Sprintf("%s için %s tarihinde geçerli tarife yok", product, req.TermStart))
	}
	if err != nil {
		return Calculation{}, err
	}

	age := caldate.AgeOn(birth, start)
	res, err := pricing.Calculate(t, pricing.Input{
		Product: product,
		Risk: pricing.Risk{DriverAge: &age, NCDLevel: req.NCDLevel, RegionCode: region,
			VehicleGroup: req.VehicleGroup},
		Coverages: req.Coverages,
	})
	var ie *pricing.InputError
	if errors.As(err, &ie) {
		return Calculation{}, httpx.Invalid(map[string]string{ie.Field: ie.Message})
	}
	if err != nil {
		return Calculation{}, err
	}

	reasons := pricing.ReviewReasons(res, s.uwThresholdKr)
	end := caldate.AddYear(start)
	c := Calculation{
		CustomerID: req.CustomerID, AgentID: agentID, Product: string(product),
		Tariff:    TariffRef{ID: t.ID, VersionNo: t.VersionNo, ValidFrom: caldate.Format(t.ValidFrom)},
		TermStart: caldate.Format(start), TermEnd: caldate.Format(end),
		Risk:      RiskData{Risk: res.Risk, Plate: plate, BirthDate: caldate.Format(birth), ReviewReasons: reasons},
		Selection: Selection{AnnualLimitKurus: pricing.Catalog[product].AnnualLimitKurus},
		Lines:     res.Lines, TotalPremium: res.TotalKurus, InputHash: res.InputHash,
		ReviewRequired: len(reasons) > 0, ReviewReasons: reasons,
		termStart: start, termEnd: end,
	}
	if c.ReviewReasons == nil {
		c.ReviewReasons = []string{}
	}
	for _, cp := range res.Coverages {
		spec, _ := pricing.CoverageSpecFor(product, cp.Code)
		c.Selection.Coverages = append(c.Selection.Coverages, policy.CoverageTerm{
			Code: cp.Code, Name: spec.Name, LimitKurus: spec.LimitKurus,
			EventLimitKurus: spec.EventLimitKurus,
			DeductibleType:  spec.DeductibleType, DeductibleValue: spec.DeductibleValue, PremiumKurus: cp.PremiumKurus,
		})
	}
	return c, nil
}

// Create hesaplar ve teklifi döküm satırlarıyla tek transaction'da kaydeder.
func (s *Service) Create(ctx context.Context, req Request) (Quote, error) {
	c, err := s.Calculate(ctx, req)
	if err != nil {
		return Quote{}, err
	}
	p, _ := auth.FromContext(ctx)
	status := StatusCalculated
	if c.ReviewRequired {
		status = StatusUWReview
	}
	riskJSON, _ := json.Marshal(c.Risk)
	selJSON, _ := json.Marshal(c.Selection)

	var id int64
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO quotes (customer_id, agent_id, created_by, product, tariff_version_id, term_start, term_end,
			                    risk_data, coverage_selection, input_hash, total_premium_kurus, status, valid_until)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
			c.CustomerID, c.AgentID, p.UserID, c.Product, c.Tariff.ID, c.termStart, c.termEnd,
			riskJSON, selJSON, c.InputHash, c.TotalPremium, status, s.now().Add(QuoteValidity)).Scan(&id)
		if err != nil {
			return err
		}
		rows := make([][]any, 0, len(c.Lines))
		for _, l := range c.Lines {
			var cov, ft, label *string
			var bp *int64
			if l.CoverageCode != "" {
				cov = &l.CoverageCode
			}
			if l.FactorType != "" {
				f := string(l.FactorType)
				ft = &f
			}
			if l.Label != "" {
				label = &l.Label
			}
			if l.MultiplierBP != 0 {
				bp = &l.MultiplierBP
			}
			rows = append(rows, []any{id, l.Seq, cov, string(l.Type), ft, label, bp, l.AmountKurus})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"quote_premium_lines"},
			[]string{"quote_id", "seq", "coverage_code", "line_type", "factor_type", "factor_label", "multiplier_bp", "amount_kurus"},
			pgx.CopyFromRows(rows)); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "QUOTE_CREATED", EntityType: "quote", EntityID: id,
			New: map[string]any{"status": status, "total_premium_kurus": c.TotalPremium,
				"tariff_version_id": c.Tariff.ID, "input_hash": c.InputHash}})
	})
	if err != nil {
		return Quote{}, err
	}
	return s.Get(ctx, id)
}

const quoteCols = `
	q.id, q.customer_id, c.full_name, q.agent_id, q.product, q.tariff_version_id, tv.version_no,
	to_char(tv.valid_from, 'YYYY-MM-DD'), to_char(q.term_start, 'YYYY-MM-DD'), to_char(q.term_end, 'YYYY-MM-DD'),
	q.risk_data, q.coverage_selection, q.input_hash, q.total_premium_kurus, q.status, q.underwriter_id,
	q.underwriter_note, q.valid_until, q.created_at, p.id`

const quoteFrom = `
	FROM quotes q
	JOIN customers c ON c.id = q.customer_id
	JOIN tariff_versions tv ON tv.id = q.tariff_version_id
	LEFT JOIN policies p ON p.quote_id = q.id`

func scanQuote(row pgx.Row) (Quote, error) {
	var q Quote
	err := row.Scan(&q.ID, &q.CustomerID, &q.CustomerName, &q.AgentID, &q.Product, &q.Tariff.ID, &q.Tariff.VersionNo,
		&q.Tariff.ValidFrom, &q.TermStart, &q.TermEnd, &q.Risk, &q.Selection, &q.InputHash, &q.TotalPremium,
		&q.Status, &q.UnderwriterID, &q.UnderwriterNote, &q.ValidUntil, &q.CreatedAt, &q.PolicyID)
	return q, err
}

// Get teklifi döküm satırlarıyla döner; kapsam dışıysa 404.
func (s *Service) Get(ctx context.Context, id int64) (Quote, error) {
	sc, _, err := s.scope(ctx)
	if err != nil {
		return Quote{}, err
	}
	q, err := scanQuote(s.pool.QueryRow(ctx, `SELECT `+quoteCols+quoteFrom+` WHERE q.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !sc.Allows(q.AgentID, q.CustomerID)) {
		return Quote{}, httpx.NotFound("teklif")
	}
	if err != nil {
		return Quote{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT seq, COALESCE(coverage_code, ''), line_type, COALESCE(factor_type, ''), COALESCE(factor_label, ''),
		       COALESCE(multiplier_bp, 0), amount_kurus
		  FROM quote_premium_lines WHERE quote_id = $1 ORDER BY seq`, id)
	if err != nil {
		return Quote{}, err
	}
	q.Lines, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (pricing.Line, error) {
		var l pricing.Line
		var typ, ft string
		err := r.Scan(&l.Seq, &l.CoverageCode, &typ, &ft, &l.Label, &l.MultiplierBP, &l.AmountKurus)
		l.Type, l.FactorType = pricing.LineType(typ), pricing.FactorType(ft)
		return l, err
	})
	return q, err
}

// List keyset sayfalı, en yeni önce. status boşsa filtre yok.
func (s *Service) List(ctx context.Context, params page.Params, status string) ([]Quote, error) {
	sc, _, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	var st *string
	if status != "" {
		st = &status
	}
	rows, err := s.pool.Query(ctx, `SELECT `+quoteCols+quoteFrom+`
		 WHERE ($1::bigint = 0 OR q.id < $1)
		   AND ($2::bigint IS NULL OR q.agent_id = $2)
		   AND ($3::bigint IS NULL OR q.customer_id = $3)
		   AND ($4::text IS NULL OR q.status = $4)
		 ORDER BY q.id DESC LIMIT $5`, params.AfterID, sc.AgentID, sc.CustomerID, st, params.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Quote, error) { return scanQuote(r) })
}

type ReviewRequest struct {
	Decision string `json:"decision"` // APPROVE | REJECT
	Note     string `json:"note"`
}

// Review UW_REVIEW teklifini onaylar/reddeder (yalnızca underwriter; rol router'da denetlenir).
func (s *Service) Review(ctx context.Context, id int64, req ReviewRequest) (Quote, error) {
	p, _ := auth.FromContext(ctx)
	req.Note = strings.TrimSpace(req.Note)
	var newStatus string
	switch strings.ToUpper(req.Decision) {
	case "APPROVE":
		newStatus = StatusApproved
	case "REJECT":
		newStatus = StatusRejected
		if req.Note == "" {
			return Quote{}, httpx.Invalid(map[string]string{"note": "red gerekçesi zorunlu"})
		}
	default:
		return Quote{}, httpx.Invalid(map[string]string{"decision": "APPROVE veya REJECT olmalı"})
	}
	if len(req.Note) > 1000 {
		return Quote{}, httpx.Invalid(map[string]string{"note": "en fazla 1000 karakter"})
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM quotes WHERE id = $1 FOR UPDATE`, id).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("teklif")
		}
		if err != nil {
			return err
		}
		if status != StatusUWReview {
			return httpx.NewError(http.StatusConflict, "QUOTE_INVALID_STATE",
				fmt.Sprintf("yalnızca UW_REVIEW teklif değerlendirilebilir (mevcut: %s)", status))
		}
		var note *string
		if req.Note != "" {
			note = &req.Note
		}
		if _, err := tx.Exec(ctx,
			`UPDATE quotes SET status = $2, underwriter_id = $3, underwriter_note = $4 WHERE id = $1`,
			id, newStatus, p.UserID, note); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "QUOTE_REVIEWED", EntityType: "quote", EntityID: id,
			Old: map[string]any{"status": status}, New: map[string]any{"status": newStatus, "note": note}})
	})
	if err != nil {
		return Quote{}, err
	}
	return s.Get(ctx, id)
}

// Convert teklifi tek transaction'da poliçeye çevirir. Teklif satırı FOR UPDATE ile kilitlenir:
// eşzamanlı iki çevirmede ikincisi CONVERTED durumunu görür ve 409 alır; policies.quote_id UNIQUE
// ikinci bir güvencedir.
func (s *Service) Convert(ctx context.Context, id int64, installmentCount ...int) (policy.Issued, error) {
	sc, _, err := s.scope(ctx)
	if err != nil {
		return policy.Issued{}, err
	}
	count := 1
	if len(installmentCount) > 0 {
		count = installmentCount[0]
	}
	var out policy.Issued
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			customerID, tariffID, total int64
			agentID                     *int64
			product, status             string
			start, end                  time.Time
			riskJSON, selJSON           []byte
			validUntil                  *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT customer_id, agent_id, product, tariff_version_id, term_start, term_end, risk_data,
			       coverage_selection, total_premium_kurus, status, valid_until
			  FROM quotes WHERE id = $1 FOR UPDATE`, id).
			Scan(&customerID, &agentID, &product, &tariffID, &start, &end, &riskJSON, &selJSON, &total, &status, &validUntil)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !sc.Allows(agentID, customerID)) {
			return httpx.NotFound("teklif")
		}
		if err != nil {
			return err
		}
		if status != StatusCalculated && status != StatusApproved {
			return httpx.NewError(http.StatusConflict, "QUOTE_NOT_CONVERTIBLE",
				fmt.Sprintf("yalnızca CALCULATED veya APPROVED teklif poliçeye çevrilebilir (mevcut: %s)", status))
		}
		now := s.now()
		if validUntil != nil && !now.Before(*validUntil) {
			return httpx.NewError(http.StatusUnprocessableEntity, "QUOTE_EXPIRED", "teklifin geçerlilik süresi dolmuş")
		}
		if start.Before(caldate.Today(now)) {
			return httpx.NewError(http.StatusUnprocessableEntity, "TERM_START_IN_PAST",
				"geriye dönük poliçe düzenlenemez; başlangıç tarihi bugün veya sonrası olmalı")
		}
		var risk RiskData
		var sel Selection
		if err := json.Unmarshal(riskJSON, &risk); err != nil {
			return err
		}
		if err := json.Unmarshal(selJSON, &sel); err != nil {
			return err
		}
		var plate *string
		if risk.Plate != "" {
			plate = &risk.Plate
		}
		out, err = policy.Issue(ctx, tx, policy.IssueInput{
			QuoteID: id, Product: pricing.Product(product), CustomerID: customerID, AgentID: agentID,
			TariffVersionID: tariffID, StartDate: start, EndDate: end, Plate: plate, RiskData: riskJSON,
			AnnualPremium: total, AnnualLimitKurus: sel.AnnualLimitKurus, Coverages: sel.Coverages,
			InstallmentCount: count,
		})
		// İkinci savunma hattı: kilit atlanmış olsa bile aynı teklifin ikinci poliçesi
		// policies.quote_id UNIQUE'a takılır; 500 yerine iş hatası döner.
		if db.Violates(err, "policies_quote_id_key", "policies_policy_no_key") {
			return httpx.NewError(http.StatusConflict, "QUOTE_ALREADY_CONVERTED", "teklif zaten poliçeye çevrilmiş")
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE quotes SET status = $2 WHERE id = $1`, id, StatusConverted); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "QUOTE_CONVERTED", EntityType: "quote", EntityID: id,
			Old: map[string]any{"status": status}, New: map[string]any{"status": StatusConverted, "policy_id": out.ID}}); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "POLICY_ISSUED", EntityType: "policy", EntityID: out.ID,
			New: map[string]any{"policy_no": out.PolicyNo, "quote_id": id, "annual_premium_kurus": total,
				"start_date": out.StartDate, "end_date": out.EndDate}})
	})
	return out, err
}
