package fraud

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGoldenSignalsAndThresholds(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, days := range []int{0, 14, 15} {
		out, err := Evaluate(Input{PolicyStart: start, Loss: start.AddDate(0, 0, days)}, DefaultConfig())
		require.NoError(t, err)
		expected := 0
		if days < 15 {
			expected = 40
		}
		assert.Equal(t, expected, out.Score)
		assert.False(t, out.Flagged)
	}
	for _, tc := range []struct {
		days, plate, workshop, score int
		flag                         bool
	}{{0, 2, 0, 75, true}, {15, 1, 3, 0, false}, {15, 2, 4, 65, true}, {0, 0, 4, 70, true}, {15, 2, 0, 35, false}, {15, 0, 4, 30, false}} {
		// 40+35=75; 35+30=65; tek 40/35/30 puan eşik 60'ın altında.
		out, err := Evaluate(Input{PolicyStart: start, Loss: start.AddDate(0, 0, tc.days), HasPlate: true, HasWorkshop: true, PlateClaims: make([]string, tc.plate), WorkshopClaims: make([]string, tc.workshop)}, DefaultConfig())
		require.NoError(t, err)
		assert.Equal(t, tc.score, out.Score)
		assert.Equal(t, tc.flag, out.Flagged)
		for _, s := range out.Signals {
			assert.NotEmpty(t, s.Explanation)
			assert.Positive(t, s.Detail.Threshold)
		}
	}
	cfg := DefaultConfig()
	cfg.Threshold = 40
	out, err := Evaluate(Input{PolicyStart: start, Loss: start}, cfg)
	require.NoError(t, err)
	assert.True(t, out.Flagged, "skor eşik değerine eşitse işaretli")
	out, err = Evaluate(Input{PolicyStart: start, Loss: start.AddDate(0, 0, 15), PlateClaims: make([]string, 2), WorkshopClaims: make([]string, 4)}, DefaultConfig())
	require.NoError(t, err)
	assert.Zero(t, out.Score, "plaka/atölye yoksa ilgili kural uygulanmaz")
}

func TestWindowAndInvalidConfig(t *testing.T) {
	loss := time.Date(2028, 3, 1, 0, 0, 0, 0, time.UTC)
	from, to := Window(loss, 365)
	assert.Equal(t, loss, to)
	assert.Equal(t, time.Date(2027, 3, 2, 0, 0, 0, 0, time.UTC), from)
	assert.True(t, loss.AddDate(0, 0, -366).Before(from))
	_, err := Evaluate(Input{PolicyStart: loss, Loss: loss}, Config{})
	assert.Error(t, err)
	_, err = Evaluate(Input{PolicyStart: loss, Loss: loss.AddDate(0, 0, -1)}, DefaultConfig())
	assert.Error(t, err)
	cfg := DefaultConfig()
	cfg.EarlyPoints = 2147483647
	assert.Error(t, cfg.Validate())
}
