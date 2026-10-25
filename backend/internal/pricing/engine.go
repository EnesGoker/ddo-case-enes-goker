// Package pricing saf prim hesaplama motorudur (T-1): veritabanı, saat ve rastgelelik kullanmaz.
//
// Hesap sırası (sabit): her teminat için taban prim → AGE → NCD → REGION → VEHICLE_GROUP.
// Tarifede tanımlı olmayan faktör tipi o üründe uygulanmaz (ör. KONUT'ta yaş/araç grubu yok).
// Her çarpan adımında sonuç kuruşa yarımı yukarı yuvarlanır (tam sayı, bp = 1/10.000) ve
// adımın farkı bir döküm satırı olur. Toplam = döküm satırlarının toplamı (her zaman eşit).
// Vergi/harç uygulanmaz (varsayım).
package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type FactorType string

const (
	FactorAge          FactorType = "AGE"
	FactorNCD          FactorType = "NCD"
	FactorRegion       FactorType = "REGION"
	FactorVehicleGroup FactorType = "VEHICLE_GROUP"
)

// FactorOrder çarpanların uygulanma sırasıdır.
var FactorOrder = []FactorType{FactorAge, FactorNCD, FactorRegion, FactorVehicleGroup}

// RegionFallbackKey tarifede tanımlı olmayan bölge kodları için kullanılan anahtar.
const RegionFallbackKey = "DIGER"

const bpScale = 10_000

// maxAmount taşma koruması: 10 milyar TL; bp çarpımı int64'e sığar.
const maxAmount = 1_000_000_000_000

type Range struct {
	Lo int  `json:"lo"`
	Hi *int `json:"hi"` // nil = üst sınırsız; aralık [Lo, Hi)
}

func (r Range) Contains(v int) bool { return v >= r.Lo && (r.Hi == nil || v < *r.Hi) }

func (r Range) String() string {
	if r.Hi == nil {
		return fmt.Sprintf("%d+", r.Lo)
	}
	return fmt.Sprintf("%d–%d", r.Lo, *r.Hi-1)
}

type Factor struct {
	Type         FactorType `json:"type"`
	Key          string     `json:"key,omitempty"`
	Range        *Range     `json:"range,omitempty"`
	MultiplierBP int64      `json:"multiplier_bp"`
}

type Tariff struct {
	ID        int64            `json:"id"`
	Product   Product          `json:"product"`
	VersionNo int              `json:"version_no"`
	ValidFrom time.Time        `json:"valid_from"`
	ValidTo   *time.Time       `json:"valid_to"`
	BaseRates map[string]int64 `json:"base_rates"`
	Factors   []Factor         `json:"factors"`
}

// ValidOn tarifenin d gününde geçerli olup olmadığı: valid_from <= d < valid_to.
func (t Tariff) ValidOn(d time.Time) bool {
	return !d.Before(t.ValidFrom) && (t.ValidTo == nil || d.Before(*t.ValidTo))
}

var ErrNoTariff = errors.New("bu ürün ve tarih için geçerli tarife yok")

// SelectTariff ACTIVE adaylar içinden on gününde geçerli olanı seçer. Seçim, istek anına göre değil
// poliçe başlangıç tarihine göredir.
func SelectTariff(candidates []Tariff, p Product, on time.Time) (Tariff, error) {
	var found []Tariff
	for _, t := range candidates {
		if t.Product == p && t.ValidOn(on) {
			found = append(found, t)
		}
	}
	switch len(found) {
	case 0:
		return Tariff{}, ErrNoTariff
	case 1:
		return found[0], nil
	default:
		return Tariff{}, fmt.Errorf("%s için %s tarihinde birden çok geçerli tarife", p, on.Format("2006-01-02"))
	}
}

type Risk struct {
	DriverAge    *int   `json:"driver_age,omitempty"`
	NCDLevel     *int   `json:"ncd_level,omitempty"`
	RegionCode   string `json:"region_code,omitempty"`
	VehicleGroup string `json:"vehicle_group,omitempty"`
}

type Input struct {
	Product   Product
	Risk      Risk
	Coverages []string
}

// InputError kullanıcı girdisi hatasıdır (HTTP 400'e çevrilir).
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return e.Field + ": " + e.Message }

func inputErr(field, format string, a ...any) error {
	return &InputError{Field: field, Message: fmt.Sprintf(format, a...)}
}

type LineType string

const (
	LineBase     LineType = "BASE"
	LineFactor   LineType = "FACTOR"
	LineRounding LineType = "ROUNDING"
)

type Line struct {
	Seq          int        `json:"seq"`
	CoverageCode string     `json:"coverage_code"`
	Type         LineType   `json:"line_type"`
	FactorType   FactorType `json:"factor_type,omitempty"`
	Label        string     `json:"factor_label"`
	MultiplierBP int64      `json:"multiplier_bp,omitempty"`
	AmountKurus  int64      `json:"amount_kurus"`
}

