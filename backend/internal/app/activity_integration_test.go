//go:build integration

package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityAllRolesAndRealEvent(t *testing.T) {
	s, _ := get(t, "", "/api/v1/system/activity")
	assert.Equal(t, http.StatusUnauthorized, s)
	before := time.Now()
	issuePolicy(t, tokenFor(t, "acente@guvenpolice.test"))
	for _, email := range []string{"musteri", "acente", "underwriter", "eksper", "hasar", "muhasebe"} {
		s, out := get(t, tokenFor(t, email+"@guvenpolice.test"), "/api/v1/system/activity")
		require.Equal(t, http.StatusOK, s, out)
		assert.Equal(t, true, out["db_ok"])
		minutes := out["minutes"].([]any)
		require.Len(t, minutes, 30)
		at, err := time.Parse(time.RFC3339Nano, out["last_event_at"].(string))
		require.NoError(t, err)
		assert.False(t, at.Before(before))
		assert.False(t, at.After(time.Now()))
		var total int64
		for _, minute := range minutes {
			m := minute.(map[string]any)
			assert.Len(t, m, 2)
			total += num(m["count"])
		}
		assert.Positive(t, total)
		assert.NotContains(t, out, "actor_user_id")
		assert.NotContains(t, out, "new_value")
	}
}

func TestActivitySimulationAndDatabaseFailure(t *testing.T) {
	ctx := context.Background()
	var id int64
	require.NoError(t, pool.QueryRow(ctx, `
      INSERT INTO simulation_runs(seed,parameters,params_hash,status,phase,processed,target)
      VALUES(42,'{}','a7-activity-test','LIVE','LIVE',123,1000) RETURNING id`).Scan(&id))
	defer func() {
		_, err := pool.Exec(ctx, `DELETE FROM simulation_runs WHERE id=$1`, id)
		require.NoError(t, err)
	}()
	out, err := system.Read(ctx, pool)
	require.NoError(t, err)
	require.NotNil(t, out.Simulation)
	assert.Equal(t, "LIVE", out.Simulation.Status)
	assert.Equal(t, int64(123), out.Simulation.Done)
	dead, cancel := context.WithCancel(ctx)
	cancel()
	out, err = system.Read(dead, pool)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, out.DBOK)
}
