package pricing

import "time"

// SampleTariffs demo seed'inin yüklediği ÖRNEK tarifelerdir. Tüm oran ve tutarlar KURGUSALDIR;
// gerçek bir sigorta şirketinin tarifesi veya mevzuat değeri değildir. ID'ler veritabanında atanır.
// Altın testler bu tanımları kullanır; seed ile testler aynı veriye dayanır.
func SampleTariffs() []Tariff {
	return []Tariff{
		{
			Product: Kasko, VersionNo: 1,
			ValidFrom: date(2025, 1, 1), ValidTo: ptr(date(2026, 7, 1)),
			BaseRates: map[string]int64{"KASKO_ANA": 1_000_000, "CAM": 70_000, "IMM": 40_000},
			Factors: concat(
				ages(map[[2]int]int64{{18, 21}: 14_000, {21, 26}: 12_500, {26, 46}: 10_000, {46, 66}: 9_500, {66, 0}: 11_500}),
				keyed(FactorNCD, map[string]int64{"0": 10_000, "1": 9_200, "2": 8_400, "3": 7_600, "4": 6_800}),
				keyed(FactorRegion, map[string]int64{"34": 11_500, "06": 10_000, "35": 10_500, RegionFallbackKey: 9_500}),
				keyed(FactorVehicleGroup, map[string]int64{"A": 9_000, "B": 10_000, "C": 11_500, "D": 14_000}),
			),
		},
		{
			Product: Kasko, VersionNo: 2,
			ValidFrom: date(2026, 7, 1),
			BaseRates: map[string]int64{"KASKO_ANA": 1_234_567, "CAM": 80_000, "IMM": 45_000},
			Factors: concat(
				ages(map[[2]int]int64{{18, 21}: 15_000, {21, 26}: 12_500, {26, 46}: 10_000, {46, 66}: 9_500, {66, 0}: 11_500}),
				keyed(FactorNCD, map[string]int64{"0": 10_000, "1": 9_000, "2": 8_000, "3": 7_000, "4": 6_000}),
				keyed(FactorRegion, map[string]int64{"34": 12_000, "06": 10_500, "35": 11_000, RegionFallbackKey: 10_000}),
				keyed(FactorVehicleGroup, map[string]int64{"A": 9_000, "B": 10_000, "C": 12_000, "D": 14_500}),
			),
		},
		{
			Product: Trafik, VersionNo: 1,
			ValidFrom: date(2025, 1, 1),
			BaseRates: map[string]int64{"TRAFIK_MADDI": 250_000, "TRAFIK_BEDENI": 350_000},
			Factors: concat(
				ages(map[[2]int]int64{{18, 21}: 17_500, {21, 26}: 13_000, {26, 66}: 10_000, {66, 0}: 12_000}),
				keyed(FactorNCD, map[string]int64{"0": 13_500, "1": 11_500, "2": 10_000, "3": 9_000, "4": 8_000}),
				keyed(FactorRegion, map[string]int64{"34": 11_500, "06": 10_000, "35": 10_500, RegionFallbackKey: 9_500}),
				keyed(FactorVehicleGroup, map[string]int64{"A": 9_000, "B": 10_000, "C": 11_000, "D": 13_000}),
			),
		},
		{
			Product: Konut, VersionNo: 1,
			ValidFrom: date(2025, 1, 1),
			BaseRates: map[string]int64{"YANGIN": 150_000, "HIRSIZLIK": 60_000},
			Factors: concat(
				keyed(FactorNCD, map[string]int64{"0": 10_000, "1": 9_500, "2": 9_000, "3": 8_500}),
				keyed(FactorRegion, map[string]int64{"34": 13_000, "35": 12_500, "06": 10_000, RegionFallbackKey: 11_000}),
			),
		},
	}
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ptr[T any](v T) *T { return &v }

// ages {alt, üst} → çarpan; üst 0 = sınırsız.
func ages(m map[[2]int]int64) []Factor {
	var out []Factor
	for k, bp := range m {
		r := Range{Lo: k[0]}
		if k[1] != 0 {
			r.Hi = ptr(k[1])
		}
		out = append(out, Factor{Type: FactorAge, Range: &r, MultiplierBP: bp})
	}
	return out
}

func keyed(t FactorType, m map[string]int64) []Factor {
	var out []Factor
	for k, bp := range m {
		out = append(out, Factor{Type: t, Key: k, MultiplierBP: bp})
	}
	return out
}

func concat(parts ...[]Factor) []Factor {
	var out []Factor
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
