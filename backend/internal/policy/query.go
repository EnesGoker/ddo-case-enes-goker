package policy

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

var (
	ErrNotFound   = errors.New("poliçe bulunamadı")
	ErrNoCoverage = errors.New("bu tarihte geçerli teminat yok")
)

// Header poliçenin özet satırıdır (kapsam kontrolü ve işlemler için).
type Header struct {
	ID                  int64      `json:"id"`
	PolicyNo            string     `json:"policy_no"`
	Product             string     `json:"product"`
	CustomerID          int64      `json:"customer_id"`
	CustomerName        string     `json:"customer_name"`
	AgentID             *int64     `json:"agent_id"`
	QuoteID             *int64     `json:"quote_id"`
	StartDate           string     `json:"start_date"`
	EndDate             string     `json:"end_date"`
	PeriodDays          int64      `json:"period_days"`
	Status              string     `json:"status"`
	CancelledAt         *time.Time `json:"cancelled_at"`
	CancelEffectiveDate *string    `json:"cancel_effective_date"`
	CancelReason        *string    `json:"cancel_reason"`

	start, end time.Time
}

type VersionView struct {
	ID              int64           `json:"id"`
	VersionNo       int             `json:"version_no"`
	EffectiveFrom   string          `json:"effective_from"`
	EffectiveTo     *string         `json:"effective_to"`
	TariffVersionID int64           `json:"tariff_version_id"`
	TariffVersionNo int             `json:"tariff_version_no"`
	VehiclePlate    *string         `json:"vehicle_plate"`
	Risk            json.RawMessage `json:"risk"`
	AnnualPremium   int64           `json:"annual_premium_kurus"`
	AnnualLimit     *int64          `json:"annual_limit_kurus"`
	RecordedAt      time.Time       `json:"recorded_at"`
	EndorsementNo   *int            `json:"endorsement_no"` // bu sürümü açan zeyil (ilk sürümde null)
	Coverages       []CoverageTerm  `json:"coverages"`
}

