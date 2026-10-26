//go:build integration && simulation

package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/app"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/EnesGoker/guvenpolice/backend/internal/simulation"
)

func TestVerifyStopsBeforeBusinessRequestsDuringLive(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_verify_live")
	require.NoError(t, err)
	defer drop()
	require.NoError(t, db.Migrate(ctx, u, migrationsDir))
	apiPool, err := db.Connect(ctx, u, 10*time.Second)
	require.NoError(t, err)
	defer apiPool.Close()
	require.NoError(t, seed.Demo(ctx, apiPool, demoPassword))
	simPool, err := simulation.OpenPool(ctx, u, 2)
	require.NoError(t, err)
	defer simPool.Close()
	cfg := simulation.DefaultConfig()
	cfg.Policies, cfg.Workers, cfg.Live = 10, 2, true
	liveCtx, stop := context.WithCancel(ctx)
	defer stop()
	finished := make(chan error, 1)
	go func() {
		_, err := simulation.Run(liveCtx, simPool, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
		finished <- err
	}()
	defer func() {
		stop()
		select {
		case err := <-finished:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("canlı simülatör kapanmadı")
		}
	}()
	require.Eventually(t, func() bool {
		var status string
		return apiPool.QueryRow(ctx, `SELECT status FROM simulation_runs ORDER BY id DESC LIMIT 1`).Scan(&status) == nil && status == "LIVE"
	}, 20*time.Second, 20*time.Millisecond)

	router := app.NewRouter(app.Deps{
		Pool: apiPool, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		JWTSecret:      []byte("integration-secret-at-least-32-bytes"),
		AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour,
	})
	var unexpected atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" && r.URL.Path != "/api/v1/system/activity" {
			unexpected.Add(1)
		}
		router.ServeHTTP(w, r)
	}))
	defer api.Close()
	commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, "bash", "../../../scripts/verify.sh")
	command.Env = append(os.Environ(), "API_URL="+api.URL, "WEB_URL=http://127.0.0.1:1", "DEMO_PASSWORD="+demoPassword)
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError, string(output))
	assert.Equal(t, 2, exitError.ExitCode(), string(output))
	assert.Contains(t, string(output), "Verify durduruldu: simülatör LIVE/RUNNING")
	assert.NotContains(t, string(output), "PASS")
	assert.Zero(t, unexpected.Load(), "ön kontrol dışı HTTP isteği gitmemeli")
}
