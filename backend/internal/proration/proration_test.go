package proration

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(s string) time.Time {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

func period(a, b string) Period { return Period{Start: d(a), End: d(b)} }

func TestPeriodDaysAndLeapDay(t *testing.T) {
	cases := []struct {
		start, end string
		days       int64
		leap       bool
	}{
		{"2027-06-01", "2028-06-01", 366, true}, // 2028-02-29 dönem içinde
		{"2026-01-01", "2027-01-01", 365, false},
		{"2026-11-01", "2027-11-01", 365, false},
		{"2027-03-01", "2028-03-01", 366, true},
		{"2028-03-01", "2029-03-01", 365, false}, // 29 Şubat başlangıçtan önce
		{"2028-02-29", "2029-03-01", 366, true},  // başlangıç günü 29 Şubat (dahil)
		{"2027-03-01", "2028-02-29", 365, false}, // bitiş günü 29 Şubat (hariç, [) )
		{"2028-02-28", "2028-03-01", 2, true},
		{"2026-12-31", "2027-01-01", 1, false}, // yıl sonu
	}
	for _, c := range cases {
		p := period(c.start, c.end)
		assert.Equal(t, c.days, p.Days(), c.start+"→"+c.end)
		assert.Equal(t, c.leap, p.HasLeapDay(), c.start+"→"+c.end)
	}
}

func TestElapsedRemainingAroundLeapDay(t *testing.T) {
	p := period("2027-06-01", "2028-06-01")
	feb29 := d("2028-02-29")
	assert.Equal(t, int64(273), p.Elapsed(feb29))
	assert.Equal(t, int64(93), p.Remaining(feb29))
	assert.Equal(t, p.Days(), p.Elapsed(feb29)+p.Remaining(feb29))

	assert.Equal(t, int64(1), DaysBetween(d("2028-02-28"), d("2028-02-29")))
	assert.Equal(t, int64(1), DaysBetween(d("2028-02-29"), d("2028-03-01")))
	assert.Equal(t, int64(1), DaysBetween(d("2027-12-31"), d("2028-01-01")))
}

func TestRoundDiv(t *testing.T) {
	cases := []struct{ num, den, want int64 }{
		{5, 2, 3}, {-5, 2, -3}, // ±2,5 → ±3 (sıfırdan uzağa)
		{1, 2, 1}, {-1, 2, -1},
		{4, 2, 2}, {1, 3, 0}, {2, 3, 1}, {-1, 3, 0}, {-2, 3, -1},
		{72_879_576, 365, 199_670}, // 199.670,0712
		{0, 365, 0},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, RoundDiv(c.num, c.den), "%d/%d", c.num, c.den)
	}
}

func TestFormatting(t *testing.T) {
	assert.Equal(t, "199670.0712", FormatRatio(72_879_576, 365))
	assert.Equal(t, "-0.5000", FormatRatio(-1, 2))
	assert.Equal(t, "1463902.4000", FormatRatio(1_829_878*8_000, 10_000))
	assert.Equal(t, "1.514.382", Kurus(1_514_382))
	assert.Equal(t, "−315.496", Kurus(-315_496))
	assert.Equal(t, "+315.496", SignedKurus(315_496))
	assert.Equal(t, "0", SignedKurus(0))
	assert.Equal(t, "199.670,0712", Decimal("199670.0712"))
	assert.Equal(t, "−0,5000", Decimal("-0.5000"))
	assert.Equal(t, "%80,00", Percent(8_000))
	assert.Equal(t, "%2,50", Percent(250))
}