type EndorsementView struct {
	ID            int64           `json:"id"`
	EndorsementNo int             `json:"endorsement_no"`
	Type          string          `json:"type"`
	EffectiveDate string          `json:"effective_date"`
	FromVersionID int64           `json:"from_version_id"`
	ToVersionID   *int64          `json:"to_version_id"`
	PremiumDelta  int64           `json:"premium_delta_kurus"`
	CalcDetail    json.RawMessage `json:"calc_detail"`
	CreatedBy     int64           `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
}

// PremiumSummary yazılmış prim = ilk prim + zeyil farkları; net = yazılmış − iptal iadesi.
type PremiumSummary struct {
	InitialKurus     int64 `json:"initial_premium_kurus"`
	EndorsementDelta int64 `json:"endorsement_deltas_kurus"`
	WrittenKurus     int64 `json:"written_premium_kurus"`
	RefundKurus      int64 `json:"cancellation_refund_kurus"`
	NetKurus         int64 `json:"net_premium_kurus"`
}

type Detail struct {
	Header
	Premium       PremiumSummary    `json:"premium"`
	Versions      []VersionView     `json:"versions"`
	Endorsements  []EndorsementView `json:"endorsements"`
	LimitBalances []LimitBalance    `json:"limit_balances"`
}

// LoadHeader poliçe başlığını okur; forUpdate ise satırı kilitler (zeyil/iptal serileştirilir).
func LoadHeader(ctx context.Context, q db.DBTX, id int64, forUpdate bool) (Header, error) {
	sql := `
		SELECT p.id, p.policy_no, p.product, p.customer_id, c.full_name, p.agent_id, p.quote_id,
		       p.start_date, p.end_date, p.status, p.cancelled_at, p.cancel_effective_date, p.cancel_reason
		  FROM policies p JOIN customers c ON c.id = p.customer_id
		 WHERE p.id = $1`
	if forUpdate {
		sql += ` FOR UPDATE OF p`
	}
	var h Header
	var cancelDate *time.Time
	err := q.QueryRow(ctx, sql, id).Scan(&h.ID, &h.PolicyNo, &h.Product, &h.CustomerID, &h.CustomerName, &h.AgentID,
		&h.QuoteID, &h.start, &h.end, &h.Status, &h.CancelledAt, &cancelDate, &h.CancelReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, ErrNotFound
	}
	if err != nil {
		return h, err
	}
	h.StartDate, h.EndDate = caldate.Format(h.start), caldate.Format(h.end)
	h.PeriodDays = int64(h.end.Sub(h.start) / (24 * time.Hour))
	if cancelDate != nil {
		s := caldate.Format(*cancelDate)
		h.CancelEffectiveDate = &s
	}
	return h, nil
}

const versionCols = `
	v.id, v.version_no, v.effective_from, v.effective_to, v.tariff_version_id, tv.version_no, v.vehicle_plate,
	v.risk_data, v.annual_premium_kurus, v.annual_limit_kurus, v.recorded_at, e.endorsement_no`

const versionFrom = `
	FROM policy_versions v
	JOIN tariff_versions tv ON tv.id = v.tariff_version_id
	LEFT JOIN endorsements e ON e.to_version_id = v.id`

func scanVersion(row pgx.Row) (VersionView, error) {
	var v VersionView
	var from time.Time
	var to *time.Time
	err := row.Scan(&v.ID, &v.VersionNo, &from, &to, &v.TariffVersionID, &v.TariffVersionNo, &v.VehiclePlate,
		&v.Risk, &v.AnnualPremium, &v.AnnualLimit, &v.RecordedAt, &v.EndorsementNo)
	v.EffectiveFrom = caldate.Format(from)
	if to != nil {
		s := caldate.Format(*to)
		v.EffectiveTo = &s
	}
	return v, err
}

// loadCoverages sürümlerin teminatlarını katalog sırasıyla ekler.
func loadCoverages(ctx context.Context, q db.DBTX, product string, versions []VersionView) error {
	if len(versions) == 0 {
		return nil
	}
	idx := map[int64]int{}
	ids := make([]int64, 0, len(versions))
	for i, v := range versions {
		idx[v.ID] = i
		ids = append(ids, v.ID)
		versions[i].Coverages = []CoverageTerm{}
	}
	rows, err := q.Query(ctx, `
		SELECT policy_version_id, coverage_code, limit_kurus, deductible_type, deductible_value, annual_premium_kurus
		  FROM policy_version_coverages WHERE policy_version_id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	var vid int64
	var c CoverageTerm
	var dtype string
	if _, err := pgx.ForEachRow(rows, []any{&vid, &c.Code, &c.LimitKurus, &dtype, &c.DeductibleValue, &c.PremiumKurus}, func() error {
		term := c
		term.DeductibleType = pricing.DeductibleType(dtype)
		if spec, ok := pricing.CoverageSpecFor(pricing.Product(product), c.Code); ok {
			term.Name = spec.Name
		}
		versions[idx[vid]].Coverages = append(versions[idx[vid]].Coverages, term)
		return nil
	}); err != nil {
		return err
	}
	order := map[string]int{}
	for i, s := range pricing.Catalog[pricing.Product(product)].Coverages {
		order[s.Code] = i
	}
	for i := range versions {
		cov := versions[i].Coverages
		sort.SliceStable(cov, func(a, b int) bool { return order[cov[a].Code] < order[cov[b].Code] })
	}
	return nil
}

