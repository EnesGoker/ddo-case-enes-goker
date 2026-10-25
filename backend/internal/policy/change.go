package policy

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
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
)

// ChangeService zeyil (araç değişikliği) ve iptal işlemleridir (F-2, T-2).
//
// Her işlem önce bir "plan" üretir (doğrulama + hesap), sonra uygular. Önizleme planı kilitsiz
// okur; kayıt aynı planı poliçe satırı FOR UPDATE ile kilitliyken üretip uygular. Böylece önizleme
// ile kayıt aynı koddan geçer ve aynı poliçeye eşzamanlı işlemler serileşir.
type ChangeService struct {
	pool *pgxpool.Pool
}

func NewChangeService(pool *pgxpool.Pool) *ChangeService { return &ChangeService{pool: pool} }

type VehicleChangeRequest struct {
	EffectiveDate string `json:"effective_date"`
	VehicleGroup  string `json:"vehicle_group"` // boşsa değişmez
	Plate         string `json:"plate"`         // boşsa değişmez
	// CoverageLimits teminat kodu → yeni limit (kuruş); yalnızca değişenler. Katalog limitini aşamaz.
	CoverageLimits map[string]int64 `json:"coverage_limits"`
	// BaseVersionNo istemcinin önizlediği açık sürüm; verilirse ve değiştiyse 409 STALE_VERSION.
	BaseVersionNo *int `json:"base_version_no"`
}

type CancelRequest struct {
	EffectiveDate string `json:"effective_date"`
	Reason        string `json:"reason"`
	Note          string `json:"note"`
	BaseVersionNo *int   `json:"base_version_no"`
}

type fromTo[T any] struct {
	From T `json:"from"`
	To   T `json:"to"`
}

type vehicleChanges struct {
	VehicleGroup *fromTo[string]          `json:"vehicle_group,omitempty"`
	Plate        *fromTo[string]          `json:"plate,omitempty"`
	Limits       map[string]fromTo[int64] `json:"limits_kurus,omitempty"`
}

type writtenParts struct {
	InitialKurus      int64   `json:"initial_kurus"`
	EndorsementDeltas []int64 `json:"endorsement_deltas_kurus"`
}

// CalcDetail endorsements.calc_detail içeriğidir: saf hesap adımları + bağlam.
type CalcDetail struct {
	proration.Detail
	TariffVersionID int64           `json:"tariff_version_id"`
	FromVersionNo   int             `json:"from_version_no"`
	ToVersionNo     *int            `json:"to_version_no,omitempty"`
	Changes         *vehicleChanges `json:"changes,omitempty"`
	NewPremiumLines []pricing.Line  `json:"new_premium_lines,omitempty"`
	WrittenParts    *writtenParts   `json:"written_premium_parts,omitempty"`
	VersionClosed   *bool           `json:"version_closed,omitempty"`
	Note            string          `json:"note,omitempty"`
}

type VehicleChangePreview struct {
	PolicyID         int64          `json:"policy_id"`
	Type             string         `json:"type"`
	EffectiveDate    string         `json:"effective_date"`
	BaseVersionNo    int            `json:"base_version_no"`
	NewVersionNo     int            `json:"new_version_no"`
	OldAnnualPremium int64          `json:"old_annual_premium_kurus"`
	NewAnnualPremium int64          `json:"new_annual_premium_kurus"`
	PremiumDelta     int64          `json:"premium_delta_kurus"`
	VehiclePlate     string         `json:"vehicle_plate"`
	VehicleGroup     string         `json:"vehicle_group"`
	Coverages        []CoverageTerm `json:"coverages"`
	Lines            []pricing.Line `json:"lines"`
	CalcDetail       CalcDetail     `json:"calc_detail"`
}

type VehicleChangeResult struct {
	Endorsement EndorsementView `json:"endorsement"`
	Version     VersionView     `json:"version"`
}

type CancelPreview struct {
	PolicyID       int64      `json:"policy_id"`
	EffectiveDate  string     `json:"effective_date"`
	Reason         string     `json:"reason"`
	Method         string     `json:"method"`
	BaseVersionNo  int        `json:"base_version_no"`
	WrittenPremium int64      `json:"written_premium_kurus"`
	RefundKurus    int64      `json:"refund_kurus"`
	PremiumDelta   int64      `json:"premium_delta_kurus"`
	ClosesVersion  bool       `json:"closes_version"`
	CalcDetail     CalcDetail `json:"calc_detail"`
}

