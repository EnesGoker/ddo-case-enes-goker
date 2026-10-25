package pricing

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sample(t *testing.T, p Product, version int) Tariff {
	t.Helper()
	for i, tr := range SampleTariffs() {
		if tr.Product == p && tr.VersionNo == version {
			tr.ID = int64(100 + i)
			return tr
		}
	}
	t.Fatalf("örnek tarife yok: %s v%d", p, version)
	return Tariff{}
}

func intp(v int) *int { return &v }

type lineCheck struct {
	code   string
	typ    LineType
	factor FactorType
	bp     int64
	amount int64
}

func assertLines(t *testing.T, res Result, want []lineCheck) {
	t.Helper()
	require.Len(t, res.Lines, len(want))
	for i, w := range want {
		l := res.Lines[i]
		assert.Equal(t, i+1, l.Seq)
		assert.Equal(t, w.code, l.CoverageCode, "satır %d", i+1)
		assert.Equal(t, w.typ, l.Type, "satır %d", i+1)
		assert.Equal(t, w.factor, l.FactorType, "satır %d", i+1)
		assert.Equal(t, w.bp, l.MultiplierBP, "satır %d", i+1)
		assert.Equal(t, w.amount, l.AmountKurus, "satır %d tutar", i+1)
	}
}

// Altın örnek 1 — KASKO v2, sürücü 23 yaş, hasarsızlık 2, bölge 34, araç grubu C, KASKO_ANA + CAM.
// Elle hesap (kuruş; her adım: yeni = (önceki × bp + 5000) / 10000, tam sayı bölme = yarımı yukarı):
//
//	KASKO_ANA taban                         1.234.567
//	  AGE  [21,26) ×1,2500: 1.234.567×12500 = 15.432.087.500 → 1.543.208,75 → 1.543.209  fark +308.642
//	  NCD  2       ×0,8000: 1.543.209×8000  = 12.345.672.000 → 1.234.567,2  → 1.234.567  fark −308.642
//	  REGION 34    ×1,2000: 1.234.567×12000 = 14.814.804.000 → 1.481.480,4  → 1.481.480  fark +246.913
//	  VG   C       ×1,2000: 1.481.480×12000 = 17.777.760.000 → 1.777.776,0  → 1.777.776  fark +296.296
//	  teminat primi 1.777.776
//	CAM taban                                  80.000
//	  AGE ×1,25 → 100.000 (+20.000); NCD ×0,80 → 80.000 (−20.000)
//	  REGION ×1,20 → 96.000 (+16.000); VG ×1,20 → 115.200 (+19.200)
//	  teminat primi 115.200
//	TOPLAM 1.777.776 + 115.200 = 1.892.976 kuruş (18.929,76 TL)
func TestGoldenKasko(t *testing.T) {
	tr := sample(t, Kasko, 2)
	res, err := Calculate(tr, Input{
		Product:   Kasko,
		Risk:      Risk{DriverAge: intp(23), NCDLevel: intp(2), RegionCode: "34", VehicleGroup: "c"},
		Coverages: []string{"CAM", "KASKO_ANA"}, // sıra fark etmez: katalog sırasına dizilir
	})
	require.NoError(t, err)

	assertLines(t, res, []lineCheck{
		{"KASKO_ANA", LineBase, "", 0, 1_234_567},
		{"KASKO_ANA", LineFactor, FactorAge, 12_500, 308_642},
		{"KASKO_ANA", LineFactor, FactorNCD, 8_000, -308_642},
		{"KASKO_ANA", LineFactor, FactorRegion, 12_000, 246_913},
		{"KASKO_ANA", LineFactor, FactorVehicleGroup, 12_000, 296_296},
		{"CAM", LineBase, "", 0, 80_000},
		{"CAM", LineFactor, FactorAge, 12_500, 20_000},
		{"CAM", LineFactor, FactorNCD, 8_000, -20_000},
		{"CAM", LineFactor, FactorRegion, 12_000, 16_000},
		{"CAM", LineFactor, FactorVehicleGroup, 12_000, 19_200},
	})
	assert.Equal(t, []CoveragePremium{{"KASKO_ANA", 1_777_776}, {"CAM", 115_200}}, res.Coverages)
	assert.Equal(t, int64(1_892_976), res.TotalKurus)
	assert.Equal(t, "Sürücü yaşı 23 (21–25)", res.Lines[1].Label)
	assert.Equal(t, "Taban prim — Kasko ana teminat", res.Lines[0].Label)
	assert.Equal(t, tr.ID, res.TariffID)
}