type CoveragePremium struct {
	Code         string `json:"code"`
	PremiumKurus int64  `json:"premium_kurus"`
}

type Result struct {
	TariffID   int64             `json:"tariff_version_id"`
	Lines      []Line            `json:"lines"`
	Coverages  []CoveragePremium `json:"coverages"`
	TotalKurus int64             `json:"total_premium_kurus"`
	InputHash  string            `json:"input_hash"`
	// Applied tarifede uygulanan faktör tipleri ve kullanılan girdi (normalize edilmiş)
	Risk Risk `json:"risk"`
}

// MulBP amount × bp / 10.000, yarımı yukarı yuvarlanmış (amount >= 0).
func MulBP(amount, bp int64) int64 {
	return (amount*bp + bpScale/2) / bpScale
}

type appliedFactor struct {
	factor Factor
	label  string
}

// Calculate prim dökümünü üretir. Aynı tarife + aynı girdi her zaman aynı sonucu verir.
func Calculate(t Tariff, in Input) (Result, error) {
	if in.Product != t.Product {
		return Result{}, inputErr("product", "tarife %s ürününe ait, istenen %s", t.Product, in.Product)
	}
	coverages, err := NormalizeCoverages(in.Product, in.Coverages)
	if err != nil {
		return Result{}, err
	}
	risk := normalizeRisk(in.Risk)

	var applied []appliedFactor
	usedRisk := Risk{}
	for _, ft := range FactorOrder {
		if !hasFactorType(t, ft) {
			continue
		}
		f, label, err := resolveFactor(t, ft, risk)
		if err != nil {
			return Result{}, err
		}
		applied = append(applied, appliedFactor{f, label})
		switch ft {
		case FactorAge:
			usedRisk.DriverAge = risk.DriverAge
		case FactorNCD:
			usedRisk.NCDLevel = risk.NCDLevel
		case FactorRegion:
			usedRisk.RegionCode = risk.RegionCode
		case FactorVehicleGroup:
			usedRisk.VehicleGroup = risk.VehicleGroup
		}
	}

	res := Result{TariffID: t.ID, Risk: usedRisk}
	seq := 0
	add := func(l Line) {
		seq++
		l.Seq = seq
		res.Lines = append(res.Lines, l)
	}
	for _, code := range coverages {
		base, ok := t.BaseRates[code]
		if !ok {
			return Result{}, inputErr("coverages", "%s teminatı bu tarife sürümünde tanımlı değil", code)
		}
		if base < 0 || base > maxAmount {
			return Result{}, fmt.Errorf("tarife %d: %s taban primi geçersiz", t.ID, code)
		}
		spec, _ := CoverageSpecFor(in.Product, code)
		running := base
		add(Line{CoverageCode: code, Type: LineBase, Label: "Taban prim — " + spec.Name, AmountKurus: base})
		for _, a := range applied {
			next := MulBP(running, a.factor.MultiplierBP)
			add(Line{
				CoverageCode: code,
				Type:         LineFactor,
				FactorType:   a.factor.Type,
				Label:        a.label,
				MultiplierBP: a.factor.MultiplierBP,
				AmountKurus:  next - running,
			})
			running = next
		}
		res.Coverages = append(res.Coverages, CoveragePremium{Code: code, PremiumKurus: running})
		res.TotalKurus += running
	}

	// Değişmez: döküm toplamı = nihai prim. Adım adım farklar kullanıldığı için fark oluşmaz;
	// yine de oluşursa sessizce yutulmaz, açık bir ROUNDING satırı yazılır.
	var sum int64
	for _, l := range res.Lines {
		sum += l.AmountKurus
	}
	if diff := res.TotalKurus - sum; diff != 0 {
		add(Line{Type: LineRounding, Label: "Yuvarlama farkı", AmountKurus: diff})
	}

	res.InputHash = inputHash(t.ID, in.Product, usedRisk, coverages)
	return res, nil
}

// NormalizeCoverages teminatları doğrular ve katalog sırasına dizer (zorunlular dahil olmalı).
func NormalizeCoverages(p Product, codes []string) ([]string, error) {
	spec, ok := Catalog[p]
	if !ok {
		return nil, inputErr("product", "bilinmeyen ürün %q", p)
	}
	if len(codes) == 0 {
		return nil, inputErr("coverages", "en az bir teminat seçilmeli")
	}
	seen := map[string]bool{}
	for _, c := range codes {
		c = strings.ToUpper(strings.TrimSpace(c))
		if _, ok := CoverageSpecFor(p, c); !ok {
			return nil, inputErr("coverages", "%s ürününde %q teminatı yok", p, c)
		}
		if seen[c] {
			return nil, inputErr("coverages", "%q teminatı birden çok kez seçilmiş", c)
		}
		seen[c] = true
	}
	var out []string
	for _, c := range spec.Coverages {
		if c.Required && !seen[c.Code] {
			return nil, inputErr("coverages", "%s zorunlu teminattır", c.Code)
		}
		if seen[c.Code] {
			out = append(out, c.Code)
		}
	}
	return out, nil
}

