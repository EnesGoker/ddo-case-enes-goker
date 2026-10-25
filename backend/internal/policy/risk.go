package policy

import (
	"regexp"
	"strings"

	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

// RiskData quotes.risk_data ve policy_versions.risk_data içeriğidir: fiyatlamada kullanılan
// (normalize) risk girdisi + plaka + doğum tarihi (+ teklifte onay nedenleri).
type RiskData struct {
	pricing.Risk
	Plate         string   `json:"plate,omitempty"`
	BirthDate     string   `json:"birth_date"`
	ReviewReasons []string `json:"review_reasons,omitempty"`
}

// Türk plakası: il kodu (01-81) + 1-3 harf + 2-4 rakam, boşluksuz büyük harf.
var plateRe = regexp.MustCompile(`^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,4}$`)

// NormalizePlate boşlukları siler, büyük harfe çevirir ve biçimi doğrular.
func NormalizePlate(raw string) (string, bool) {
	p := strings.ToUpper(strings.ReplaceAll(raw, " ", ""))
	return p, plateRe.MatchString(p)
}