type CancelResult struct {
	Endorsement EndorsementView `json:"endorsement"`
	Policy      Header          `json:"policy"`
	RefundKurus int64           `json:"refund_kurus"`
}

// openVersion açık (effective_to NULL) sürüm.
type openVersion struct {
	ID            int64
	VersionNo     int
	EffectiveFrom time.Time
	TariffID      int64
	Plate         *string
	Risk          RiskData
	Annual        int64
	AnnualLimit   *int64
	Coverages     []CoverageTerm
}

func loadOpen(ctx context.Context, q db.DBTX, h Header) (openVersion, error) {
	var o openVersion
	var raw []byte
	err := q.QueryRow(ctx, `
		SELECT id, version_no, effective_from, tariff_version_id, vehicle_plate, risk_data,
		       annual_premium_kurus, annual_limit_kurus
		  FROM policy_versions WHERE policy_id = $1 AND effective_to IS NULL`, h.ID).
		Scan(&o.ID, &o.VersionNo, &o.EffectiveFrom, &o.TariffID, &o.Plate, &raw, &o.Annual, &o.AnnualLimit)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, fmt.Errorf("poliçe %d: yürürlükte açık sürüm yok (tutarsız veri)", h.ID)
	}
	if err != nil {
		return o, err
	}
	if err := json.Unmarshal(raw, &o.Risk); err != nil {
		return o, err
	}
	vs := []VersionView{{ID: o.ID}}
	if err := loadCoverages(ctx, q, h.Product, vs); err != nil {
		return o, err
	}
	o.Coverages = vs[0].Coverages
	return o, nil
}

// target poliçeyi yükler, kapsamı ve yürürlükte olmasını denetler; açık sürümü döner.
func (s *ChangeService) target(ctx context.Context, q db.DBTX, id int64, lock bool, baseVersion *int) (Header, openVersion, error) {
	p, _ := auth.FromContext(ctx)
	sc, err := access.Resolve(ctx, s.pool, p)
	if err != nil {
		return Header{}, openVersion{}, access.AsHTTP(err)
	}
	h, err := LoadHeader(ctx, q, id, lock)
	if errors.Is(err, ErrNotFound) || (err == nil && !sc.Allows(h.AgentID, h.CustomerID)) {
		return h, openVersion{}, httpx.NotFound("poliçe")
	}
	if err != nil {
		return h, openVersion{}, err
	}
	switch h.Status {
	case "ACTIVE":
	case "CANCELLED":
		return h, openVersion{}, httpx.NewError(http.StatusUnprocessableEntity, "POLICY_CANCELLED",
			"iptal edilmiş poliçede zeyil/iptal yapılamaz")
	default:
		return h, openVersion{}, httpx.NewError(http.StatusUnprocessableEntity, "POLICY_NOT_ACTIVE",
			"poliçe yürürlükte değil: "+h.Status)
	}
	o, err := loadOpen(ctx, q, h)
	if err != nil {
		return h, o, err
	}
	if baseVersion != nil && *baseVersion != o.VersionNo {
		return h, o, httpx.NewError(http.StatusConflict, "STALE_VERSION",
			fmt.Sprintf("poliçe bu arada değişti (güncel sürüm %d, istekteki %d); önizlemeyi yenileyin", o.VersionNo, *baseVersion))
	}
	return h, o, nil
}

func parseDate(field, v string) (time.Time, error) {
	d, err := caldate.Parse(v)
	if err != nil {
		return d, httpx.Invalid(map[string]string{field: err.Error()})
	}
	return d, nil
}

func invalidDate(msg string) error {
	return httpx.NewError(http.StatusUnprocessableEntity, "INVALID_EFFECTIVE_DATE", msg)
}

type vehiclePlan struct {
	h        Header
	open     openVersion
	date     time.Time
	risk     RiskData
	plate    string
	group    string
	result   pricing.Result
	cov      []CoverageTerm
	limits   map[string]int64 // yalnızca değişen teminat limitleri
	calc     CalcDetail
	newVerNo int
}