func normalizeRisk(r Risk) Risk {
	r.RegionCode = strings.ToUpper(strings.TrimSpace(r.RegionCode))
	r.VehicleGroup = strings.ToUpper(strings.TrimSpace(r.VehicleGroup))
	return r
}

func hasFactorType(t Tariff, ft FactorType) bool {
	for _, f := range t.Factors {
		if f.Type == ft {
			return true
		}
	}
	return false
}

func resolveFactor(t Tariff, ft FactorType, r Risk) (Factor, string, error) {
	find := func(key string) (Factor, bool) {
		for _, f := range t.Factors {
			if f.Type == ft && f.Key == key {
				return f, true
			}
		}
		return Factor{}, false
	}
	switch ft {
	case FactorAge:
		if r.DriverAge == nil {
			return Factor{}, "", inputErr("driver_age", "sürücü yaşı gerekli")
		}
		for _, f := range t.Factors {
			if f.Type == ft && f.Range != nil && f.Range.Contains(*r.DriverAge) {
				return f, fmt.Sprintf("Sürücü yaşı %d (%s)", *r.DriverAge, f.Range), nil
			}
		}
		return Factor{}, "", inputErr("driver_age", "sürücü yaşı %d tarife aralıklarının dışında", *r.DriverAge)
	case FactorNCD:
		if r.NCDLevel == nil {
			return Factor{}, "", inputErr("ncd_level", "hasarsızlık kademesi gerekli")
		}
		if f, ok := find(strconv.Itoa(*r.NCDLevel)); ok {
			return f, fmt.Sprintf("Hasarsızlık kademesi %d", *r.NCDLevel), nil
		}
		return Factor{}, "", inputErr("ncd_level", "hasarsızlık kademesi %d tarifede yok", *r.NCDLevel)
	case FactorRegion:
		if r.RegionCode == "" {
			return Factor{}, "", inputErr("region_code", "bölge kodu gerekli")
		}
		if f, ok := find(r.RegionCode); ok {
			return f, "Bölge " + r.RegionCode, nil
		}
		if f, ok := find(RegionFallbackKey); ok {
			return f, "Bölge " + r.RegionCode + " (diğer iller)", nil
		}
		return Factor{}, "", inputErr("region_code", "bölge %s tarifede yok", r.RegionCode)
	case FactorVehicleGroup:
		if r.VehicleGroup == "" {
			return Factor{}, "", inputErr("vehicle_group", "araç grubu gerekli")
		}
		if f, ok := find(r.VehicleGroup); ok {
			return f, "Araç grubu " + r.VehicleGroup, nil
		}
		return Factor{}, "", inputErr("vehicle_group", "araç grubu %s tarifede yok", r.VehicleGroup)
	}
	return Factor{}, "", fmt.Errorf("bilinmeyen faktör tipi %s", ft)
}

// inputHash fiyatı etkileyen normalize girdinin (yalnızca uygulanan faktörler) ve tarife
// sürümünün SHA-256 özetidir. Aynı fiyat girdisi → aynı hash.
func inputHash(tariffID int64, p Product, r Risk, coverages []string) string {
	canonical := struct {
		V         int      `json:"v"`
		TariffID  int64    `json:"tariff_version_id"`
		Product   Product  `json:"product"`
		Risk      Risk     `json:"risk"`
		Coverages []string `json:"coverages"`
	}{1, tariffID, p, r, coverages}
	b, _ := json.Marshal(canonical)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Review nedenleri: biri varsa teklif UW_REVIEW olur.
const (
	ReasonYoungDriver = "YOUNG_DRIVER" // sürücü yaşı < 21
	ReasonHighPremium = "HIGH_PREMIUM" // toplam prim eşiğin üstünde
)

const YoungDriverAge = 21

// ReviewReasons underwriter onayı gerektiren durumları döner (kural kurgusaldır).
func ReviewReasons(res Result, thresholdKurus int64) []string {
	var out []string
	if res.Risk.DriverAge != nil && *res.Risk.DriverAge < YoungDriverAge {
		out = append(out, ReasonYoungDriver)
	}
	if thresholdKurus > 0 && res.TotalKurus > thresholdKurus {
		out = append(out, ReasonHighPremium)
	}
	return out
}
