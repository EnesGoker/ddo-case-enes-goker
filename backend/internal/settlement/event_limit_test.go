package settlement

import (
	"math"
	"testing"

	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoldenEventLimitsInOrder(t *testing.T) {
	annual, event := int64(600_000), int64(800_000)
	in := Input{GrossDamageKurus: 1_000_000, DeductibleType: pricing.DeductibleFixed, DeductibleValue: 100_000,
		CoverageRemainingKurus: 700_000, AnnualRemainingKurus: &annual, EventLimitKurus: &event}
	r, err := Calculate(in)
	require.NoError(t, err)
	assert.Equal(t, int64(600_000), r.PayableKurus)
	codes := []string{}
	for _, s := range r.CalcSteps {
		codes = append(codes, s.Code)
	}
	assert.Equal(t, []string{"GROSS_DAMAGE", "DEDUCTIBLE", "AFTER_DEDUCTIBLE", "EVENT_LIMIT", "COVERAGE_LIMIT", "ANNUAL_LIMIT", "PAYABLE"}, codes)
	for _, index := range []int{3, 4, 5} {
		assert.Equal(t, int64(100_000), r.CalcSteps[index].ClippedKurus)
	}
	event = 400_000
	r, err = Calculate(in)
	require.NoError(t, err)
	assert.Equal(t, event, r.PayableKurus)
	assert.Equal(t, int64(500_000), r.CalcSteps[3].ClippedKurus)
	assert.Zero(t, r.CalcSteps[4].ClippedKurus)
	assert.Zero(t, r.CalcSteps[5].ClippedKurus)
	in.EventLimitKurus = nil
	r, err = Calculate(in)
	require.NoError(t, err)
	assert.Equal(t, annual, r.PayableKurus)
	assert.True(t, r.CalcSteps[3].Skipped)
	event = 900_000
	in.EventLimitKurus = &event
	r, err = Calculate(in)
	require.NoError(t, err)
	assert.Zero(t, r.CalcSteps[3].ClippedKurus, "eşit sınır kırpılmaz")
	assert.False(t, r.CalcSteps[3].Skipped)
}

func TestEventLimitInt64AndValidation(t *testing.T) {
	event := int64(math.MaxInt64 - 1)
	in := Input{GrossDamageKurus: math.MaxInt64, DeductibleType: pricing.DeductibleNone,
		CoverageRemainingKurus: math.MaxInt64, EventLimitKurus: &event}
	r, err := Calculate(in)
	require.NoError(t, err)
	assert.Equal(t, event, r.PayableKurus)
	assert.Equal(t, int64(1), r.CalcSteps[3].ClippedKurus)
	for _, invalid := range []int64{0, -1} {
		in.EventLimitKurus = &invalid
		_, err := Calculate(in)
		assert.Error(t, err)
	}
}