// Altın örnek 1 — araç değişikliği (A2 altın girdisiyle uyumlu gerçek primler):
//
//	Poliçe 2026-11-01 → 2027-11-01: 365 gün (2027 artık yıl değil).
//	Zeyil 2027-03-15: geçen = 30 (Kas) + 31 (Ara) + 31 (Oca) + 28 (Şub) + 14 = 134; kalan = 365 − 134 = 231.
//	Eski yıllık prim 1.514.382 (araç grubu C), yeni 1.829.878 (araç grubu D: KASKO_ANA 1.718.518 + CAM 111.360).
//	Fark = 1.829.878 − 1.514.382 = 315.496 kuruş.
//	Delta = 315.496 × 231 / 365 = 72.879.576 / 365 = 199.670 + 26/365 = 199.670,0712 → 199.670 kuruş ek prim.
func TestGoldenVehicleChange(t *testing.T) {
	det, err := VehicleChange{
		Period: period("2026-11-01", "2027-11-01"), EffectiveDate: d("2027-03-15"),
		OldAnnualKurus: 1_514_382, NewAnnualKurus: 1_829_878,
	}.Calculate()
	require.NoError(t, err)

	assert.Equal(t, "VEHICLE_CHANGE", det.Kind)
	assert.Equal(t, MethodProRata, det.Method)
	assert.Equal(t, int64(365), det.PeriodDays)
	assert.False(t, det.LeapDayInPeriod)
	assert.Equal(t, int64(134), det.ElapsedDays)
	assert.Equal(t, int64(231), det.RemainingDays)
	assert.Equal(t, int64(315_496), *det.AnnualDiffKurus)
	assert.Equal(t, int64(72_879_576), det.Numerator)
	assert.Equal(t, int64(365), det.Denominator)
	assert.Equal(t, "199670.0712", det.Unrounded)
	assert.Equal(t, int64(199_670), det.Rounded)
	assert.Equal(t, int64(199_670), det.PremiumDeltaKurus)
	assert.Equal(t, RoundingRule, det.Rounding)
	require.Len(t, det.Steps, 5)
	assert.Contains(t, det.Steps[0], "365 gün (29 Şubat içermiyor)")
	assert.Contains(t, det.Steps[3], "+315.496 × 231 / 365 = 199.670,0712")
	assert.Contains(t, det.Steps[4], "199.670 kuruş ek prim")
}

func TestVehicleChangeSignAndRounding(t *testing.T) {
	p := period("2026-11-01", "2027-11-01")
	down, err := VehicleChange{Period: p, EffectiveDate: d("2027-03-15"),
		OldAnnualKurus: 1_829_878, NewAnnualKurus: 1_514_382}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(-199_670), down.PremiumDeltaKurus, "prim düşüşü simetrik iade")
	assert.Contains(t, down.Steps[4], "iade")

	same, err := VehicleChange{Period: p, EffectiveDate: d("2027-03-15"),
		OldAnnualKurus: 1_000, NewAnnualKurus: 1_000}.Calculate()
	require.NoError(t, err)
	assert.Zero(t, same.PremiumDeltaKurus)

	// Yarım kuruş: 1 × 1 / 2 = 0,5 → +1; −0,5 → −1 (sıfırdan uzağa)
	two := period("2026-01-01", "2026-01-03")
	up, err := VehicleChange{Period: two, EffectiveDate: d("2026-01-02"), OldAnnualKurus: 0, NewAnnualKurus: 1}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(1), up.PremiumDeltaKurus)
	assert.Equal(t, "0.5000", up.Unrounded)
	dn, err := VehicleChange{Period: two, EffectiveDate: d("2026-01-02"), OldAnnualKurus: 1, NewAnnualKurus: 0}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(-1), dn.PremiumDeltaKurus)
}

// 29 Şubat tarihli zeyil, 366 günlük dönemde: 366.000 × 93 / 366 = 93.000 (tam).
func TestVehicleChangeOnLeapDay(t *testing.T) {
	det, err := VehicleChange{Period: period("2027-06-01", "2028-06-01"), EffectiveDate: d("2028-02-29"),
		OldAnnualKurus: 1_000_000, NewAnnualKurus: 1_366_000}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(366), det.PeriodDays)
	assert.True(t, det.LeapDayInPeriod)
	assert.Equal(t, int64(273), det.ElapsedDays)
	assert.Equal(t, int64(93), det.RemainingDays)
	assert.Equal(t, int64(93_000), det.PremiumDeltaKurus)
	assert.Equal(t, "93000.0000", det.Unrounded)
	assert.Contains(t, det.Steps[0], "366 gün (29 Şubat içeriyor)")
}

func TestVehicleChangeDateBounds(t *testing.T) {
	p := period("2026-11-01", "2027-11-01")
	for _, date := range []string{"2026-11-01", "2027-11-01", "2026-10-31", "2028-01-01"} {
		_, err := VehicleChange{Period: p, EffectiveDate: d(date), OldAnnualKurus: 1, NewAnnualKurus: 2}.Calculate()
		var ie *InputError
		require.ErrorAs(t, err, &ie, date)
		assert.Equal(t, "effective_date", ie.Field)
	}
	_, err := VehicleChange{Period: p, EffectiveDate: d("2026-11-02"), OldAnnualKurus: 1, NewAnnualKurus: 2}.Calculate()
	assert.NoError(t, err, "başlangıçtan bir gün sonrası geçerli")
	_, err = VehicleChange{Period: p, EffectiveDate: d("2027-10-31"), OldAnnualKurus: 1, NewAnnualKurus: 2}.Calculate()
	assert.NoError(t, err, "son gün geçerli (kalan 1 gün)")
	_, err = VehicleChange{Period: period("2027-01-01", "2026-01-01"), EffectiveDate: d("2026-06-01")}.Calculate()
	assert.Error(t, err, "ters dönem")
}