func (s *ChangeService) planVehicle(ctx context.Context, q db.DBTX, id int64, req VehicleChangeRequest, lock bool) (vehiclePlan, error) {
	var pl vehiclePlan
	date, err := parseDate("effective_date", req.EffectiveDate)
	if err != nil {
		return pl, err
	}
	pl.h, pl.open, err = s.target(ctx, q, id, lock, req.BaseVersionNo)
	if err != nil {
		return pl, err
	}
	product := pricing.Product(pl.h.Product)
	if !product.IsVehicle() {
		return pl, httpx.NewError(http.StatusUnprocessableEntity, "NOT_A_VEHICLE_POLICY",
			"araç değişikliği yalnızca KASKO/TRAFİK poliçelerinde yapılabilir")
	}
	// Geriye dönük zincir yok: tarih açık sürümün başlangıcından sonra, poliçe bitişinden önce olmalı.
	if !date.After(pl.open.EffectiveFrom) || !date.Before(pl.h.end) {
		return pl, invalidDate(fmt.Sprintf("zeyil tarihi açık sürümün başlangıcından (%s) sonra ve poliçe bitişinden (%s) önce olmalı; son zeyilden önceki bir tarihe zeyil yapılamaz",
			caldate.Format(pl.open.EffectiveFrom), pl.h.EndDate))
	}
	if err := claimsAfter(ctx, q, pl.h.ID, date); err != nil {
		return pl, err
	}
	pl.date = date

	pl.risk = pl.open.Risk
	pl.risk.ReviewReasons = nil // teklif anına ait; yeni sürüme taşınmaz
	pl.plate, pl.group = pl.risk.Plate, pl.risk.VehicleGroup
	details := map[string]string{}
	if req.Plate != "" {
		p, ok := NormalizePlate(req.Plate)
		if !ok {
			details["plate"] = "geçerli bir plaka olmalı (ör. 34ABC123)"
		}
		pl.plate = p
	}
	if g := strings.ToUpper(strings.TrimSpace(req.VehicleGroup)); g != "" {
		pl.group = g
	}
	pl.limits = map[string]int64{}
	current := map[string]CoverageTerm{}
	for _, c := range pl.open.Coverages {
		current[c.Code] = c
	}
	for code, limit := range req.CoverageLimits {
		c, ok := current[code]
		spec, _ := pricing.CoverageSpecFor(product, code)
		switch {
		case !ok:
			details["coverage_limits."+code] = "poliçede bu teminat yok"
		case limit < 1 || limit > spec.LimitKurus:
			details["coverage_limits."+code] = fmt.Sprintf("1 ile %d kuruş arasında olmalı (katalog limiti)", spec.LimitKurus)
		case limit != c.LimitKurus:
			pl.limits[code] = limit
		}
	}
	if len(details) > 0 {
		return pl, httpx.Invalid(details)
	}
	if pl.plate == pl.risk.Plate && pl.group == pl.risk.VehicleGroup && len(pl.limits) == 0 {
		return pl, httpx.NewError(http.StatusUnprocessableEntity, "NO_CHANGE",
			"plaka, araç grubu ve teminat limitleri aynı; zeyil gerekmiyor")
	}

	// Yeni yıllık prim: poliçenin orijinal tarife sürümüyle (zeyil tarihindeki değil) hesaplanır.
	t, err := tariff.LoadByID(ctx, q, pl.open.TariffID)
	if err != nil {
		return pl, err
	}
	codes := make([]string, 0, len(pl.open.Coverages))
	for _, c := range pl.open.Coverages {
		codes = append(codes, c.Code)
	}
	newRisk := pl.risk.Risk
	newRisk.VehicleGroup = pl.group
	pl.result, err = pricing.Calculate(t, pricing.Input{Product: product, Risk: newRisk, Coverages: codes})
	var ie *pricing.InputError
	if errors.As(err, &ie) {
		return pl, httpx.Invalid(map[string]string{ie.Field: ie.Message})
	}
	if err != nil {
		return pl, err
	}
	pl.risk.Risk = pl.result.Risk
	pl.risk.Plate = pl.plate

	if len(pl.limits) > 0 {
		used, err := usedLimits(ctx, q, pl.h)
		if err != nil {
			return pl, err
		}
		below := map[string]string{}
		for code, limit := range pl.limits {
			if limit < used[code] {
				below[code] = fmt.Sprintf("yeni limit %d kuruş, kullanılan %d kuruş", limit, used[code])
			}
		}
		if len(below) > 0 {
			return pl, &httpx.AppError{Status: http.StatusUnprocessableEntity, Code: "LIMIT_BELOW_USAGE",
				Message: "teminat limiti kullanılmış tutarın altına düşürülemez", Details: below}
		}
	}

	premium := map[string]int64{}
	for _, c := range pl.result.Coverages {
		premium[c.Code] = c.PremiumKurus
	}
	changes := &vehicleChanges{}
	for _, c := range pl.open.Coverages {
		n := c
		n.PremiumKurus = premium[c.Code]
		if l, ok := pl.limits[c.Code]; ok {
			n.LimitKurus = l
			if changes.Limits == nil {
				changes.Limits = map[string]fromTo[int64]{}
			}
			changes.Limits[c.Code] = fromTo[int64]{c.LimitKurus, l}
		}
		pl.cov = append(pl.cov, n)
	}
	if pl.group != pl.open.Risk.VehicleGroup {
		changes.VehicleGroup = &fromTo[string]{pl.open.Risk.VehicleGroup, pl.group}
	}
	if pl.plate != pl.open.Risk.Plate {
		changes.Plate = &fromTo[string]{pl.open.Risk.Plate, pl.plate}
	}

	det, err := proration.VehicleChange{
		Period:        proration.Period{Start: pl.h.start, End: pl.h.end},
		EffectiveDate: date, OldAnnualKurus: pl.open.Annual, NewAnnualKurus: pl.result.TotalKurus,
	}.Calculate()
	if err != nil {
		return pl, invalidDate(err.Error())
	}
	pl.newVerNo = pl.open.VersionNo + 1
	pl.calc = CalcDetail{Detail: det, TariffVersionID: pl.open.TariffID, FromVersionNo: pl.open.VersionNo,
		ToVersionNo: &pl.newVerNo, Changes: changes, NewPremiumLines: pl.result.Lines}
	return pl, nil
}

