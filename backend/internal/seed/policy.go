package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/clock"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
	"github.com/jackc/pgx/v5"
)

// issuePriced mevcut tarife motoru ve Issue yolunu kullanır; tarihsel sürümleri değiştirmez.
func issuePriced(ctx context.Context, tx pgx.Tx, customerID, agentID, actorID int64, no, plate string, start time.Time, count int) (policy.Issued, error) {
	var birth time.Time
	var region string
	if err := tx.QueryRow(ctx, `SELECT birth_date,region_code FROM customers WHERE id=$1`, customerID).Scan(&birth, &region); err != nil {
		return policy.Issued{}, err
	}
	normalized, ok := policy.NormalizePlate(plate)
	if !ok {
		return policy.Issued{}, fmt.Errorf("geçersiz seed plakası: %s", plate)
	}
	tariffs, err := tariff.LoadActive(ctx, tx, pricing.Kasko)
	if err != nil {
		return policy.Issued{}, err
	}
	selected, err := pricing.SelectTariff(tariffs, pricing.Kasko, start)
	if err != nil {
		return policy.Issued{}, err
	}
	age, ncd := caldate.AgeOn(birth, start), 2
	risk := policy.RiskData{Risk: pricing.Risk{DriverAge: &age, NCDLevel: &ncd, RegionCode: region, VehicleGroup: "C"}, Plate: normalized, BirthDate: caldate.Format(birth)}
	result, err := pricing.Calculate(selected, pricing.Input{Product: pricing.Kasko, Risk: risk.Risk, Coverages: []string{"KASKO_ANA"}})
	if err != nil {
		return policy.Issued{}, err
	}
	encoded, err := json.Marshal(risk)
	if err != nil {
		return policy.Issued{}, err
	}
	coverage, _ := pricing.CoverageSpecFor(pricing.Kasko, "KASKO_ANA")
	return policy.Issue(clock.WithTime(ctx, start), tx, policy.IssueInput{
		PolicyNoOverride: no, ActorID: actorID, Product: pricing.Kasko, CustomerID: customerID, AgentID: &agentID,
		TariffVersionID: selected.ID, StartDate: start, EndDate: caldate.AddYear(start), Plate: &normalized,
		RiskData: encoded, AnnualPremium: result.TotalKurus, AnnualLimitKurus: pricing.Catalog[pricing.Kasko].AnnualLimitKurus, InstallmentCount: count,
		Coverages: []policy.CoverageTerm{{Code: coverage.Code, LimitKurus: coverage.LimitKurus, DeductibleType: coverage.DeductibleType, DeductibleValue: coverage.DeductibleValue, PremiumKurus: result.TotalKurus}},
	})
}