func TestShortPeriodBands(t *testing.T) {
	table := SampleShortPeriodTable()
	cases := map[int64]int{0: 2_500, 30: 2_500, 31: 4_000, 90: 4_000, 91: 6_000, 180: 6_000,
		181: 8_000, 270: 8_000, 271: 10_000, 365: 10_000, 366: 10_000}
	for day, want := range cases {
		b, err := ShortPeriodRate(table, day)
		require.NoError(t, err, day)
		assert.Equal(t, want, b.RateBP, "%d gün", day)
	}
	for _, day := range []int64{-1, 367, 1000} {
		_, err := ShortPeriodRate(table, day)
		assert.True(t, errors.Is(err, ErrNoShortPeriodRate), day)
	}
	open := []Band{{FromDay: 0, RateBP: 5_000}}
	b, err := ShortPeriodRate(open, 10_000)
	require.NoError(t, err)
	assert.Equal(t, "[0, ∞)", b.String())
}

// Altın örnek 2 — müşteri talebiyle kısa dönem iptal (altın örnek 1'deki zeyilden sonra):
//
//	Dönem 2026-11-01 → 2027-11-01 = 365 gün. İptal 2027-06-10:
//	geçen = 30 + 31 + 31 + 28 + 31 + 30 + 31 (Kas–May) + 9 = 221 gün, kalan 144.
//	Kısa dönem: 221 ∈ [181, 271) → %80. Açık sürümün yıllık primi 1.829.878.
//	Kazanılmış = 1.829.878 × 8000 / 10000 = 1.463.902,4 → 1.463.902.
//	Yazılmış = 1.514.382 (ilk) + 199.670 (zeyil) = 1.714.052.
//	İade = 1.714.052 − 1.463.902 = 250.150 kuruş → premium_delta = −250.150.
func TestGoldenShortPeriodCancellation(t *testing.T) {
	det, err := Cancellation{
		Period: period("2026-11-01", "2027-11-01"), EffectiveDate: d("2027-06-10"), Reason: ReasonCustomerRequest,
		AnnualKurus: 1_829_878, WrittenKurus: 1_714_052, ShortPeriod: SampleShortPeriodTable(),
	}.Calculate()
	require.NoError(t, err)

	assert.Equal(t, "CANCELLATION", det.Kind)
	assert.Equal(t, MethodShortPeriod, det.Method)
	assert.Equal(t, int64(221), det.ElapsedDays)
	assert.Equal(t, int64(144), det.RemainingDays)
	require.NotNil(t, det.ShortPeriodBand)
	assert.Equal(t, 8_000, det.ShortPeriodBand.RateBP)
	assert.Equal(t, "1463902.4000", det.Unrounded)
	assert.Equal(t, int64(1_463_902), *det.EarnedKurus)
	assert.Equal(t, int64(250_150), *det.RefundKurus)
	assert.Equal(t, int64(-250_150), det.PremiumDeltaKurus)
	assert.Contains(t, strings.Join(det.Steps, "\n"), "221 gün ∈ [181, 271) → kazanılmış oran %80,00")
	assert.Contains(t, strings.Join(det.Steps, "\n"), "İade: 1.714.052 − 1.463.902 = 250.150 kuruş")
}

// Aynı tarihte şirket kaynaklı (gün esaslı) iptal:
//
//	İade = 1.829.878 × 144 / 365 = 263.502.432 / 365 = 721.924,4712 → 721.924 kuruş.
func TestProRataCancellation(t *testing.T) {
	for _, reason := range []string{ReasonUnderwriter, ReasonNonPayment, ReasonOther} {
		det, err := Cancellation{
			Period: period("2026-11-01", "2027-11-01"), EffectiveDate: d("2027-06-10"), Reason: reason,
			AnnualKurus: 1_829_878, WrittenKurus: 1_714_052,
		}.Calculate()
		require.NoError(t, err, reason)
		assert.Equal(t, MethodProRata, det.Method)
		assert.Equal(t, "721924.4712", det.Unrounded)
		assert.Equal(t, int64(721_924), *det.RefundKurus)
		assert.Equal(t, int64(-721_924), det.PremiumDeltaKurus)
		assert.Nil(t, det.ShortPeriodBand)
		assert.False(t, det.RefundLimited)
	}
}