func usedLimits(ctx context.Context, q db.DBTX, h Header) (map[string]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT coverage_code, used_kurus FROM policy_limit_balances
		 WHERE policy_id = $1 AND period_start = $2 AND coverage_code IS NOT NULL`, h.ID, h.start)
	if err != nil {
		return nil, err
	}
	used := map[string]int64{}
	var code string
	var v int64
	_, err = pgx.ForEachRow(rows, []any{&code, &v}, func() error { used[code] = v; return nil })
	return used, err
}

func (pl vehiclePlan) preview() VehicleChangePreview {
	return VehicleChangePreview{
		PolicyID: pl.h.ID, Type: "VEHICLE_CHANGE", EffectiveDate: caldate.Format(pl.date),
		BaseVersionNo: pl.open.VersionNo, NewVersionNo: pl.newVerNo,
		OldAnnualPremium: pl.open.Annual, NewAnnualPremium: pl.result.TotalKurus, PremiumDelta: pl.calc.PremiumDeltaKurus,
		VehiclePlate: pl.plate, VehicleGroup: pl.group, Coverages: pl.cov, Lines: pl.result.Lines, CalcDetail: pl.calc,
	}
}

func (s *ChangeService) PreviewVehicleChange(ctx context.Context, id int64, req VehicleChangeRequest) (VehicleChangePreview, error) {
	pl, err := s.planVehicle(ctx, s.pool, id, req, false)
	if err != nil {
		return VehicleChangePreview{}, err
	}
	return pl.preview(), nil
}

// VehicleChange açık sürümü zeyil tarihinde kapatır, yeni sürümü açar ve zeyili kaydeder (tek transaction).
func (s *ChangeService) VehicleChange(ctx context.Context, id int64, req VehicleChangeRequest) (VehicleChangeResult, error) {
	var out VehicleChangeResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		pl, err := s.planVehicle(ctx, tx, id, req, true)
		if err != nil {
			return err
		}
		if err := closeVersion(ctx, tx, pl.open.ID, pl.date); err != nil {
			return err
		}
		riskJSON, err := json.Marshal(pl.risk)
		if err != nil {
			return err
		}
		var plate *string
		if pl.plate != "" {
			plate = &pl.plate
		}
		var newID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO policy_versions (policy_id, version_no, effective_from, effective_to, tariff_version_id,
			                             vehicle_plate, risk_data, annual_premium_kurus, annual_limit_kurus)
			VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, $8) RETURNING id`,
			pl.h.ID, pl.newVerNo, pl.date, pl.open.TariffID, plate, riskJSON, pl.result.TotalKurus, pl.open.AnnualLimit).
			Scan(&newID); err != nil {
			return err
		}
		for _, c := range pl.cov {
			if _, err := tx.Exec(ctx, `
				INSERT INTO policy_version_coverages (policy_version_id, coverage_code, limit_kurus,
				                                      deductible_type, deductible_value, annual_premium_kurus)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				newID, c.Code, c.LimitKurus, string(c.DeductibleType), c.DeductibleValue, c.PremiumKurus); err != nil {
				return err
			}
		}
		e, err := insertEndorsement(ctx, tx, pl.h.ID, "VEHICLE_CHANGE", pl.date, pl.open.ID, &newID,
			pl.calc.PremiumDeltaKurus, pl.calc)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET endorsement_id = $2 WHERE id = $1`, newID, e.ID); err != nil {
			return err
		}
		// Limit değişikliği: koşullu UPDATE (eşzamanlı hasar kullanımı limitin altına düşmeyi yakalar)
		for code, limit := range pl.limits {
			tag, err := tx.Exec(ctx, `
				UPDATE policy_limit_balances SET limit_kurus = $3
				 WHERE policy_id = $1 AND coverage_code = $2 AND period_start = $4 AND used_kurus <= $3`,
				pl.h.ID, code, limit, pl.h.start)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return &httpx.AppError{Status: http.StatusUnprocessableEntity, Code: "LIMIT_BELOW_USAGE",
					Message: "teminat limiti kullanılmış tutarın altına düşürülemez", Details: map[string]string{code: "kullanım değişti"}}
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "POLICY_ENDORSED", EntityType: "policy", EntityID: pl.h.ID,
			Old: map[string]any{"version_no": pl.open.VersionNo, "annual_premium_kurus": pl.open.Annual,
				"vehicle_plate": pl.open.Risk.Plate, "vehicle_group": pl.open.Risk.VehicleGroup},
			New: map[string]any{"version_no": pl.newVerNo, "annual_premium_kurus": pl.result.TotalKurus,
				"vehicle_plate": pl.plate, "vehicle_group": pl.group, "effective_date": caldate.Format(pl.date),
				"endorsement_no": e.EndorsementNo, "premium_delta_kurus": e.PremiumDelta, "limit_changes": pl.limits},
		}); err != nil {
			return err
		}
		out.Endorsement = e
		v, err := scanVersion(tx.QueryRow(ctx, `SELECT `+versionCols+versionFrom+` WHERE v.id = $1`, newID))
		if err != nil {
			return err
		}
		vs := []VersionView{v}
		if err := loadCoverages(ctx, tx, pl.h.Product, vs); err != nil {
			return err
		}
		out.Version = vs[0]
		return nil
	})
	return out, concurrentChange(err)
}