// Altın örnek 2 — KONUT v1, hasarsızlık 1, bölge 16 (tarifede yok → DİĞER ×1,10), YANGIN + HIRSIZLIK.
// KONUT tarifesinde yaş ve araç grubu faktörü yok; verilse de uygulanmaz.
//
//	YANGIN    150.000 → NCD 1 ×0,95 → 142.500 (−7.500) → DİĞER ×1,10 → 156.750 (+14.250)
//	HIRSIZLIK  60.000 → NCD 1 ×0,95 →  57.000 (−3.000) → DİĞER ×1,10 →  62.700 (+5.700)
//	TOPLAM 156.750 + 62.700 = 219.450 kuruş (2.194,50 TL)
func TestGoldenKonut(t *testing.T) {
	tr := sample(t, Konut, 1)
	res, err := Calculate(tr, Input{
		Product:   Konut,
		Risk:      Risk{DriverAge: intp(40), NCDLevel: intp(1), RegionCode: "16", VehicleGroup: "D"},
		Coverages: []string{"YANGIN", "HIRSIZLIK"},
	})
	require.NoError(t, err)

	assertLines(t, res, []lineCheck{
		{"YANGIN", LineBase, "", 0, 150_000},
		{"YANGIN", LineFactor, FactorNCD, 9_500, -7_500},
		{"YANGIN", LineFactor, FactorRegion, 11_000, 14_250},
		{"HIRSIZLIK", LineBase, "", 0, 60_000},
		{"HIRSIZLIK", LineFactor, FactorNCD, 9_500, -3_000},
		{"HIRSIZLIK", LineFactor, FactorRegion, 11_000, 5_700},
	})
	assert.Equal(t, int64(219_450), res.TotalKurus)
	assert.Equal(t, "Bölge 16 (diğer iller)", res.Lines[2].Label)
	assert.Nil(t, res.Risk.DriverAge, "KONUT'ta yaş kullanılmaz")
	assert.Empty(t, res.Risk.VehicleGroup)

	// Uygulanmayan girdi hash'i değiştirmez
	res2, err := Calculate(tr, Input{Product: Konut, Risk: Risk{NCDLevel: intp(1), RegionCode: "16"},
		Coverages: []string{"YANGIN", "HIRSIZLIK"}})
	require.NoError(t, err)
	assert.Equal(t, res.InputHash, res2.InputHash)
}

func TestDeterminism(t *testing.T) {
	tr := sample(t, Kasko, 2)
	in := Input{Product: Kasko, Risk: Risk{DriverAge: intp(19), NCDLevel: intp(0), RegionCode: "35", VehicleGroup: "D"},
		Coverages: []string{"KASKO_ANA", "CAM", "IMM"}}
	first, err := Calculate(tr, in)
	require.NoError(t, err)
	require.Len(t, first.InputHash, 64)
	for i := 0; i < 100; i++ {
		// Faktör listesinin sırası değişse de sonuç değişmemeli
		shuffled := tr
		shuffled.Factors = append([]Factor(nil), tr.Factors...)
		for a, b := 0, len(shuffled.Factors)-1; a < b; a, b = a+1, b-1 {
			if i%2 == 1 {
				shuffled.Factors[a], shuffled.Factors[b] = shuffled.Factors[b], shuffled.Factors[a]
			}
		}
		got, err := Calculate(shuffled, in)
		require.NoError(t, err)
		require.Equal(t, first, got, "deneme %d", i)
	}

	// Farklı tarife sürümü → farklı hash
	other := tr
	other.ID = tr.ID + 1
	got, err := Calculate(other, in)
	require.NoError(t, err)
	assert.NotEqual(t, first.InputHash, got.InputHash)
}

// Döküm toplamı = nihai prim; tüm ürün/yaş/kademe/bölge/grup kombinasyonları.
func TestBreakdownSumEqualsTotal(t *testing.T) {
	regions := []string{"34", "06", "35", "16", "81"}
	groups := []string{"A", "B", "C", "D"}
	n := 0
	for _, tr := range SampleTariffs() {
		for _, cov := range [][]string{allCoverages(tr.Product), requiredCoverages(tr.Product)} {
			for age := 18; age <= 90; age += 3 {
				for ncd := 0; ncd <= 3; ncd++ {
					for _, reg := range regions {
						for _, g := range groups {
							res, err := Calculate(tr, Input{Product: tr.Product,
								Risk:      Risk{DriverAge: intp(age), NCDLevel: intp(ncd), RegionCode: reg, VehicleGroup: g},
								Coverages: cov})
							require.NoError(t, err)
							var sum int64
							for _, l := range res.Lines {
								sum += l.AmountKurus
								assert.NotEqual(t, LineRounding, l.Type)
							}
							var covSum int64
							for _, c := range res.Coverages {
								covSum += c.PremiumKurus
								assert.Positive(t, c.PremiumKurus)
							}
							require.Equal(t, res.TotalKurus, sum)
							require.Equal(t, res.TotalKurus, covSum)
							n++
						}
					}
				}
			}
		}
	}
	assert.Greater(t, n, 5000)
}