// LoadDetail poliçeyi tüm sürümleri, teminatları, zeyilleri ve limit bakiyeleriyle okur.
func LoadDetail(ctx context.Context, q db.DBTX, h Header) (Detail, error) {
	d := Detail{Header: h, Versions: []VersionView{}, Endorsements: []EndorsementView{}, LimitBalances: []LimitBalance{}}
	rows, err := q.Query(ctx, `SELECT `+versionCols+versionFrom+` WHERE v.policy_id = $1 ORDER BY v.version_no`, h.ID)
	if err != nil {
		return d, err
	}
	if d.Versions, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (VersionView, error) { return scanVersion(r) }); err != nil {
		return d, err
	}
	if err := loadCoverages(ctx, q, h.Product, d.Versions); err != nil {
		return d, err
	}

	rows, err = q.Query(ctx, `
		SELECT id, endorsement_no, type, effective_date, from_version_id, to_version_id, premium_delta_kurus,
		       calc_detail, created_by, created_at
		  FROM endorsements WHERE policy_id = $1 ORDER BY endorsement_no`, h.ID)
	if err != nil {
		return d, err
	}
	d.Endorsements, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (EndorsementView, error) {
		var e EndorsementView
		var eff time.Time
		err := r.Scan(&e.ID, &e.EndorsementNo, &e.Type, &eff, &e.FromVersionID, &e.ToVersionID, &e.PremiumDelta,
			&e.CalcDetail, &e.CreatedBy, &e.CreatedAt)
		e.EffectiveDate = caldate.Format(eff)
		return e, err
	})
	if err != nil {
		return d, err
	}

	rows, err = q.Query(ctx, `
		SELECT coverage_code, period_start, period_end, limit_kurus, used_kurus
		  FROM policy_limit_balances WHERE policy_id = $1 ORDER BY coverage_code NULLS LAST, period_start`, h.ID)
	if err != nil {
		return d, err
	}
	d.LimitBalances, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (LimitBalance, error) {
		var b LimitBalance
		var ps, pe time.Time
		err := r.Scan(&b.CoverageCode, &ps, &pe, &b.LimitKurus, &b.UsedKurus)
		b.PeriodStart, b.PeriodEnd = caldate.Format(ps), caldate.Format(pe)
		return b, err
	})
	if err != nil {
		return d, err
	}

	for _, v := range d.Versions {
		if v.VersionNo == 1 {
			d.Premium.InitialKurus = v.AnnualPremium // dönem tam 1 yıl: ilk yazılan prim = yıllık prim
		}
	}
	for _, e := range d.Endorsements {
		if e.Type == "CANCELLATION" {
			d.Premium.RefundKurus = -e.PremiumDelta
		} else {
			d.Premium.EndorsementDelta += e.PremiumDelta
		}
	}
	d.Premium.WrittenKurus = d.Premium.InitialKurus + d.Premium.EndorsementDelta
	d.Premium.NetKurus = d.Premium.WrittenKurus - d.Premium.RefundKurus
	return d, nil
}

// VersionOn on gününde geçerli sürümü döner. Sürüm aralığı [effective_from, effective_to) ile birlikte
// poliçe düzeyindeki sınırlar da uygulanır: [start_date, end_date) ve iptal varsa cancel_effective_date'ten önce.
// (Son sürümün effective_to'su NULL'dır; dönem sonu ve iptal poliçeden gelir.) Hasar (A4) aynı kuralı kullanır.
func VersionOn(ctx context.Context, q db.DBTX, policyID int64, product string, on time.Time) (VersionView, error) {
	v, err := scanVersion(q.QueryRow(ctx, `SELECT `+versionCols+versionFrom+`
		JOIN policies p ON p.id = v.policy_id
		WHERE v.policy_id = $1
		  AND $2::date >= p.start_date AND $2::date < p.end_date
		  AND (p.cancel_effective_date IS NULL OR $2::date < p.cancel_effective_date)
		  AND v.effective_from <= $2::date AND (v.effective_to IS NULL OR $2::date < v.effective_to)`, policyID, on))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNoCoverage
	}
	if err != nil {
		return v, err
	}
	vs := []VersionView{v}
	if err := loadCoverages(ctx, q, product, vs); err != nil {
		return v, err
	}
	return vs[0], nil
}
