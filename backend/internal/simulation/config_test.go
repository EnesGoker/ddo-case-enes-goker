package simulation

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDefaultsAndParameters(t *testing.T) {
	c := DefaultConfig()
	assert.Equal(t, 100000, c.Policies)
	assert.Equal(t, 5, c.Years)
	assert.Equal(t, 8, c.Workers)
	require.NoError(t, c.Validate())
	_, a, err := c.parameters()
	require.NoError(t, err)
	c.Workers = 16
	c.Live = true
	_, b, err := c.parameters()
	require.NoError(t, err)
	assert.Equal(t, a, b)
	c.Seed++
	_, b, err = c.parameters()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	c.Workers = 0
	assert.Error(t, c.Validate())
}

func TestLoadConfigRejectsInvalidEnvironment(t *testing.T) {
	for key, value := range map[string]string{"SIM_POLICIES": "no", "SIM_SEED": "no", "SIM_WORKERS": "65", "SIM_YEARS": "0", "SIM_ANCHOR_DATE": "2026-02-29", "SIM_LIVE": "no"} {
		t.Run(key, func(t *testing.T) { t.Setenv(key, value); _, err := LoadConfig(); assert.Error(t, err) })
	}
}