func allCoverages(p Product) []string {
	var out []string
	for _, c := range Catalog[p].Coverages {
		out = append(out, c.Code)
	}
	return out
}

func requiredCoverages(p Product) []string {
	var out []string
	for _, c := range Catalog[p].Coverages {
		if c.Required {
			out = append(out, c.Code)
		}
	}
	return out
}

func TestAgeRangeBoundaries(t *testing.T) {
	tr := sample(t, Kasko, 2)
	cases := map[int]int64{18: 15_000, 20: 15_000, 21: 12_500, 25: 12_500, 26: 10_000, 45: 10_000,
		46: 9_500, 65: 9_500, 66: 11_500, 99: 11_500}
	for age, bp := range cases {
		res, err := Calculate(tr, Input{Product: Kasko,
			Risk:      Risk{DriverAge: intp(age), NCDLevel: intp(0), RegionCode: "06", VehicleGroup: "B"},
			Coverages: []string{"KASKO_ANA"}})
		require.NoError(t, err)
		assert.Equal(t, bp, res.Lines[1].MultiplierBP, "yaş %d", age)
	}
	_, err := Calculate(tr, Input{Product: Kasko,
		Risk:      Risk{DriverAge: intp(17), NCDLevel: intp(0), RegionCode: "06", VehicleGroup: "B"},
		Coverages: []string{"KASKO_ANA"}})
	var ie *InputError
	require.ErrorAs(t, err, &ie)
	assert.Equal(t, "driver_age", ie.Field)
}

func TestNCDLevels(t *testing.T) {
	tr := sample(t, Kasko, 2)
	want := []int64{10_000, 9_000, 8_000, 7_000, 6_000}
	for lvl, bp := range want {
		res, err := Calculate(tr, Input{Product: Kasko,
			Risk:      Risk{DriverAge: intp(30), NCDLevel: intp(lvl), RegionCode: "06", VehicleGroup: "B"},
			Coverages: []string{"KASKO_ANA"}})
		require.NoError(t, err)
		assert.Equal(t, bp, res.Lines[2].MultiplierBP)
		assert.Equal(t, FactorNCD, res.Lines[2].FactorType)
	}
	_, err := Calculate(tr, Input{Product: Kasko,
		Risk:      Risk{DriverAge: intp(30), NCDLevel: intp(5), RegionCode: "06", VehicleGroup: "B"},
		Coverages: []string{"KASKO_ANA"}})
	var ie *InputError
	require.ErrorAs(t, err, &ie)
	assert.Equal(t, "ncd_level", ie.Field)
}

func TestMulBPHalfUp(t *testing.T) {
	cases := []struct{ amount, bp, want int64 }{
		{5, 5_000, 3},          // 2,5 → 3
		{15, 5_000, 8},         // 7,5 → 8
		{1, 15_000, 2},         // 1,5 → 2
		{3, 5_000, 2},          // 1,5 → 2
		{1, 4_999, 0},          // 0,4999 → 0
		{1, 5_000, 1},          // 0,5 → 1
		{1_234_567, 12_500, 1_543_209},
		{0, 12_345, 0},
		{maxAmount, 20_000, 2 * maxAmount},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, MulBP(c.amount, c.bp), "%d×%d", c.amount, c.bp)
	}
}

func TestSelectTariffByStartDate(t *testing.T) {
	all := SampleTariffs()
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }

	v, err := SelectTariff(all, Kasko, d("2026-06-30"))
	require.NoError(t, err)
	assert.Equal(t, 1, v.VersionNo, "valid_to'dan bir gün önce eski sürüm")

	v, err = SelectTariff(all, Kasko, d("2026-07-01"))
	require.NoError(t, err)
	assert.Equal(t, 2, v.VersionNo, "valid_to günü yeni sürüm ([) aralık)")

	v, err = SelectTariff(all, Kasko, d("2025-01-01"))
	require.NoError(t, err)
	assert.Equal(t, 1, v.VersionNo)

	_, err = SelectTariff(all, Kasko, d("2024-12-31"))
	assert.ErrorIs(t, err, ErrNoTariff)

	v, err = SelectTariff(all, Konut, d("2030-01-01"))
	require.NoError(t, err)
	assert.Equal(t, Konut, v.Product)

	overlap := append(all, Tariff{Product: Konut, VersionNo: 9, ValidFrom: d("2025-06-01")})
	_, err = SelectTariff(overlap, Konut, d("2025-07-01"))
	assert.Error(t, err)
}

