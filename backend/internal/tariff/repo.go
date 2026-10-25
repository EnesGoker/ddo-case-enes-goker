// Package tariff sürümlü tarifeleri veritabanından pricing.Tariff yapısına yükler.
package tariff

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
)

// LoadActive ürünün tüm ACTIVE sürümlerini (oranları ve faktörleriyle) yükler.
// Tarihe göre seçim pricing.SelectTariff ile yapılır (tek kural, birim testli).
func LoadActive(ctx context.Context, q db.DBTX, product pricing.Product) ([]pricing.Tariff, error) {
	rows, err := q.Query(ctx, `
		SELECT id, product, version_no, valid_from, valid_to
		  FROM tariff_versions WHERE status = 'ACTIVE' AND product = $1 ORDER BY version_no`, string(product))
	if err != nil {
		return nil, err
	}
	return collect(ctx, q, rows)
}

var ErrNotFound = errors.New("tarife sürümü bulunamadı")

// LoadByID sürümü durumundan bağımsız yükler: poliçe, düzenlendiği tarife sürümüyle yeniden
// hesaplanır (sürüm sonradan RETIRED olsa da).
func LoadByID(ctx context.Context, q db.DBTX, id int64) (pricing.Tariff, error) {
	rows, err := q.Query(ctx, `
		SELECT id, product, version_no, valid_from, valid_to FROM tariff_versions WHERE id = $1`, id)
	if err != nil {
		return pricing.Tariff{}, err
	}
	ts, err := collect(ctx, q, rows)
	if err != nil {
		return pricing.Tariff{}, err
	}
	if len(ts) == 0 {
		return pricing.Tariff{}, ErrNotFound
	}
	return ts[0], nil
}

// collect sürüm satırlarını okur ve taban oranlarıyla faktörlerini ekler.
func collect(ctx context.Context, q db.DBTX, rows pgx.Rows) ([]pricing.Tariff, error) {
	tariffs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pricing.Tariff, error) {
		var t pricing.Tariff
		var p string
		err := r.Scan(&t.ID, &p, &t.VersionNo, &t.ValidFrom, &t.ValidTo)
		t.Product = pricing.Product(p)
		t.BaseRates = map[string]int64{}
		return t, err
	})
	if err != nil || len(tariffs) == 0 {
		return tariffs, err
	}
	byID := map[int64]*pricing.Tariff{}
	ids := make([]int64, 0, len(tariffs))
	for i := range tariffs {
		byID[tariffs[i].ID] = &tariffs[i]
		ids = append(ids, tariffs[i].ID)
	}

	rows, err = q.Query(ctx, `
		SELECT tariff_version_id, coverage_code, base_premium_kurus
		  FROM tariff_base_rates WHERE tariff_version_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	var id, amount int64
	var code string
	if _, err := pgx.ForEachRow(rows, []any{&id, &code, &amount}, func() error {
		byID[id].BaseRates[code] = amount
		return nil
	}); err != nil {
		return nil, err
	}

	rows, err = q.Query(ctx, `
		SELECT tariff_version_id, factor_type, factor_key, lower(num_range), upper(num_range), multiplier_bp
		  FROM tariff_factors WHERE tariff_version_id = ANY($1) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	var ftype string
	var key *string
	var lo, hi *int
	var bp int64
	if _, err := pgx.ForEachRow(rows, []any{&id, &ftype, &key, &lo, &hi, &bp}, func() error {
		f := pricing.Factor{Type: pricing.FactorType(ftype), MultiplierBP: bp}
		if key != nil {
			f.Key = *key
		}
		if lo != nil {
			r := pricing.Range{Lo: *lo}
			if hi != nil {
				h := *hi
				r.Hi = &h
			}
			f.Range = &r
		}
		byID[id].Factors = append(byID[id].Factors, f)
		return nil
	}); err != nil {
		return nil, err
	}
	return tariffs, nil
}

// ShortPeriodRate kısa dönem tablosu satırı (geçen gün [from, to) → kazanılmış oran).
type ShortPeriodRate = proration.Band

func LoadShortPeriod(ctx context.Context, q db.DBTX, tariffID int64) ([]ShortPeriodRate, error) {
	rows, err := q.Query(ctx, `
		SELECT lower(days_range), upper(days_range), rate_bp
		  FROM tariff_short_period_rates WHERE tariff_version_id = $1 ORDER BY lower(days_range)`, tariffID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ShortPeriodRate, error) {
		var s ShortPeriodRate
		return s, r.Scan(&s.FromDay, &s.ToDay, &s.RateBP)
	})
}

// Select ürün + tarih için geçerli tarifeyi yükler ve seçer.
func Select(ctx context.Context, q db.DBTX, product pricing.Product, on time.Time) (pricing.Tariff, error) {
	all, err := LoadActive(ctx, q, product)
	if err != nil {
		return pricing.Tariff{}, err
	}
	return pricing.SelectTariff(all, product, on)
}
