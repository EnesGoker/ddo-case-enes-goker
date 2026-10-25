// Package policy poliçe düzenleme (tekliften) ve listeleme işlemleridir.
package policy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment/store"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/clock"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

// CoverageTerm poliçe sürümüne yazılan teminat (tekliften kopyalanır).
type CoverageTerm struct {
	Code            string                 `json:"code"`
	Name            string                 `json:"name,omitempty"`
	LimitKurus      int64                  `json:"limit_kurus"`
	DeductibleType  pricing.DeductibleType `json:"deductible_type"`
	DeductibleValue int64                  `json:"deductible_value"`
	PremiumKurus    int64                  `json:"premium_kurus"`
}

type IssueInput struct {
	InstallmentCount int
	ActorID          int64
	PolicyNoOverride string
	QuoteID          int64
	Product          pricing.Product
	CustomerID       int64
	AgentID          *int64
	TariffVersionID  int64
	StartDate        time.Time
	EndDate          time.Time
	Plate            *string
	RiskData         []byte // JSON
	AnnualPremium    int64
	AnnualLimitKurus int64
	Coverages        []CoverageTerm
}

type LimitBalance struct {
	CoverageCode *string `json:"coverage_code"` // null = yıllık toplam limit
	PeriodStart  string  `json:"period_start"`
	PeriodEnd    string  `json:"period_end"`
	LimitKurus   int64   `json:"limit_kurus"`
	UsedKurus    int64   `json:"used_kurus"`
}

type Issued struct {
	ID               int64          `json:"id"`
	PolicyNo         string         `json:"policy_no"`
	Product          string         `json:"product"`
	CustomerID       int64          `json:"customer_id"`
	AgentID          *int64         `json:"agent_id"`
	QuoteID          int64          `json:"quote_id"`
	StartDate        string         `json:"start_date"`
	EndDate          string         `json:"end_date"`
	Status           string         `json:"status"`
	VersionID        int64          `json:"version_id"`
	VersionNo        int            `json:"version_no"`
	AnnualPremium    int64          `json:"annual_premium_kurus"`
	AnnualLimitKurus int64          `json:"annual_limit_kurus"`
	VehiclePlate     *string        `json:"vehicle_plate"`
	Coverages        []CoverageTerm `json:"coverages"`
	LimitBalances    []LimitBalance `json:"limit_balances"`
}

var productCode = map[pricing.Product]string{pricing.Kasko: "KS", pricing.Trafik: "TR", pricing.Konut: "KN"}

// PolicyNo teklif ile poliçe 1:1 olduğundan (policies.quote_id UNIQUE) teklif no'sundan türetilir.
func PolicyNo(p pricing.Product, start time.Time, quoteID int64) string {
	return fmt.Sprintf("GP-%s-%d-%08d", productCode[p], start.Year(), quoteID)
}

