// Package lossratio yazılan prim bazlı, tam sayı hasar/prim oranını hesaplar.
package lossratio

import (
	"math/big"

	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
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
	q.Add(q, big.NewInt(proration.RoundDiv(r.Int64(), premium)))
	if sign < 0 {
		q.Neg(q)
	}
	if !q.IsInt64() {
		return nil
	}
	value := q.Int64()
	return &value
}
