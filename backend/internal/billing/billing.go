// Package billing DB ve saat bağımlılığı olmayan tam sayı faturalama hesaplarıdır.
package billing

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
)

var ErrOverpayment = errors.New("ödeme açık bakiyeyi aşıyor")

type Installment struct {
	ID     int64
	Seq    int
	Due    time.Time
	Amount int64
	Paid   int64
	Status string
}

type Charge struct {
	ID            int64
	InstallmentID int64
	Amount        int64
	Paid          int64
}

type Allocation struct {
	InstallmentID int64
	ChargeID      int64
	Amount        int64
}

type Adjustment struct {
	Rows       []Installment
	CashRefund int64
	Total      int64
}

func Open(r Installment) bool {
	return r.Status == "PENDING" || r.Status == "PARTIAL"
}

func Split(total int64, n int, start time.Time) ([]Installment, error) {
	if n < 1 || n > 12 || total < int64(n) {
		return nil, fmt.Errorf("taksit sayısı 1–12 ve toplam kuruştan küçük/eşit olmalı")
	}
	rows := make([]Installment, n)
	for i := range rows {
		rows[i] = Installment{Seq: i + 1, Due: caldate.AddMonths(start, i), Amount: total / int64(n), Status: "PENDING"}
	}
	rows[0].Amount += total % int64(n)
	return rows, nil
}

func InterestAmount(principal, rate, days int64) (int64, error) {
	if principal < 0 || rate <= 0 || days < 0 {
		return 0, fmt.Errorf("geçersiz faiz girdisi")
	}
	// Ara çarpımı taşırmadan aynı yarımı yukarı kuralı kullanılır.
	numerator := new(big.Int).Mul(big.NewInt(principal), big.NewInt(rate))
	numerator.Mul(numerator, big.NewInt(days))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, big.NewInt(1_000_000), remainder)
	rounded := proration.RoundDiv(remainder.Int64(), 1_000_000)
	quotient.Add(quotient, big.NewInt(rounded))
	if !quotient.IsInt64() {
		return 0, fmt.Errorf("faiz tutarı int64 sınırını aşıyor")
	}
	return quotient.Int64(), nil
}

func ordered(rows []Installment) []Installment {
	out := append([]Installment(nil), rows...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Due.Equal(out[j].Due) {
			return out[i].Seq < out[j].Seq
		}
		return out[i].Due.Before(out[j].Due)
	})
	return out
}

func Allocate(rows []Installment, charges []Charge, amount int64) ([]Allocation, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("ödeme pozitif olmalı")
	}
	rows = ordered(rows)
	charges = append([]Charge(nil), charges...)
	sort.Slice(charges, func(i, j int) bool { return charges[i].ID < charges[j].ID })
	remaining := amount
	out := []Allocation{}
	for _, row := range rows {
		// Faiz, anapara kapalı olsa da borç olarak kalabilir.
		for _, c := range charges {
			if c.InstallmentID != row.ID {
				continue
			}
			part := min(remaining, c.Amount-c.Paid)
			if part > 0 {
				out = append(out, Allocation{InstallmentID: row.ID, ChargeID: c.ID, Amount: part})
				remaining -= part
			}
		}
		if Open(row) {
			part := min(remaining, row.Amount-row.Paid)
			if part > 0 {
				out = append(out, Allocation{InstallmentID: row.ID, Amount: part})
				remaining -= part
			}
		}
	}
	if remaining > 0 {
		return nil, ErrOverpayment
	}
	return out, nil
}

func Adjust(rows []Installment, delta int64, effective time.Time) (Adjustment, error) {
	result := Adjustment{Rows: append([]Installment(nil), rows...)}
	if delta == math.MinInt64 {
		return result, fmt.Errorf("prim farkı int64 sınırını aşıyor")
	}
	if delta > 0 {
		indices := []int{}
		maxSeq := 0
		for i, row := range result.Rows {
			maxSeq = max(maxSeq, row.Seq)
			if Open(row) && !row.Due.Before(effective) {
				indices = append(indices, i)
			}
		}
		if len(indices) == 0 {
			result.Rows = append(result.Rows, Installment{Seq: maxSeq + 1, Due: effective, Amount: delta, Status: "PENDING"})
		} else {
			sort.Slice(indices, func(i, j int) bool {
				a, b := result.Rows[indices[i]], result.Rows[indices[j]]
				if a.Due.Equal(b.Due) {
					return a.Seq < b.Seq
				}
				return a.Due.Before(b.Due)
			})
			for n, i := range indices {
				part := delta / int64(len(indices))
				if n == 0 {
					part += delta % int64(len(indices))
				}
				if part > math.MaxInt64-result.Rows[i].Amount {
					return result, fmt.Errorf("taksit tutarı taşması")
				}
				result.Rows[i].Amount += part
			}
		}
	} else if delta < 0 {
		remaining := -delta
		descending := ordered(result.Rows)
		for j := len(descending) - 1; j >= 0 && remaining > 0; j-- {
			row := descending[j]
			if !Open(row) {
				continue
			}
			for i := range result.Rows {
				if result.Rows[i].Seq != row.Seq {
					continue
				}
				take := min(row.Amount-row.Paid, remaining)
				remaining -= take
				if row.Amount-take == row.Paid && row.Paid == 0 {
					result.Rows[i].Status = "CANCELLED" // amount>0 şema kısıtı korunur.
				} else {
					result.Rows[i].Amount -= take
					if result.Rows[i].Amount == row.Paid {
						result.Rows[i].Status = "PAID"
					}
				}
			}
		}
		result.CashRefund = remaining
	}
	for _, row := range result.Rows {
		if row.Status != "CANCELLED" {
			if row.Amount > math.MaxInt64-result.Total {
				return result, fmt.Errorf("plan toplamı taşması")
			}
			result.Total += row.Amount
		}
	}
	return result, nil
}

func Overdue30(rows []Installment, asOf time.Time) bool {
	for _, row := range rows {
		if Open(row) && row.Amount > row.Paid && proration.DaysBetween(row.Due, asOf) >= 30 {
			return true
		}
	}
	return false
}