type cancelPlan struct {
	h       Header
	open    openVersion
	date    time.Time
	reason  string
	written int64
	calc    CalcDetail
	closes  bool
}

func (s *ChangeService) planCancel(ctx context.Context, q db.DBTX, id int64, req CancelRequest, lock bool) (cancelPlan, error) {
	var pl cancelPlan
	date, err := parseDate("effective_date", req.EffectiveDate)
	if err != nil {
		return pl, err
	}
	pl.reason = strings.ToUpper(strings.TrimSpace(req.Reason))
	if _, err := proration.MethodFor(pl.reason); err != nil {
		return pl, httpx.Invalid(map[string]string{"reason": "CUSTOMER_REQUEST, NON_PAYMENT, UNDERWRITER veya OTHER olmalı"})
	}
	if len(req.Note) > 1000 {
		return pl, httpx.Invalid(map[string]string{"note": "en fazla 1000 karakter"})
	}
	// Acente yalnızca müşteri talebiyle iptal edebilir; şirket kaynaklı gerekçeler underwriter'a aittir.
	if p, _ := auth.FromContext(ctx); p.Role == auth.RoleAgent && pl.reason != proration.ReasonCustomerRequest {
		return pl, httpx.NewError(http.StatusForbidden, httpx.CodeForbidden,
			"acente yalnızca CUSTOMER_REQUEST gerekçesiyle iptal edebilir")
	}
	pl.h, pl.open, err = s.target(ctx, q, id, lock, req.BaseVersionNo)
	if err != nil {
		return pl, err
	}
	if date.Before(pl.open.EffectiveFrom) || !date.Before(pl.h.end) {
		return pl, invalidDate(fmt.Sprintf("iptal tarihi açık sürümün başlangıcı (%s, dahil) ile poliçe bitişi (%s, hariç) arasında olmalı",
			caldate.Format(pl.open.EffectiveFrom), pl.h.EndDate))
	}
	if err := claimsAfter(ctx, q, pl.h.ID, date); err != nil {
		return pl, err
	}
	pl.date = date

	parts := writtenParts{EndorsementDeltas: []int64{}}
	if err := q.QueryRow(ctx, `SELECT annual_premium_kurus FROM policy_versions WHERE policy_id = $1 AND version_no = 1`,
		pl.h.ID).Scan(&parts.InitialKurus); err != nil {
		return pl, err
	}
	rows, err := q.Query(ctx, `
		SELECT premium_delta_kurus FROM endorsements
		 WHERE policy_id = $1 AND type <> 'CANCELLATION' ORDER BY endorsement_no`, pl.h.ID)
	if err != nil {
		return pl, err
	}
	if parts.EndorsementDeltas, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
		return pl, err
	}
	pl.written = parts.InitialKurus
	for _, d := range parts.EndorsementDeltas {
		pl.written += d
	}
	table, err := tariff.LoadShortPeriod(ctx, q, pl.open.TariffID)
	if err != nil {
		return pl, err
	}
	det, err := proration.Cancellation{
		Period:        proration.Period{Start: pl.h.start, End: pl.h.end},
		EffectiveDate: date, Reason: pl.reason, AnnualKurus: pl.open.Annual, WrittenKurus: pl.written,
		ShortPeriod: table,
	}.Calculate()
	if errors.Is(err, proration.ErrNoShortPeriodRate) {
		return pl, httpx.NewError(http.StatusUnprocessableEntity, "SHORT_PERIOD_RATE_MISSING", err.Error())
	}
	if err != nil {
		return pl, invalidDate(err.Error())
	}
	pl.closes = date.After(pl.open.EffectiveFrom)
	pl.calc = CalcDetail{Detail: det, TariffVersionID: pl.open.TariffID, FromVersionNo: pl.open.VersionNo,
		WrittenParts: &parts, VersionClosed: &pl.closes, Note: strings.TrimSpace(req.Note)}
	if !pl.closes {
		pl.calc.Steps = append(pl.calc.Steps, "İptal tarihi açık sürümün başlangıç günü: sıfır uzunlukta sürüm aralığı "+
			"CHECK ile yasak olduğundan sürüm kapatılmaz; teminat poliçe düzeyindeki iptal tarihiyle sona erer")
	}
	return pl, nil
}

