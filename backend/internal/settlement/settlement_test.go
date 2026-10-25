package settlement

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

func TestGoldenSettlements(t *testing.T) {
	annual := int64(60_000)
	cases := []struct {
		name                string
		in                  Input
		deductible, payable int64
	}{
		// 100.000 - 5.000 = 95.000; limitler kırpmaz.
		{"sabit", Input{100_000, pricing.DeductibleFixed, 5_000, 200_000, nil}, 5_000, 95_000},
		// 101 × 5000 / 10000 = 50,5 → 51; 101 - 51 = 50.
		{"yüzde-yarım", Input{101, pricing.DeductiblePercentBP, 5_000, 200_000, nil}, 51, 50},
		// 100.000 - 5.000 = 95.000; teminat kalan 70.000 → 25.000 kırpılır.
		{"teminat", Input{100_000, pricing.DeductibleFixed, 5_000, 70_000, nil}, 5_000, 70_000},
		// 100.000 - 5.000 = 95.000 → teminat 70.000 → yıllık 60.000.
		{"yıllık", Input{100_000, pricing.DeductibleFixed, 5_000, 70_000, &annual}, 5_000, 60_000},
		// min(200,100)=100 muafiyet; ödeme 0.
		{"muafiyet-brütten-büyük", Input{100, pricing.DeductibleFixed, 200, 1000, nil}, 100, 0},
		{"muafiyetsiz", Input{100, pricing.DeductibleNone, 0, 1000, nil}, 0, 100},
		// Büyük tutarda çarpım taşması olmadan %100.
		{"int64-sınırı", Input{math.MaxInt64, pricing.DeductiblePercentBP, 10_000, math.MaxInt64, nil}, math.MaxInt64, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := Calculate(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.deductible, r.DeductibleKurus)
			assert.Equal(t, c.payable, r.PayableKurus)
			require.Len(t, r.CalcSteps, 6)
			assert.Equal(t, c.in.AnnualRemainingKurus == nil, r.CalcSteps[4].Skipped)
			for _, step := range r.CalcSteps {
				assert.NotEmpty(t, step.Description)
				assert.GreaterOrEqual(t, step.ClippedKurus, int64(0))
			}
			if c.name == "yıllık" {
				assert.Equal(t, int64(25_000), r.CalcSteps[3].ClippedKurus)
				assert.Equal(t, int64(10_000), r.CalcSteps[4].ClippedKurus)
			}
		})
	}
}

func TestInvalidSettlementInput(t *testing.T) {
	negative := int64(-1)
	for _, in := range []Input{
		{-1, pricing.DeductibleNone, 0, 100, nil}, {1, pricing.DeductibleFixed, -1, 100, nil},
		{1, pricing.DeductibleNone, 1, 100, nil}, {1, pricing.DeductiblePercentBP, 10_001, 100, nil},
		{1, "BAD", 0, 100, nil}, {1, pricing.DeductibleNone, 0, -1, nil}, {1, pricing.DeductibleNone, 0, 100, &negative},
	} {
		_, err := Calculate(in)
		require.Error(t, err)
	}
}
