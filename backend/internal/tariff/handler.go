package tariff

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

type coverageView struct {
	pricing.CoverageSpec
	BasePremiumKurus int64 `json:"base_premium_kurus"`
}

type activeResponse struct {
	ID               int64                `json:"id"`
	Product          pricing.Product      `json:"product"`
	VersionNo        int                  `json:"version_no"`
	ValidFrom        string               `json:"valid_from"`
	ValidTo          *string              `json:"valid_to"`
	FactorOrder      []pricing.FactorType `json:"factor_order"`
	Factors          []pricing.Factor     `json:"factors"`
	Coverages        []coverageView       `json:"coverages"`
	AnnualLimitKurus int64                `json:"annual_limit_kurus"`
	ShortPeriodRates []ShortPeriodRate    `json:"short_period_rates"`
}

// ActiveHandler GET /api/v1/tariffs/active?product=&date= (date yoksa bugün, Türkiye saati).
func ActiveHandler(pool *pgxpool.Pool, now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		product := pricing.Product(strings.ToUpper(r.URL.Query().Get("product")))
		details := map[string]string{}
		if !product.Valid() {
			details["product"] = "KASKO, TRAFIK veya KONUT olmalı"
		}
		on := caldate.Today(now())
		if v := r.URL.Query().Get("date"); v != "" {
			d, err := caldate.Parse(v)
			if err != nil {
				details["date"] = err.Error()
			}
			on = d
		}
		if len(details) > 0 {
			httpx.ValidationError(w, details)
			return
		}
		t, err := Select(r.Context(), pool, product, on)
		if errors.Is(err, pricing.ErrNoTariff) {
			httpx.Error(w, http.StatusNotFound, "NO_TARIFF", err.Error())
			return
		}
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		sp, err := LoadShortPeriod(r.Context(), pool, t.ID)
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		resp := activeResponse{
			ID: t.ID, Product: t.Product, VersionNo: t.VersionNo,
			ValidFrom:        caldate.Format(t.ValidFrom),
			FactorOrder:      pricing.FactorOrder,
			Factors:          t.Factors,
			AnnualLimitKurus: pricing.Catalog[product].AnnualLimitKurus,
			ShortPeriodRates: sp,
		}
		if t.ValidTo != nil {
			s := caldate.Format(*t.ValidTo)
			resp.ValidTo = &s
		}
		for _, c := range pricing.Catalog[product].Coverages {
			resp.Coverages = append(resp.Coverages, coverageView{CoverageSpec: c, BasePremiumKurus: t.BaseRates[c.Code]})
		}
		httpx.JSON(w, http.StatusOK, resp)
	}
}
