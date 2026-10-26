package workflow

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGoldenSLA(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	for kind, hours := range map[string]int{"APPRAISAL": 72, "ADJUSTMENT": 48, "FRAUD_REVIEW": 24} {
		due, err := DueAt(now, kind, DefaultConfig())
		require.NoError(t, err)
		assert.Equal(t, now.Add(time.Duration(hours)*time.Hour), due)
		assert.False(t, Breached("OPEN", due, nil, due))
		assert.True(t, Breached("OPEN", due, nil, due.Add(time.Nanosecond)))
		late := due.Add(time.Second)
		assert.True(t, Breached("DONE", due, &late, now))
		assert.False(t, Breached("CANCELLED", due, &late, now))
		assert.False(t, Breached("DONE", due, &due, late))
		assert.Equal(t, int64(-1), RemainingSeconds(due, due.Add(time.Nanosecond)))
		assert.Equal(t, int64(1), RemainingSeconds(due, due.Add(-time.Second)))
	}
	_, err := DueAt(now, "APPRAISAL", Config{})
	assert.Error(t, err, "sıfır SLA sessizce kabul edilmez")
	_, err = DueAt(time.Time{}, "APPRAISAL", DefaultConfig())
	assert.Error(t, err)
	_, err = DueAt(now, "OTHER", DefaultConfig())
	assert.Error(t, err)
}

func TestPickAssignee(t *testing.T) {
	assert.Nil(t, PickAssignee(nil))
	assert.Equal(t, int64(2), *PickAssignee([]Load{{UserID: 3, Open: 0}, {UserID: 2, Open: 0}, {UserID: 1, Open: 1}}))
	assert.True(t, Open("IN_PROGRESS"))
	assert.False(t, Open("CANCELLED"))
}
