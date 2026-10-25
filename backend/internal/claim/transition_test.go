package claim

import (
	"context"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/stretchr/testify/assert"
)

func TestTransitionRejectsZeroOptionsBeforeDatabaseAccess(t *testing.T) {
	for _, options := range []TransitionOptions{{}, {At: time.Now()}, {SLA: workflow.DefaultConfig()}, {At: time.Now(), SLA: workflow.Config{Appraisal: time.Hour}}} {
		_, err := Transition(context.Background(), nil, 1, "APPRAISAL", auth.Principal{}, "test", 1, options)
		assert.Error(t, err)
	}
}
