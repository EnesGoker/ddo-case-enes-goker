package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

// shortPeriodRates örnek kısa dönem tablosu (KURGUSAL): poliçe kaç gün yaşadıysa yıllık primin
// yüzde kaçı kazanılmış sayılır. A3 (iptal) kullanacak.
var shortPeriodRates = []struct {
	from, to int
	rateBP   int
}{
	{0, 31, 2_500}, {31, 91, 4_000}, {91, 181, 6_000}, {181, 271, 8_000}, {271, 367, 10_000},
}

// tariffs pricing.SampleTariffs()'ı (ürün, sürüm) yoksa ACTIVE olarak ekler.
func tariffs(ctx context.Context, tx pgx.Tx) error {
	for _, t := range pricing.SampleTariffs() {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM tariff_versions WHERE product = $1 AND version_no = $2)`,
			string(t.Product), t.VersionNo).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tariff_versions (product, version_no, valid_from, valid_to, status)
			VALUES ($1, $2, $3, $4, 'ACTIVE') RETURNING id`,
			string(t.Product), t.VersionNo, t.ValidFrom, t.ValidTo).Scan(&id)
		if err != nil {
			return fmt.Errorf("tarife %s v%d: %w", t.Product, t.VersionNo, err)
		}
		for code, amount := range t.BaseRates {
			if _, err := tx.Exec(ctx,
				`INSERT INTO tariff_base_rates (tariff_version_id, coverage_code, base_premium_kurus) VALUES ($1, $2, $3)`,
				id, code, amount); err != nil {
				return err
			}
		}
		for _, f := range t.Factors {
			var key *string
			var lo, hi *int
			if f.Range != nil {
				lo, hi = &f.Range.Lo, f.Range.Hi
			} else {
				k := f.Key
				key = &k
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO tariff_factors (tariff_version_id, factor_type, factor_key, num_range, multiplier_bp)
				VALUES ($1, $2, $3, CASE WHEN $4::int IS NULL AND $5::int IS NULL THEN NULL
				                         ELSE int4range($4::int, $5::int, '[)') END, $6)`,
				id, string(f.Type), key, lo, hi, f.MultiplierBP); err != nil {
				return err
			}
		}
		for _, s := range shortPeriodRates {
			if _, err := tx.Exec(ctx, `
				INSERT INTO tariff_short_period_rates (tariff_version_id, days_range, rate_bp)
				VALUES ($1, int4range($2, $3, '[)'), $4)`, id, s.from, s.to, s.rateBP); err != nil {
				return err
			}
		}
	}
	return nil
}