func (pl cancelPlan) preview() CancelPreview {
	return CancelPreview{
		PolicyID: pl.h.ID, EffectiveDate: caldate.Format(pl.date), Reason: pl.reason, Method: string(pl.calc.Method),
		BaseVersionNo: pl.open.VersionNo, WrittenPremium: pl.written, RefundKurus: -pl.calc.PremiumDeltaKurus,
		PremiumDelta: pl.calc.PremiumDeltaKurus, ClosesVersion: pl.closes, CalcDetail: pl.calc,
	}
}

func (s *ChangeService) PreviewCancel(ctx context.Context, id int64, req CancelRequest) (CancelPreview, error) {
	pl, err := s.planCancel(ctx, s.pool, id, req, false)
	if err != nil {
		return CancelPreview{}, err
	}
	return pl.preview(), nil
}

// Cancel poliçeyi iptal eder: açık sürüm iptal tarihinde kapanır (yeni sürüm açılmaz), poliçe CANCELLED olur
// ve iade tutarı iptal zeyline yazılır. Tahsilat/muhasebe mahsubu A5'te.
func (s *ChangeService) Cancel(ctx context.Context, id int64, req CancelRequest) (CancelResult, error) {
	var out CancelResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		pl, err := s.planCancel(ctx, tx, id, req, true)
		if err != nil {
			return err
		}
		if pl.closes {
			if err := closeVersion(ctx, tx, pl.open.ID, pl.date); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `
			UPDATE policies SET status = 'CANCELLED', cancelled_at = now(), cancel_effective_date = $2, cancel_reason = $3
			 WHERE id = $1 AND status = 'ACTIVE'`, pl.h.ID, pl.date, pl.reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return httpx.NewError(http.StatusConflict, "CONCURRENT_MODIFICATION", "poliçe eşzamanlı olarak değişti")
		}
		e, err := insertEndorsement(ctx, tx, pl.h.ID, "CANCELLATION", pl.date, pl.open.ID, nil, pl.calc.PremiumDeltaKurus, pl.calc)
		if err != nil {
			return err
		}
		refund := -pl.calc.PremiumDeltaKurus
		if err := audit.Write(ctx, tx, audit.Entry{Action: "POLICY_CANCELLED", EntityType: "policy", EntityID: pl.h.ID,
			Old: map[string]any{"status": pl.h.Status, "open_version_no": pl.open.VersionNo},
			New: map[string]any{"status": "CANCELLED", "cancel_effective_date": caldate.Format(pl.date),
				"cancel_reason": pl.reason, "method": pl.calc.Method, "refund_kurus": refund,
				"endorsement_no": e.EndorsementNo, "version_closed": pl.closes},
		}); err != nil {
			return err
		}
		h, err := LoadHeader(ctx, tx, pl.h.ID, false)
		if err != nil {
			return err
		}
		out = CancelResult{Endorsement: e, Policy: h, RefundKurus: refund}
		return nil
	})
	return out, concurrentChange(err)
}

