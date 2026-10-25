// Package proration zeyil ve iptal prim hesaplarıdır (T-2): veritabanı ve saat kullanmaz,
// yalnızca tam sayı aritmetiği.
//
// Kurallar (A3 kararı, kurgusal iş varsayımı):
//   - Dönem gün sayısı = end − start (gerçek gün; içinde 29 Şubat varsa 366). Aralıklar [başlangıç, bitiş).
//   - Geçen gün = tarih − start, kalan gün = end − tarih.
//   - Araç değişikliği: delta = (yeni yıllık − eski yıllık) × kalan / dönem; + ek prim, − iade.
//   - İptal, müşteri talebi: kısa dönem. Kazanılmış = yıllık × oran(geçen gün); iade = yazılmış − kazanılmış (≥ 0).
//   - İptal, şirket kaynaklı (UNDERWRITER, NON_PAYMENT, OTHER): gün esaslı. İade = yıllık × kalan / dönem (≤ yazılmış).
//   - Yuvarlama: kuruşa, yarımı yukarı; negatif tutarlarda mutlak değer üzerinden (sıfırdan uzağa),
//     böylece +X ve −X fark aynı büyüklükte delta üretir.
package proration

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// RoundingRule calc_detail'de yazılan yuvarlama kuralı adı.
const RoundingRule = "HALF_AWAY_FROM_ZERO"

// InputError hesap girdisi hatasıdır (tarih dönem dışında, bilinmeyen gerekçe vb.).
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return e.Field + ": " + e.Message }

func inputErr(field, format string, a ...any) error {
	return &InputError{Field: field, Message: fmt.Sprintf(format, a...)}
}

// ErrNoShortPeriodRate geçen gün sayısı kısa dönem tablosunda bulunamadı (tarife yapılandırma hatası).
var ErrNoShortPeriodRate = errors.New("kısa dönem tablosunda bu gün sayısı için oran yok")

// DaysBetween a'dan b'ye takvim günü sayısı (b − a). Tarihler UTC gece yarısı olmalıdır.
func DaysBetween(a, b time.Time) int64 {
	return int64(b.Sub(a) / (24 * time.Hour))
}

// Period poliçe dönemi [Start, End).
type Period struct {
	Start, End time.Time
}

func (p Period) Days() int64                 { return DaysBetween(p.Start, p.End) }
func (p Period) Elapsed(d time.Time) int64   { return DaysBetween(p.Start, d) }
func (p Period) Remaining(d time.Time) int64 { return DaysBetween(d, p.End) }
func (p Period) Contains(d time.Time) bool   { return !d.Before(p.Start) && d.Before(p.End) }
func (p Period) String() string {
	return p.Start.Format(dateLayout) + " → " + p.End.Format(dateLayout)
}
func (p Period) validate() error {
	if !p.End.After(p.Start) {
		return inputErr("period", "dönem bitişi başlangıçtan sonra olmalı")
	}
	return nil
}