func TestCoverageValidation(t *testing.T) {
	tr := sample(t, Kasko, 2)
	risk := Risk{DriverAge: intp(30), NCDLevel: intp(0), RegionCode: "06", VehicleGroup: "B"}
	for name, cov := range map[string][]string{
		"boş":            nil,
		"zorunlu eksik":  {"CAM"},
		"bilinmeyen":     {"KASKO_ANA", "UZAY"},
		"tekrarlı":       {"KASKO_ANA", "kasko_ana"},
		"başka ürününkü": {"KASKO_ANA", "YANGIN"},
	} {
		_, err := Calculate(tr, Input{Product: Kasko, Risk: risk, Coverages: cov})
		var ie *InputError
		require.ErrorAs(t, err, &ie, name)
		assert.Equal(t, "coverages", ie.Field, name)
	}

	_, err := Calculate(tr, Input{Product: Konut, Risk: risk, Coverages: []string{"YANGIN"}})
	assert.Error(t, err, "tarife ürünü uyuşmuyor")

	// Teminat katalogda var ama tarifede taban oranı yoksa reddedilir
	noCam := tr
	noCam.BaseRates = map[string]int64{"KASKO_ANA": 1}
	_, err = Calculate(noCam, Input{Product: Kasko, Risk: risk, Coverages: []string{"KASKO_ANA", "CAM"}})
	var ie *InputError
	assert.True(t, errors.As(err, &ie))
}

func TestMissingRiskInputs(t *testing.T) {
	tr := sample(t, Trafik, 1)
	full := Risk{DriverAge: intp(30), NCDLevel: intp(2), RegionCode: "06", VehicleGroup: "B"}
	for field, r := range map[string]Risk{
		"driver_age":    {NCDLevel: full.NCDLevel, RegionCode: "06", VehicleGroup: "B"},
		"ncd_level":     {DriverAge: full.DriverAge, RegionCode: "06", VehicleGroup: "B"},
		"region_code":   {DriverAge: full.DriverAge, NCDLevel: full.NCDLevel, VehicleGroup: "B"},
		"vehicle_group": {DriverAge: full.DriverAge, NCDLevel: full.NCDLevel, RegionCode: "06"},
	} {
		_, err := Calculate(tr, Input{Product: Trafik, Risk: r, Coverages: requiredCoverages(Trafik)})
		var ie *InputError
		require.ErrorAs(t, err, &ie, field)
		assert.Equal(t, field, ie.Field)
	}
	_, err := Calculate(tr, Input{Product: Trafik, Risk: Risk{DriverAge: intp(30), NCDLevel: intp(2),
		RegionCode: "06", VehicleGroup: "Z"}, Coverages: requiredCoverages(Trafik)})
	assert.Error(t, err, "bilinmeyen araç grubu")
}

func TestReviewReasons(t *testing.T) {
	tr := sample(t, Kasko, 2)
	calc := func(age int, group string) Result {
		res, err := Calculate(tr, Input{Product: Kasko,
			Risk:      Risk{DriverAge: intp(age), NCDLevel: intp(0), RegionCode: "34", VehicleGroup: group},
			Coverages: []string{"KASKO_ANA", "CAM", "IMM"}})
		require.NoError(t, err)
		return res
	}
	assert.Empty(t, ReviewReasons(calc(30, "A"), 3_000_000))
	assert.Equal(t, []string{ReasonYoungDriver}, ReviewReasons(calc(20, "A"), 10_000_000))
	young := calc(19, "D")
	assert.Greater(t, young.TotalKurus, int64(3_000_000), fmt.Sprint(young.TotalKurus))
	assert.Equal(t, []string{ReasonYoungDriver, ReasonHighPremium}, ReviewReasons(young, 3_000_000))
	assert.Empty(t, ReviewReasons(calc(30, "D"), 0), "eşik 0 = kapalı")
}

func TestSampleTariffsAreWellFormed(t *testing.T) {
	for _, tr := range SampleTariffs() {
		require.True(t, tr.Product.Valid())
		for _, c := range Catalog[tr.Product].Coverages {
			assert.Contains(t, tr.BaseRates, c.Code, "%s v%d", tr.Product, tr.VersionNo)
		}
		for _, f := range tr.Factors {
			assert.Positive(t, f.MultiplierBP)
			assert.True(t, (f.Range == nil) != (f.Key == ""), "faktör ya anahtarlı ya aralıklı olmalı")
		}
	}
}
