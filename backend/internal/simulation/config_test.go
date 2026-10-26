package simulation

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
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
	raw, b, err := c.parameters()
	require.NoError(t, err)
	assert.Equal(t, a, b)
	assert.Contains(t, string(raw), `"workers":16`)
	assert.Contains(t, string(raw), `"live":true`)
	assert.Contains(t, string(raw), `"version":3`)
	assert.Equal(t, 48*time.Hour, c.OpenWindow)
	c.Seed++
	_, b, err = c.parameters()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	c.Workers = 0
	assert.Error(t, c.Validate())
}

func TestLoadConfigRejectsInvalidEnvironment(t *testing.T) {
	for key, value := range map[string]string{"SIM_POLICIES": "no", "SIM_SEED": "no", "SIM_WORKERS": "65", "SIM_YEARS": "0", "SIM_ANCHOR_DATE": "2026-02-29", "SIM_LIVE": "no", "SIM_OPEN_WINDOW": "0h"} {
		t.Run(key, func(t *testing.T) { t.Setenv(key, value); _, err := LoadConfig(); assert.Error(t, err) })
	}
}

func TestSimulationTariffStartAndWindow(t *testing.T) {
	c := DefaultConfig()
	c.Anchor = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, c.Validate())
	c.Anchor = c.Anchor.AddDate(0, 0, -1)
	assert.ErrorContains(t, c.Validate(), "2020-01-01")
	c = DefaultConfig()
	_, before, err := c.parameters()
	require.NoError(t, err)
	t.Setenv("SIM_OPEN_WINDOW", "24h")
	loaded, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, loaded.OpenWindow)
	_, after, err := loaded.parameters()
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}