// HasLeapDay dönemin bir 29 Şubat içerip içermediği.
func (p Period) HasLeapDay() bool {
	for y := p.Start.Year(); y <= p.End.Year(); y++ {
		if !isLeap(y) {
			continue
		}
		feb29 := time.Date(y, time.February, 29, 0, 0, 0, 0, time.UTC)
		if p.Contains(feb29) {
			return true
		}
	}
	return false
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

// RoundDiv num/den'i kuruşa yuvarlar: yarımı yukarı, negatifte sıfırdan uzağa. den > 0 olmalı.
func RoundDiv(num, den int64) int64 {
	if num >= 0 {
		return (2*num + den) / (2 * den)
	}
	return -((-2*num + den) / (2 * den))
}

// FormatRatio num/den'i 4 ondalıkla (kesilmiş) makine biçiminde yazar: "-199670.0712".
func FormatRatio(num, den int64) string {
	sign := ""
	if num < 0 {
		sign = "-"
		num = -num
	}
	whole, rem := num/den, num%den
	return fmt.Sprintf("%s%d.%04d", sign, whole, rem*10_000/den)
}

// Band kısa dönem tablosu satırı: geçen gün [FromDay, ToDay) → kazanılmış prim oranı (bp).
type Band struct {
	FromDay int  `json:"from_day"`
	ToDay   *int `json:"to_day"` // nil = üst sınırsız
	RateBP  int  `json:"rate_bp"`
}

func (b Band) Contains(day int64) bool {
	return day >= int64(b.FromDay) && (b.ToDay == nil || day < int64(*b.ToDay))
}

func (b Band) String() string {
	if b.ToDay == nil {
		return fmt.Sprintf("[%d, ∞)", b.FromDay)
	}
	return fmt.Sprintf("[%d, %d)", b.FromDay, *b.ToDay)
}

// SampleShortPeriodTable demo tarifelerinin kısa dönem tablosu (KURGUSAL örnek; seed bunu yükler).
func SampleShortPeriodTable() []Band {
	to := func(v int) *int { return &v }
	return []Band{
		{0, to(31), 2_500}, {31, to(91), 4_000}, {91, to(181), 6_000}, {181, to(271), 8_000}, {271, to(367), 10_000},
	}
}

// ShortPeriodRate geçen gün sayısının düştüğü bandı döner.
func ShortPeriodRate(table []Band, elapsed int64) (Band, error) {
	for _, b := range table {
		if b.Contains(elapsed) {
			return b, nil
		}
	}
	return Band{}, fmt.Errorf("%w: %d gün", ErrNoShortPeriodRate, elapsed)
}

type Method string

const (
	MethodProRata     Method = "PRO_RATA"
	MethodShortPeriod Method = "SHORT_PERIOD"
)

// İptal gerekçeleri (policies.cancel_reason CHECK ile aynı).
const (
	ReasonCustomerRequest = "CUSTOMER_REQUEST"
	ReasonNonPayment      = "NON_PAYMENT"
	ReasonUnderwriter     = "UNDERWRITER"
	ReasonOther           = "OTHER"
)

// MethodFor gerekçeye göre iade yöntemi: müşteri talebi kısa dönem, diğerleri (şirket kaynaklı) gün esaslı.
func MethodFor(reason string) (Method, error) {
	switch reason {
	case ReasonCustomerRequest:
		return MethodShortPeriod, nil
	case ReasonNonPayment, ReasonUnderwriter, ReasonOther:
		return MethodProRata, nil
	}
	return "", inputErr("reason", "CUSTOMER_REQUEST, NON_PAYMENT, UNDERWRITER veya OTHER olmalı")
}

// Detail calc_detail'e yazılan adım adım hesap kaydıdır.
type Detail struct {
	Kind            string `json:"kind"` // VEHICLE_CHANGE | CANCELLATION
	Method          Method `json:"method"`
	Reason          string `json:"reason,omitempty"`
	PeriodStart     string `json:"period_start"`
	PeriodEnd       string `json:"period_end"`
	PeriodDays      int64  `json:"period_days"`
	LeapDayInPeriod bool   `json:"leap_day_in_period"`
	EffectiveDate   string `json:"effective_date"`
	ElapsedDays     int64  `json:"elapsed_days"`
	RemainingDays   int64  `json:"remaining_days"`

	OldAnnualKurus  *int64 `json:"old_annual_premium_kurus,omitempty"`
	NewAnnualKurus  *int64 `json:"new_annual_premium_kurus,omitempty"`
	AnnualDiffKurus *int64 `json:"annual_difference_kurus,omitempty"`

	AnnualKurus     *int64 `json:"annual_premium_kurus,omitempty"`
	WrittenKurus    *int64 `json:"written_premium_kurus,omitempty"`
	ShortPeriodBand *Band  `json:"short_period_band,omitempty"`
	EarnedKurus     *int64 `json:"earned_premium_kurus,omitempty"`
	RefundKurus     *int64 `json:"refund_kurus,omitempty"`
	RefundLimited   bool   `json:"refund_limited_to_written,omitempty"`

	// Yuvarlanan oran: Numerator / Denominator → Unrounded (4 ondalık, kesilmiş) → Rounded
	Numerator   int64  `json:"numerator"`
	Denominator int64  `json:"denominator"`
	Unrounded   string `json:"unrounded_kurus"`
	Rounded     int64  `json:"rounded_kurus"`
	Rounding    string `json:"rounding"`

	PremiumDeltaKurus int64    `json:"premium_delta_kurus"` // + ek prim, − iade
	Steps             []string `json:"steps"`
}

func ptr(v int64) *int64 { return &v }

func baseDetail(kind string, p Period, d time.Time) Detail {
	return Detail{
		Kind: kind, PeriodStart: p.Start.Format(dateLayout), PeriodEnd: p.End.Format(dateLayout),
		PeriodDays: p.Days(), LeapDayInPeriod: p.HasLeapDay(), EffectiveDate: d.Format(dateLayout),
		ElapsedDays: p.Elapsed(d), RemainingDays: p.Remaining(d), Rounding: RoundingRule,
	}
}

func (d *Detail) step(format string, a ...any) { d.Steps = append(d.Steps, fmt.Sprintf(format, a...)) }

func (d *Detail) ratio(num, den int64) int64 {
	d.Numerator, d.Denominator = num, den
	d.Unrounded = FormatRatio(num, den)
	d.Rounded = RoundDiv(num, den)
	return d.Rounded
}

func (d *Detail) periodSteps(p Period, label string) {
	leap := "29 Şubat içermiyor"
	if d.LeapDayInPeriod {
		leap = "29 Şubat içeriyor"
	}
	d.step("Dönem: %s = %d gün (%s)", p, d.PeriodDays, leap)
	d.step("%s %s: geçen %d gün, kalan %d gün", label, d.EffectiveDate, d.ElapsedDays, d.RemainingDays)
}

// VehicleChange araç değişikliği zeyli girdisi.
type VehicleChange struct {
	Period         Period
	EffectiveDate  time.Time
	OldAnnualKurus int64
	NewAnnualKurus int64
}

// Calculate gün esaslı prim farkını hesaplar. Zeyil tarihi dönem içinde ve başlangıçtan sonra olmalı.
func (in VehicleChange) Calculate() (Detail, error) {
	if err := in.Period.validate(); err != nil {
		return Detail{}, err
	}
	if !in.EffectiveDate.After(in.Period.Start) || !in.EffectiveDate.Before(in.Period.End) {
		return Detail{}, inputErr("effective_date", "zeyil tarihi %s ile %s arasında (ikisi hariç) olmalı",
			in.Period.Start.Format(dateLayout), in.Period.End.Format(dateLayout))
	}
	d := baseDetail("VEHICLE_CHANGE", in.Period, in.EffectiveDate)
	d.Method = MethodProRata
	diff := in.NewAnnualKurus - in.OldAnnualKurus
	d.OldAnnualKurus, d.NewAnnualKurus, d.AnnualDiffKurus = ptr(in.OldAnnualKurus), ptr(in.NewAnnualKurus), ptr(diff)

	d.periodSteps(in.Period, "Zeyil tarihi")
	d.step("Yıllık prim: eski %s, yeni %s kuruş → fark %s kuruş",
		Kurus(in.OldAnnualKurus), Kurus(in.NewAnnualKurus), SignedKurus(diff))
	delta := d.ratio(diff*d.RemainingDays, d.PeriodDays)
	d.step("Gün esaslı fark: %s × %d / %d = %s kuruş", SignedKurus(diff), d.RemainingDays, d.PeriodDays, Decimal(d.Unrounded))
	d.PremiumDeltaKurus = delta
	switch {
	case delta > 0:
		d.step("Yuvarlama (yarımı yukarı): %s kuruş ek prim", Kurus(delta))
	case delta < 0:
		d.step("Yuvarlama (yarımı yukarı, sıfırdan uzağa): %s kuruş iade", Kurus(-delta))
	default:
		d.step("Prim farkı yok")
	}
	return d, nil
}

// Cancellation iptal girdisi. AnnualKurus iptal anındaki (açık sürüm) yıllık prim;
// WrittenKurus ilk prim + tüm zeyil farkları (yazılmış prim).
type Cancellation struct {
	Period        Period
	EffectiveDate time.Time
	Reason        string
	AnnualKurus   int64
	WrittenKurus  int64
	ShortPeriod   []Band
}

// Calculate iade tutarını hesaplar. İptal tarihi [start, end) içinde olmalı (başlangıç günü dahil).
func (in Cancellation) Calculate() (Detail, error) {
	if err := in.Period.validate(); err != nil {
		return Detail{}, err
	}
	if !in.Period.Contains(in.EffectiveDate) {
		return Detail{}, inputErr("effective_date", "iptal tarihi %s (dahil) ile %s (hariç) arasında olmalı",
			in.Period.Start.Format(dateLayout), in.Period.End.Format(dateLayout))
	}
	method, err := MethodFor(in.Reason)
	if err != nil {
		return Detail{}, err
	}
	d := baseDetail("CANCELLATION", in.Period, in.EffectiveDate)
	d.Method, d.Reason = method, in.Reason
	d.AnnualKurus, d.WrittenKurus = ptr(in.AnnualKurus), ptr(in.WrittenKurus)
	d.periodSteps(in.Period, "İptal tarihi")

	var refund int64
	switch method {
	case MethodShortPeriod:
		band, err := ShortPeriodRate(in.ShortPeriod, d.ElapsedDays)
		if err != nil {
			return Detail{}, err
		}
		d.ShortPeriodBand = &band
		d.step("Yöntem: kısa dönem (müşteri talebi); %d gün ∈ %s → kazanılmış oran %s",
			d.ElapsedDays, band, Percent(band.RateBP))
		earned := d.ratio(in.AnnualKurus*int64(band.RateBP), 10_000)
		d.EarnedKurus = ptr(earned)
		d.step("Kazanılmış prim: %s × %d / 10000 = %s → %s kuruş",
			Kurus(in.AnnualKurus), band.RateBP, Decimal(d.Unrounded), Kurus(earned))
		refund = in.WrittenKurus - earned
		d.step("Yazılmış prim (ilk prim + zeyil farkları): %s kuruş", Kurus(in.WrittenKurus))
		if refund < 0 {
			d.step("İade: %s − %s < 0 → 0 kuruş", Kurus(in.WrittenKurus), Kurus(earned))
			refund = 0
		} else {
			d.step("İade: %s − %s = %s kuruş", Kurus(in.WrittenKurus), Kurus(earned), Kurus(refund))
		}
	case MethodProRata:
		d.step("Yöntem: gün esaslı (şirket kaynaklı iptal: %s)", in.Reason)
		refund = d.ratio(in.AnnualKurus*d.RemainingDays, d.PeriodDays)
		d.step("İade: %s × %d / %d = %s → %s kuruş",
			Kurus(in.AnnualKurus), d.RemainingDays, d.PeriodDays, Decimal(d.Unrounded), Kurus(refund))
		if refund > in.WrittenKurus {
			d.step("İade yazılmış primle sınırlandı: %s kuruş", Kurus(in.WrittenKurus))
			refund, d.RefundLimited = in.WrittenKurus, true
		}
		if refund < 0 {
			refund = 0
		}
	}
	d.RefundKurus = ptr(refund)
	d.PremiumDeltaKurus = -refund
	return d, nil
}

// Kurus 1234567 → "1.234.567" (gösterim; işaretsiz büyüklük için).
func Kurus(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if neg {
		return "−" + b.String()
	}
	return b.String()
}

// SignedKurus artı işaretini de yazar: "+315.496", "−315.496", "0".
func SignedKurus(v int64) string {
	if v > 0 {
		return "+" + Kurus(v)
	}
	return Kurus(v)
}

// Decimal makine biçimli "199670.0712" → "199.670,0712" (Türkçe gösterim).
func Decimal(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	n, _ := strconv.ParseInt(whole, 10, 64)
	out := Kurus(n) + "," + frac
	if neg {
		return "−" + out
	}
	return out
}

// Percent 8000 bp → "%80,00".
func Percent(bp int) string {
	return fmt.Sprintf("%%%d,%02d", bp/100, bp%100)
}