func TestCancellationBoundaries(t *testing.T) {
	p := period("2026-11-01", "2027-11-01")
	table := SampleShortPeriodTable()

	// Başlangıç gününde iptal: geçen 0 gün
	start, err := Cancellation{Period: p, EffectiveDate: d("2026-11-01"), Reason: ReasonUnderwriter,
		AnnualKurus: 365_000, WrittenKurus: 365_000}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(0), start.ElapsedDays)
	assert.Equal(t, int64(365), start.RemainingDays)
	assert.Equal(t, int64(365_000), *start.RefundKurus, "gün esaslı: tamamı iade")

	startSP, err := Cancellation{Period: p, EffectiveDate: d("2026-11-01"), Reason: ReasonCustomerRequest,
		AnnualKurus: 365_000, WrittenKurus: 365_000, ShortPeriod: table}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(91_250), *startSP.EarnedKurus, "kısa dönem 0 gün → %25 kazanılmış")
	assert.Equal(t, int64(273_750), *startSP.RefundKurus)

	// Son günde iptal: kalan 1 gün
	last, err := Cancellation{Period: p, EffectiveDate: d("2027-10-31"), Reason: ReasonUnderwriter,
		AnnualKurus: 365_000, WrittenKurus: 365_000}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(364), last.ElapsedDays)
	assert.Equal(t, int64(1), last.RemainingDays)
	assert.Equal(t, int64(1_000), *last.RefundKurus)

	lastSP, err := Cancellation{Period: p, EffectiveDate: d("2027-10-31"), Reason: ReasonCustomerRequest,
		AnnualKurus: 365_000, WrittenKurus: 365_000, ShortPeriod: table}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(0), *lastSP.RefundKurus, "%100 kazanılmış → iade yok")

	// Dönem dışı tarihler
	for _, date := range []string{"2026-10-31", "2027-11-01", "2028-01-01"} {
		_, err := Cancellation{Period: p, EffectiveDate: d(date), Reason: ReasonUnderwriter, AnnualKurus: 1}.Calculate()
		var ie *InputError
		require.ErrorAs(t, err, &ie, date)
		assert.Equal(t, "effective_date", ie.Field)
	}
}

func TestCancellationOnLeapDay(t *testing.T) {
	p := period("2027-06-01", "2028-06-01")
	pr, err := Cancellation{Period: p, EffectiveDate: d("2028-02-29"), Reason: ReasonUnderwriter,
		AnnualKurus: 3_660_000, WrittenKurus: 3_660_000}.Calculate()
	require.NoError(t, err)
	assert.True(t, pr.LeapDayInPeriod)
	assert.Equal(t, int64(366), pr.PeriodDays)
	assert.Equal(t, int64(930_000), *pr.RefundKurus, "3.660.000 × 93 / 366")

	sp, err := Cancellation{Period: p, EffectiveDate: d("2028-02-29"), Reason: ReasonCustomerRequest,
		AnnualKurus: 3_660_000, WrittenKurus: 3_660_000, ShortPeriod: SampleShortPeriodTable()}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, 10_000, sp.ShortPeriodBand.RateBP, "273 gün → [271, 367)")
	assert.Equal(t, int64(0), *sp.RefundKurus)
}

func TestRefundNeverNegativeOrAboveWritten(t *testing.T) {
	p := period("2026-11-01", "2027-11-01")
	// Kısa dönem: kazanılmış > yazılmış → iade 0
	sp, err := Cancellation{Period: p, EffectiveDate: d("2027-09-01"), Reason: ReasonCustomerRequest,
		AnnualKurus: 2_000, WrittenKurus: 1_000, ShortPeriod: SampleShortPeriodTable()}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(0), *sp.RefundKurus)
	assert.Equal(t, int64(0), sp.PremiumDeltaKurus)

	// Gün esaslı: hesaplanan iade yazılmış primi aşarsa yazılmışla sınırlanır
	pr, err := Cancellation{Period: p, EffectiveDate: d("2026-11-02"), Reason: ReasonUnderwriter,
		AnnualKurus: 3_650_000, WrittenKurus: 1_000_000}.Calculate()
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000), *pr.RefundKurus)
	assert.True(t, pr.RefundLimited)
}

func TestCancellationErrors(t *testing.T) {
	p := period("2026-11-01", "2027-11-01")
	_, err := Cancellation{Period: p, EffectiveDate: d("2027-01-01"), Reason: "KEYFI", AnnualKurus: 1}.Calculate()
	var ie *InputError
	require.ErrorAs(t, err, &ie)
	assert.Equal(t, "reason", ie.Field)

	_, err = Cancellation{Period: p, EffectiveDate: d("2027-01-01"), Reason: ReasonCustomerRequest,
		AnnualKurus: 1, WrittenKurus: 1, ShortPeriod: nil}.Calculate()
	assert.ErrorIs(t, err, ErrNoShortPeriodRate, "tablo yoksa yapılandırma hatası")

	m, err := MethodFor(ReasonCustomerRequest)
	require.NoError(t, err)
	assert.Equal(t, MethodShortPeriod, m)
}
