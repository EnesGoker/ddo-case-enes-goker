// Package settlement saf, deterministik tazminat hesabıdır. Para int64 kuruştur.
package settlement

import (
	"fmt"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

type Input struct {
	GrossDamageKurus       int64
	DeductibleType         pricing.DeductibleType
	DeductibleValue        int64
	CoverageRemainingKurus int64
	AnnualRemainingKurus   *int64
}

type Step struct {
	Code         string `json:"code"`
	InputKurus   int64  `json:"input_kurus"`
	OutputKurus  int64  `json:"output_kurus"`
	ClippedKurus int64  `json:"clipped_kurus"`
	Description  string `json:"description"`
	Skipped      bool   `json:"skipped"`
}

type Result struct {
	GrossDamageKurus int64  `json:"gross_damage_kurus"`
	DeductibleKurus  int64  `json:"deductible_kurus"`
	PayableKurus     int64  `json:"payable_kurus"`
	CalcSteps        []Step `json:"calc_steps"`
}

func Calculate(in Input) (Result, error) {
	r := Result{GrossDamageKurus: in.GrossDamageKurus}
	if in.GrossDamageKurus < 0 || in.DeductibleValue < 0 || in.CoverageRemainingKurus < 0 || (in.AnnualRemainingKurus != nil && *in.AnnualRemainingKurus < 0) {
		return r, fmt.Errorf("tutarlar negatif olamaz")
	}
	switch in.DeductibleType {
	case pricing.DeductibleNone:
		if in.DeductibleValue != 0 {
			return r, fmt.Errorf("NONE muafiyet değeri sıfır olmalı")
		}
	case pricing.DeductibleFixed:
		r.DeductibleKurus = min(in.DeductibleValue, in.GrossDamageKurus)
	case pricing.DeductiblePercentBP:
		if in.DeductibleValue > 10000 {
			return r, fmt.Errorf("muafiyet 10000 bp'yi aşamaz")
		}
		// Bölüm ve kalan ayrı çarpılır; MaxInt64 tutarında bile ara çarpım taşmaz.
		r.DeductibleKurus = (in.GrossDamageKurus/10000)*in.DeductibleValue + pricing.MulBP(in.GrossDamageKurus%10000, in.DeductibleValue)
	default:
		return r, fmt.Errorf("geçersiz muafiyet tipi")
	}
	net := in.GrossDamageKurus - r.DeductibleKurus
	capped := min(net, in.CoverageRemainingKurus)
	r.PayableKurus = capped
	annual := Step{Code: "ANNUAL_LIMIT", InputKurus: capped, OutputKurus: capped, Description: "Yıllık toplam limit yok; adım atlandı", Skipped: true}
	if in.AnnualRemainingKurus != nil {
		r.PayableKurus = min(capped, *in.AnnualRemainingKurus)
		annual = Step{Code: "ANNUAL_LIMIT", InputKurus: capped, OutputKurus: r.PayableKurus, ClippedKurus: capped - r.PayableKurus, Description: fmt.Sprintf("Yıllık kalan %d; %d kuruş kırpıldı", *in.AnnualRemainingKurus, capped-r.PayableKurus)}
	}
	r.CalcSteps = []Step{
		{Code: "GROSS_DAMAGE", InputKurus: in.GrossDamageKurus, OutputKurus: in.GrossDamageKurus, Description: "Son ekspertizin brüt hasarı"},
		{Code: "DEDUCTIBLE", InputKurus: in.GrossDamageKurus, OutputKurus: r.DeductibleKurus, Description: fmt.Sprintf("%s muafiyet (%d): %d kuruş; yarımı yukarı", in.DeductibleType, in.DeductibleValue, r.DeductibleKurus)},
		{Code: "AFTER_DEDUCTIBLE", InputKurus: in.GrossDamageKurus, OutputKurus: net, ClippedKurus: r.DeductibleKurus, Description: "Brüt hasardan muafiyet çıkarıldı"},
		{Code: "COVERAGE_LIMIT", InputKurus: net, OutputKurus: capped, ClippedKurus: net - capped, Description: fmt.Sprintf("Teminat kalan %d; %d kuruş kırpıldı", in.CoverageRemainingKurus, net-capped)},
		annual,
		{Code: "PAYABLE", InputKurus: r.PayableKurus, OutputKurus: r.PayableKurus, Description: "Ödenecek tazminat"},
	}
	return r, nil
}