// closeVersion açık sürümü kapatır (00003 tetikleyicisinin izin verdiği tek değişiklik).
func closeVersion(ctx context.Context, tx pgx.Tx, versionID int64, at time.Time) error {
	tag, err := tx.Exec(ctx, `UPDATE policy_versions SET effective_to = $2 WHERE id = $1 AND effective_to IS NULL`,
		versionID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return httpx.NewError(http.StatusConflict, "CONCURRENT_MODIFICATION", "açık sürüm eşzamanlı olarak kapatıldı")
	}
	return nil
}

// insertEndorsement sıradaki zeyil numarasıyla kaydı ekler (poliçe kilidi altında; UNIQUE ikinci savunma).
func insertEndorsement(ctx context.Context, tx pgx.Tx, policyID int64, typ string, at time.Time, fromVersion int64,
	toVersion *int64, delta int64, calc CalcDetail) (EndorsementView, error) {
	calcJSON, err := json.Marshal(calc)
	if err != nil {
		return EndorsementView{}, err
	}
	p, _ := auth.FromContext(ctx)
	e := EndorsementView{Type: typ, EffectiveDate: caldate.Format(at), FromVersionID: fromVersion, ToVersionID: toVersion,
		PremiumDelta: delta, CalcDetail: calcJSON, CreatedBy: p.UserID}
	err = tx.QueryRow(ctx, `
		INSERT INTO endorsements (policy_id, endorsement_no, type, effective_date, from_version_id, to_version_id,
		                          premium_delta_kurus, calc_detail, created_by)
		VALUES ($1, (SELECT COALESCE(max(endorsement_no), 0) + 1 FROM endorsements WHERE policy_id = $1),
		        $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, endorsement_no, created_at`,
		policyID, typ, at, fromVersion, toVersion, delta, calcJSON, p.UserID).Scan(&e.ID, &e.EndorsementNo, &e.CreatedAt)
	return e, err
}

// concurrentChange kilit atlanırsa devreye girecek kısıt ihlallerini 409'a çevirir (ikinci savunma hattı).
func concurrentChange(err error) error {
	if db.Violates(err, "policy_versions_policy_id_version_no_key", "policy_versions_policy_id_daterange_excl",
		"endorsements_policy_id_endorsement_no_key") {
		return httpx.NewError(http.StatusConflict, "CONCURRENT_MODIFICATION",
			"poliçe eşzamanlı olarak değişti; güncel hali okuyup tekrar deneyin")
	}
	return err
}

// Hasar açma da aynı poliçe satırını kilitler: kontrol ile sürüm değişikliği arasına hasar giremez.
func claimsAfter(ctx context.Context, q db.DBTX, policyID int64, at time.Time) error {
	var found bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM claims WHERE policy_id = $1 AND status <> 'REJECTED' AND loss_date >= $2)`, policyID, at).Scan(&found); err != nil {
		return err
	}
	if found {
		return httpx.NewError(http.StatusUnprocessableEntity, "CLAIMS_AFTER_EFFECTIVE_DATE", "bu tarihte veya sonrasında reddedilmemiş hasar var")
	}
	return nil
}
