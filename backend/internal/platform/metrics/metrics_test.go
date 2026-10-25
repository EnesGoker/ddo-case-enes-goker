package metrics

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectorScrapesFreshAndOmitsFailedSamples(t *testing.T) {
	r := NewRegistry()
	calls := 0
	r.Collect(func(ctx context.Context) ([]Sample, error) {
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
		// Add is safe because collector runs outside registry mutex.
		r.Add("requests_total", 1)
		calls++
		if calls == 2 {
			return nil, errors.New("database unavailable")
		}
		return []Sample{{Name: "open_tasks", Value: int64(calls), Labels: []string{"task_type", "APPRAISAL"}}}, nil
	})
	for _, want := range []string{`open_tasks{task_type="APPRAISAL"} 1`, "", `open_tasks{task_type="APPRAISAL"} 3`} {
		w := httptest.NewRecorder()
		r.Handler()(w, httptest.NewRequest("GET", "/metrics", nil))
		assert.Equal(t, 200, w.Code)
		if want == "" {
			assert.NotContains(t, w.Body.String(), "open_tasks")
		} else {
			assert.Contains(t, w.Body.String(), want)
		}
	}
}
