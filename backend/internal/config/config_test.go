package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFraudAndSLAEnvironmentValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/demo")
	t.Setenv("JWT_SECRET", "fictional-unit-test-secret-32-bytes")
	t.Setenv("SEED_DEMO", "false")
	t.Run("defaults", func(t *testing.T) {
		c, err := Load()
		require.NoError(t, err)
		assert.Equal(t, 60, c.Fraud.Threshold)
		assert.Equal(t, 72*time.Hour, c.Workflow.Appraisal)
	})
	for _, key := range []string{"FRAUD_THRESHOLD", "FRAUD_EARLY_DAYS", "FRAUD_EARLY_POINTS", "FRAUD_REPEAT_DAYS", "FRAUD_REPEAT_MINIMUM", "FRAUD_REPEAT_POINTS", "FRAUD_WORKSHOP_DAYS", "FRAUD_WORKSHOP_MINIMUM", "FRAUD_WORKSHOP_POINTS", "SLA_APPRAISAL", "SLA_ADJUSTMENT", "SLA_FRAUD_REVIEW"} {
		t.Run(key, func(t *testing.T) {
			for _, value := range []string{"0", "-1", "invalid"} {
				t.Setenv(key, value)
				_, err := Load()
				assert.Error(t, err)
			}
		})
	}
	t.Run("overrides", func(t *testing.T) {
		t.Setenv("FRAUD_THRESHOLD", "75")
		t.Setenv("SLA_APPRAISAL", "96h")
		c, err := Load()
		require.NoError(t, err)
		assert.Equal(t, 75, c.Fraud.Threshold)
		assert.Equal(t, 96*time.Hour, c.Workflow.Appraisal)
	})
}
