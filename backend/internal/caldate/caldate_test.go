package caldate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(s string) time.Time {
	t, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseIsStrict(t *testing.T) {
	_, err := Parse("2026-02-29")
	assert.Error(t, err, "2026 artık yıl değil")
	_, err = Parse("2028-02-29")
	assert.NoError(t, err)
	_, err = Parse("26-1-1")
	assert.Error(t, err)
}

func TestAgeOn(t *testing.T) {
	cases := []struct {
		birth, on string
		want      int
	}{
		{"1988-04-12", "2026-04-11", 37},
		{"1988-04-12", "2026-04-12", 38}, // doğum günü dahil
		{"2000-02-29", "2021-02-28", 20}, // artık olmayan yıl: 28 Şubat'ta henüz değil
		{"2000-02-29", "2021-03-01", 21},
		{"2000-02-29", "2024-02-29", 24},
		{"2005-12-31", "2026-12-31", 21},
		{"2005-12-31", "2026-12-30", 20},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, AgeOn(d(c.birth), d(c.on)), c.birth+" @ "+c.on)
	}
}

func TestTodayUsesTurkeyTime(t *testing.T) {
	// 31 Aralık 22:30 UTC = 1 Ocak 01:30 Türkiye
	now := time.Date(2026, 12, 31, 22, 30, 0, 0, time.UTC)
	assert.Equal(t, "2027-01-01", Format(Today(now)))
	now = time.Date(2026, 12, 31, 20, 59, 0, 0, time.UTC)
	assert.Equal(t, "2026-12-31", Format(Today(now)))
}

func TestAddYear(t *testing.T) {
	assert.Equal(t, "2027-11-01", Format(AddYear(d("2026-11-01"))))
	assert.Equal(t, "2029-03-01", Format(AddYear(d("2028-02-29"))))
	assert.Equal(t, "2027-01-01", Format(AddYear(d("2026-01-01"))))
}

func TestOf(t *testing.T) {
	in := time.Date(2026, 5, 6, 23, 59, 0, 0, time.UTC)
	require.Equal(t, time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), Of(in))
}
