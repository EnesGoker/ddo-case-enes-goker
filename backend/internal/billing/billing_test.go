package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestGoldenSplit(t *testing.T) {
	// 100/3 = 33 kalan 1: ilk taksit 34, diğerleri 33.
	rows, err := Split(100, 3, date("2028-01-31"))
	require.NoError(t, err)
	assert.Equal(t, []int64{34, 33, 33}, []int64{rows[0].Amount, rows[1].Amount, rows[2].Amount})
	assert.Equal(t, date("2028-02-29"), rows[1].Due)
	assert.Equal(t, date("2028-03-31"), rows[2].Due)
	for _, c := range []struct {
		total int64
		n     int
	}{{0, 1}, {1, 2}, {100, 0}, {100, 13}} {
		_, err := Split(c.total, c.n, date("2026-01-31"))
		assert.Error(t, err)
	}
}

func TestGoldenInterest(t *testing.T) {
	// 101 kuruş × 1000 ppm × 5 gün / 1.000.000 = 0,505 → 1 kuruş.
	amount, err := InterestAmount(101, 1000, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(1), amount)
	amount, err = InterestAmount(100000, 1000, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), amount)
	amount, err = InterestAmount(100000, 1000, 0)
	require.NoError(t, err)
	assert.Zero(t, amount)
	_, err = InterestAmount(-1, 1000, 1)
	assert.Error(t, err)
}

func TestGoldenAllocation(t *testing.T) {
	// İlk taksit: faiz 10 + anapara 70; ikinci: faiz 5 + anapara 15 = toplam 100.
	rows := []Installment{{ID: 1, Seq: 1, Due: date("2026-01-01"), Amount: 100, Paid: 30, Status: "PARTIAL"}, {ID: 2, Seq: 2, Due: date("2026-02-01"), Amount: 100, Status: "PENDING"}}
	charges := []Charge{{ID: 1, InstallmentID: 1, Amount: 10}, {ID: 2, InstallmentID: 2, Amount: 5}}
	allocations, err := Allocate(rows, charges, 100)
	require.NoError(t, err)
	assert.Equal(t, []Allocation{{InstallmentID: 1, ChargeID: 1, Amount: 10}, {InstallmentID: 1, Amount: 70}, {InstallmentID: 2, ChargeID: 2, Amount: 5}, {InstallmentID: 2, Amount: 15}}, allocations)
	_, err = Allocate(rows, charges, 186)
	assert.ErrorIs(t, err, ErrOverpayment)
}

func TestGoldenAdjustments(t *testing.T) {
	rows := []Installment{{ID: 1, Seq: 1, Due: date("2026-01-01"), Amount: 100, Paid: 80, Status: "PARTIAL"}, {ID: 2, Seq: 2, Due: date("2026-02-01"), Amount: 100, Status: "PENDING"}}
	// -150: en son taksit 100 iptal; ilk taksitin açık 20'si azalır; nakit iade 30.
	r, err := Adjust(rows, -150, date("2026-01-15"))
	require.NoError(t, err)
	assert.Equal(t, int64(30), r.CashRefund)
	assert.Equal(t, "CANCELLED", r.Rows[1].Status)
	assert.Equal(t, int64(100), r.Rows[1].Amount)
	assert.Equal(t, "PAID", r.Rows[0].Status)
	assert.Equal(t, int64(80), r.Total)
	// +5: iki uygun taksite 3 + 2.
	r, err = Adjust(rows, 5, date("2026-01-01"))
	require.NoError(t, err)
	assert.Equal(t, int64(103), r.Rows[0].Amount)
	assert.Equal(t, int64(102), r.Rows[1].Amount)
	r, err = Adjust(rows, 10, date("2026-03-01"))
	require.NoError(t, err)
	assert.Len(t, r.Rows, 3)
	assert.Equal(t, 3, r.Rows[2].Seq)
	assert.Equal(t, date("2026-03-01"), r.Rows[2].Due)
	assert.Equal(t, int64(100), rows[0].Amount, "girdi değişmez")
}

func TestThirtyDayBoundary(t *testing.T) {
	row := Installment{Due: date("2026-01-01"), Amount: 100, Status: "PENDING"}
	assert.False(t, Overdue30([]Installment{row}, date("2026-01-30")))
	assert.True(t, Overdue30([]Installment{row}, date("2026-01-31")))
	row.Status = "CANCELLED"
	assert.False(t, Overdue30([]Installment{row}, date("2026-03-01")))
}
