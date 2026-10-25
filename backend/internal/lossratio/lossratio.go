// Package lossratio yazılan prim bazlı, tam sayı hasar/prim oranını hesaplar.
package lossratio

import (
	"math/big"
)

// LossRatioBP prim <=0 veya int64'e sığmayan oranda nil döner; yarımı sıfırdan uzağa yuvarlar.
func LossRatioBP(claims, premium int64) *int64 {
	if premium <= 0 {
		return nil
	}
	numerator := new(big.Int).Mul(big.NewInt(claims), big.NewInt(10000))
	sign := numerator.Sign()
	numerator.Abs(numerator)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(numerator, big.NewInt(premium), r)
	// RoundDiv yerine güvenli kalan karşılaştırması: MaxInt64 paydada ara toplam taşabilir.
	rounded := int64(0)
	if r.Int64() >= premium-r.Int64() {
		rounded = 1
	}
	q.Add(q, big.NewInt(rounded))
	if sign < 0 {
		q.Neg(q)
	}
	if !q.IsInt64() {
		return nil
	}
	value := q.Int64()
	return &value
}
