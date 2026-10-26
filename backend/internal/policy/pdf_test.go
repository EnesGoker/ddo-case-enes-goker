package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/policypdf"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

func TestPDFDocumentUsesDetailFieldsOfLatestVersion(t *testing.T) {
	limit, event, to, plate, cancel, reason := int64(9_000_000), int64(250_000), "2026-06-01", "34 ABC 12", "2026-06-01", "CUSTOMER_REQUEST"
	d := Detail{Header: Header{PolicyNo: "GP-1", Product: "KASKO", CustomerName: "Çağrı Öz", Status: "CANCELLED",
		StartDate: "2026-01-01", EndDate: "2027-01-01", CancelEffectiveDate: &cancel, CancelReason: &reason},
		Premium: PremiumSummary{InitialKurus: 100_000, EndorsementDelta: -2_500, WrittenKurus: 97_500, RefundKurus: 40_000, NetKurus: 57_500}}
	v := VersionView{VersionNo: 2, TariffVersionNo: 3, EffectiveFrom: "2026-03-01", EffectiveTo: &to, AnnualPremium: 95_000,
		AnnualLimit: &limit, VehiclePlate: &plate, Coverages: []CoverageTerm{{Code: "CAM", Name: "Cam kırılması", LimitKurus: 500_000,
			EventLimitKurus: &event, DeductibleType: pricing.DeductibleFixed, DeductibleValue: 10_000, PremiumKurus: 5_000}}}
	at := time.Date(2026, 10, 26, 20, 0, 0, 0, time.UTC)
	require.Equal(t, policypdf.Document{PolicyNo: "GP-1", Product: "KASKO", Customer: "Çağrı Öz", Status: "CANCELLED",
		Start: "2026-01-01", End: "2027-01-01", CancelDate: cancel, CancelReason: reason, VersionFrom: "2026-03-01", VersionTo: to,
		Plate: plate, VersionNo: 2, TariffNo: 3, Annual: 95_000, Initial: 100_000, Delta: -2_500, Written: 97_500, Refund: 40_000,
		Net: 57_500, AnnualLimit: &limit, GeneratedAt: at, Coverages: []policypdf.Coverage{{Code: "CAM", Name: "Cam kırılması",
			DeductibleType: "FIXED", Limit: 500_000, Deductible: 10_000, Premium: 5_000, EventLimit: &event}}}, pdfDocument(d, v, at))
}
