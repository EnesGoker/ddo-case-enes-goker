package seed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

// Tarihler yalnızca ilk oluşturulurken bugüne göre seçilir; tekrar seed geçmişi değiştirmez.
func billingPolicies(ctx context.Context, tx pgx.Tx, agentID, actorID int64) error {
	var customerID, tariffID int64
	if err := tx.QueryRow(ctx, `
  SELECT id FROM customers WHERE national_id = '10000000001'`).Scan(&customerID); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `
  SELECT id FROM tariff_versions WHERE product = 'KASKO' AND version_no = 2`).Scan(&tariffID); err != nil {
		return err
	}
	for _, sample := range []struct {
		no   string
		days int
	}{{"GP-DEMO-OVERDUE-A", 75}, {"GP-DEMO-OVERDUE-B", 20}} {
		var exists bool
		if err := tx.QueryRow(ctx, `
   SELECT EXISTS (SELECT 1 FROM policies WHERE policy_no = $1)`, sample.no).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		start := caldate.Today(time.Now()).AddDate(0, 0, -sample.days)
		plate := "34DEMO55"
		_, err := policy.Issue(ctx, tx, policy.IssueInput{
			PolicyNoOverride: sample.no, ActorID: actorID, Product: pricing.Kasko,
			CustomerID: customerID, AgentID: &agentID, TariffVersionID: tariffID,
			StartDate: start, EndDate: caldate.AddYear(start), Plate: &plate,
			RiskData:      []byte(`{"driver_age":38,"ncd_level":2,"region_code":"34","vehicle_group":"C","plate":"34DEMO55"}`),
			AnnualPremium: 300000, AnnualLimitKurus: 180000000, InstallmentCount: 3,
			Coverages: []policy.CoverageTerm{{Code: "KASKO_ANA", LimitKurus: 150000000, DeductibleType: pricing.DeductiblePercentBP, DeductibleValue: 200, PremiumKurus: 300000}},
		})
		if err != nil {
			return err
		}
	}
	return nil
}
