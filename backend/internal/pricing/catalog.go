package pricing

// Teminat kataloğu: limit ve muafiyetler. KURGUSALDIR — gerçek ürün/mevzuat değildir.
// Prim tarife tablosundan gelir (tariff_base_rates); limit ve muafiyet seçilen teminatla birlikte
// teklife kopyalanır, böylece katalog değişse de eski teklif/poliçe etkilenmez.

type Product string

const (
	Kasko  Product = "KASKO"
	Trafik Product = "TRAFIK"
	Konut  Product = "KONUT"
)

func (p Product) Valid() bool { _, ok := Catalog[p]; return ok }

// IsVehicle araç ürünlerinde plaka ve araç grubu zorunludur.
func (p Product) IsVehicle() bool { return p == Kasko || p == Trafik }

type DeductibleType string

const (
	DeductibleNone      DeductibleType = "NONE"
	DeductibleFixed     DeductibleType = "FIXED"      // kuruş
	DeductiblePercentBP DeductibleType = "PERCENT_BP" // hasarın bp oranı
)

type CoverageSpec struct {
	Code            string         `json:"code"`
	Name            string         `json:"name"`
	Required        bool           `json:"required"`
	LimitKurus      int64          `json:"limit_kurus"`
	DeductibleType  DeductibleType `json:"deductible_type"`
	DeductibleValue int64          `json:"deductible_value"`
}

type ProductSpec struct {
	Coverages        []CoverageSpec `json:"coverages"`
	AnnualLimitKurus int64          `json:"annual_limit_kurus"` // yıllık toplam limit
}

var Catalog = map[Product]ProductSpec{
	Kasko: {
		AnnualLimitKurus: 180_000_000,
		Coverages: []CoverageSpec{
			{"KASKO_ANA", "Kasko ana teminat", true, 150_000_000, DeductiblePercentBP, 200},
			{"CAM", "Cam kırılması", false, 2_500_000, DeductibleFixed, 50_000},
			{"IMM", "İhtiyari mali mesuliyet", false, 50_000_000, DeductibleNone, 0},
		},
	},
	Trafik: {
		AnnualLimitKurus: 300_000_000,
		Coverages: []CoverageSpec{
			{"TRAFIK_MADDI", "Maddi hasar (araç başına)", true, 30_000_000, DeductibleNone, 0},
			{"TRAFIK_BEDENI", "Sakatlanma ve ölüm (kişi başına)", true, 270_000_000, DeductibleNone, 0},
		},
	},
	Konut: {
		AnnualLimitKurus: 210_000_000,
		Coverages: []CoverageSpec{
			{"YANGIN", "Yangın ve bina", true, 200_000_000, DeductibleFixed, 100_000},
			{"HIRSIZLIK", "Hırsızlık", false, 10_000_000, DeductiblePercentBP, 1000},
		},
	},
}

// CoverageSpecFor ürün kataloğundaki teminatı döner.
func CoverageSpecFor(p Product, code string) (CoverageSpec, bool) {
	for _, c := range Catalog[p].Coverages {
		if c.Code == code {
			return c, true
		}
	}
	return CoverageSpec{}, false
}
