package lossratio

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestGoldenLossRatios(t *testing.T) {
	// 1/3 × 10000=3333.33→3333; 2/3 ×10000=6666.66→6667.
	for _, c := range []struct{ claims, premium, want int64 }{{1, 3, 3333}, {2, 3, 6667}, {0, 100, 0}, {1, 32, 313}, {-1, 32, -313}, {math.MaxInt64, math.MaxInt64, 10000}} {
		got := LossRatioBP(c.claims, c.premium)
		require.NotNil(t, got)
		assert.Equal(t, c.want, *got)
	}
	assert.Nil(t, LossRatioBP(1, 0))
	assert.Nil(t, LossRatioBP(1, -1))
	assert.Nil(t, LossRatioBP(math.MaxInt64, 1))
}
