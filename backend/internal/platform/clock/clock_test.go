package clock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestContextTimeIsolationAndRealFallback(t *testing.T) {
	base := context.Background()
	past := time.Date(2021, 2, 3, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			at := past.Add(time.Duration(i) * time.Hour)
			ctx := WithTime(base, at)
			assert.Equal(t, at, Now(ctx))
			before := time.Now()
			real := Now(base)
			assert.False(t, real.Before(before))
			assert.False(t, real.After(time.Now()))
		})
	}
	wg.Wait()
	assert.WithinDuration(t, time.Now(), Now(WithTime(base, time.Time{})), time.Second)
}