// Issue poliçeyi, ilk sürümü, teminatları ve limit bakiyelerini çağıranın transaction'ında yazar.
func Issue(ctx context.Context, tx pgx.Tx, in IssueInput) (Issued, error) {
	out := Issued{
		PolicyNo: PolicyNo(in.Product, in.StartDate, in.QuoteID), Product: string(in.Product),
		CustomerID: in.CustomerID, AgentID: in.AgentID, QuoteID: in.QuoteID,
		StartDate: caldate.Format(in.StartDate), EndDate: caldate.Format(in.EndDate), Status: "ACTIVE",
		VersionNo: 1, AnnualPremium: in.AnnualPremium, AnnualLimitKurus: in.AnnualLimitKurus,
		VehiclePlate: in.Plate, Coverages: in.Coverages,
	}
	if in.PolicyNoOverride != "" {
		out.PolicyNo = in.PolicyNoOverride
	}
	var quoteID *int64
	if in.QuoteID > 0 {
		quoteID = &in.QuoteID
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO policies (policy_no, product, customer_id, agent_id, quote_id, start_date, end_date, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		out.PolicyNo, string(in.Product), in.CustomerID, in.AgentID, quoteID, in.StartDate, in.EndDate, clock.Now(ctx)).Scan(&out.ID)
	if err != nil {
		return out, fmt.Errorf("poliçe: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO policy_versions (policy_id, version_no, effective_from, effective_to, tariff_version_id,
		                             vehicle_plate, risk_data, annual_premium_kurus, annual_limit_kurus, recorded_at)
		VALUES ($1, 1, $2, NULL, $3, $4, $5, $6, $7, $8) RETURNING id`,
		out.ID, in.StartDate, in.TariffVersionID, in.Plate, in.RiskData, in.AnnualPremium, in.AnnualLimitKurus, clock.Now(ctx)).
		Scan(&out.VersionID)
	if err != nil {
		return out, fmt.Errorf("poliçe sürümü: %w", err)
	}
	add := func(code *string, limit int64) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO policy_limit_balances (policy_id, coverage_code, period_start, period_end, limit_kurus)
			VALUES ($1, $2, $3, $4, $5)`, out.ID, code, in.StartDate, in.EndDate, limit)
		if err == nil {
			out.LimitBalances = append(out.LimitBalances, LimitBalance{
				CoverageCode: code, PeriodStart: out.StartDate, PeriodEnd: out.EndDate, LimitKurus: limit})
		}
		return err
	}
	for _, c := range in.Coverages {
		if _, err := tx.Exec(ctx, `
			INSERT INTO policy_version_coverages (policy_version_id, coverage_code, limit_kurus,
			                                      deductible_type, deductible_value, annual_premium_kurus)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			out.VersionID, c.Code, c.LimitKurus, string(c.DeductibleType), c.DeductibleValue, c.PremiumKurus); err != nil {
			return out, fmt.Errorf("teminat %s: %w", c.Code, err)
		}
		code := c.Code
		if err := add(&code, c.LimitKurus); err != nil {
			return out, fmt.Errorf("limit bakiyesi %s: %w", c.Code, err)
		}
	}
	if err := add(nil, in.AnnualLimitKurus); err != nil {
		return out, fmt.Errorf("yıllık limit bakiyesi: %w", err)
	}
	// Yeni poliçe de ilk faturalama kaydından önce aynı satır kilidini alır.
	if _, err := LoadHeader(ctx, tx, out.ID, true); err != nil {
		return out, err
	}
	count := in.InstallmentCount
	if count == 0 {
		count = 1
	}
	actor := in.ActorID
	if actor == 0 {
		if p, ok := auth.FromContext(ctx); ok {
			actor = p.UserID
		}
	}
	if actor == 0 && in.QuoteID > 0 {
		if err := tx.QueryRow(ctx, `
   SELECT created_by FROM quotes WHERE id = $1`, in.QuoteID).Scan(&actor); err != nil {
			return out, err
		}
	}
	if err := store.Create(ctx, tx, out.ID, in.AnnualPremium, count, in.StartDate, actor); err != nil {
		return out, err
	}
	return out, nil
}

type ListItem struct {
	ID            int64   `json:"id"`
	PolicyNo      string  `json:"policy_no"`
	Product       string  `json:"product"`
	CustomerID    int64   `json:"customer_id"`
	CustomerName  string  `json:"customer_name"`
	AgentID       *int64  `json:"agent_id"`
	StartDate     string  `json:"start_date"`
	EndDate       string  `json:"end_date"`
	Status        string  `json:"status"`
	AnnualPremium int64   `json:"annual_premium_kurus"`
	VehiclePlate  *string `json:"vehicle_plate"`
}

// ListHandler GET /api/v1/policies — müşteri/acente kendi kayıtlarını, diğer yetkili roller tümünü görür.
func ListHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, bad := page.Parse(r)
		if bad != nil {
			httpx.ValidationError(w, bad)
			return
		}
		p, _ := auth.FromContext(r.Context())
		scope, err := access.Resolve(r.Context(), pool, p)
		if err != nil {
			httpx.Fail(w, r, access.AsHTTP(err))
			return
		}
		rows, err := pool.Query(r.Context(), `
			SELECT p.id, p.policy_no, p.product, p.customer_id, c.full_name, p.agent_id,
			       to_char(p.start_date, 'YYYY-MM-DD'), to_char(p.end_date, 'YYYY-MM-DD'), p.status,
			       v.annual_premium_kurus, v.vehicle_plate
			  FROM policies p
			  JOIN customers c ON c.id = p.customer_id
			  JOIN LATERAL (SELECT annual_premium_kurus, vehicle_plate FROM policy_versions
			                 WHERE policy_id = p.id ORDER BY version_no DESC LIMIT 1) v ON true
			 WHERE ($1::bigint = 0 OR p.id < $1)
			   AND ($2::bigint IS NULL OR p.agent_id = $2)
			   AND ($3::bigint IS NULL OR p.customer_id = $3)
			 ORDER BY p.id DESC
			 LIMIT $4`, params.AfterID, scope.AgentID, scope.CustomerID, params.Limit)
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ListItem, error) {
			var it ListItem
			err := row.Scan(&it.ID, &it.PolicyNo, &it.Product, &it.CustomerID, &it.CustomerName, &it.AgentID,
				&it.StartDate, &it.EndDate, &it.Status, &it.AnnualPremium, &it.VehiclePlate)
			return it, err
		})
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, page.Build(items, params.Limit, func(i ListItem) int64 { return i.ID }))
	}
}
