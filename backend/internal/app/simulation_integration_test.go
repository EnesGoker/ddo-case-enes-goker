//go:build integration

package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/EnesGoker/guvenpolice/backend/internal/simulation"
)

func TestSimulatorDeterminismAndInvariants(t *testing.T) {
	ctx := context.Background()
	cfg := simulation.DefaultConfig()
	cfg.Policies = 1000
	var fingerprints []string
	for run := range 3 {
		if run == 1 {
			cfg.Workers = 16
		}
		if run == 2 {
			cfg.Seed++
			cfg.Workers = 8
		}
		u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_sim")
		require.NoError(t, err)
		func() {
			defer drop()
			require.NoError(t, db.Migrate(ctx, u, migrationsDir))
			p, err := simulation.OpenPool(ctx, u, cfg.Workers)
			require.NoError(t, err)
			defer p.Close()
			require.NoError(t, seed.Demo(ctx, p, demoPassword, seed.Options{Now: func() time.Time { return cfg.Anchor.Add(12 * time.Hour) }}))
			connections := make([]func(), 0, 3)
			for range 3 {
				conn, err := p.Acquire(ctx)
				require.NoError(t, err)
				connections = append(connections, conn.Release)
				var synchronous string
				require.NoError(t, conn.QueryRow(ctx, `SHOW synchronous_commit`).Scan(&synchronous))
				assert.Equal(t, "off", synchronous)
			}
			for _, release := range connections {
				release()
			}
			if run == 0 {
				guard, err := p.Acquire(ctx)
				require.NoError(t, err)
				_, err = guard.Exec(ctx, `SELECT pg_advisory_lock(71007,1)`)
				require.NoError(t, err)
				_, err = simulation.Run(ctx, p, cfg, nil)
				assert.ErrorContains(t, err, "başka simülatör")
				_, err = guard.Exec(ctx, `SELECT pg_advisory_unlock(71007,1)`)
				require.NoError(t, err)
				guard.Release()
			}
			started := time.Now()
			result, err := simulation.Run(ctx, p, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			elapsed := time.Since(started)
			t.Logf("1000 policies seed=%d workers=%d elapsed=%s projection100k=%s", cfg.Seed, cfg.Workers, elapsed, elapsed*100)
			require.NoError(t, err)
			require.Len(t, result.Fingerprint, 64)
			fingerprints = append(fingerprints, result.Fingerprint)
			var n, before, after int
			require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM policies WHERE policy_no LIKE 'SIM-%'`).Scan(&n))
			assert.Equal(t, 1000, n)
			require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&before))
			replay, err := simulation.Run(ctx, p, cfg, nil)
			require.NoError(t, err)
			assert.True(t, replay.Reused)
			assert.Equal(t, result.Fingerprint, replay.Fingerprint)
			require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&after))
			assert.Equal(t, before, after)
			trial, err := ledger.ReadTrialBalance(ctx, p)
			require.NoError(t, err)
			assert.True(t, trial.Balanced)
			assert.True(t, trial.ReceivableReconciliation.Matches)
			require.NoError(t, p.QueryRow(ctx, `
              SELECT count(*) FROM claims c JOIN policies p ON p.id=c.policy_id
               WHERE p.policy_no LIKE 'SIM-%' AND c.fraud_flagged`).Scan(&n))
			assert.Positive(t, n)
			require.NoError(t, p.QueryRow(ctx, `
              SELECT count(*) FROM claim_status_history h JOIN claims c ON c.id=h.claim_id
               JOIN policies p ON p.id=c.policy_id LEFT JOIN claim_transitions tr ON tr.from_status=h.from_status AND tr.to_status=h.to_status
               WHERE p.policy_no LIKE 'SIM-%' AND h.from_status IS NOT NULL AND tr.from_status IS NULL`).Scan(&n))
			assert.Zero(t, n)
			require.NoError(t, p.QueryRow(ctx, `
              SELECT count(*) FROM (
                SELECT h.created_at,lag(h.created_at) OVER(PARTITION BY h.claim_id ORDER BY h.id) AS previous
                FROM claim_status_history h JOIN claims c ON c.id=h.claim_id JOIN policies p ON p.id=c.policy_id WHERE p.policy_no LIKE 'SIM-%'
              ) x WHERE previous > created_at`).Scan(&n))
			assert.Zero(t, n, "olay saati geriye gitmez")
			for _, query := range []string{
				`SELECT count(*) FROM policies p JOIN customers c ON c.id=p.customer_id JOIN agents a ON a.id=p.agent_id JOIN users u ON u.id=a.user_id WHERE p.policy_no LIKE 'SIM-%' AND (c.national_id NOT LIKE '9%' OR u.email NOT LIKE 'sim-%')`,
				`SELECT count(*) FROM claims c JOIN policies p ON p.id=c.policy_id JOIN users u ON u.id=c.created_by LEFT JOIN workshops w ON w.id=c.workshop_id WHERE p.policy_no LIKE 'SIM-%' AND (u.email NOT LIKE 'sim-%' OR (w.id IS NOT NULL AND w.name NOT LIKE 'Kurgusal SIM%'))`,
				`SELECT count(*) FROM appraisals a JOIN claims c ON c.id=a.claim_id JOIN policies p ON p.id=c.policy_id JOIN users u ON u.id=a.appraiser_id WHERE p.policy_no LIKE 'SIM-%' AND u.email NOT LIKE 'sim-%'`,
				`SELECT count(*) FROM installments i JOIN installment_plans pl ON pl.id=i.plan_id JOIN policies p ON p.id=pl.policy_id WHERE p.policy_no LIKE 'SIM-%' AND i.due_date > CURRENT_DATE AND i.paid_kurus>0`,
				`SELECT count(*) FROM policies p JOIN policy_versions v ON v.policy_id=p.id JOIN installment_plans pl ON pl.policy_id=p.id WHERE p.policy_no LIKE 'SIM-%' AND (p.created_at::date<>p.start_date OR v.recorded_at::date<>p.start_date OR pl.created_at::date<>p.start_date)`,
				`SELECT count(*) FROM policies p JOIN policy_versions v ON v.policy_id=p.id JOIN policy_versions d ON d.vehicle_plate=v.vehicle_plate JOIN policies dp ON dp.id=d.policy_id WHERE p.policy_no LIKE 'SIM-%' AND dp.policy_no NOT LIKE 'SIM-%'`,
			} {
				require.NoError(t, p.QueryRow(ctx, query).Scan(&n))
				assert.Zero(t, n, query)
			}
			changed := cfg
			changed.Seed += 99
			_, err = simulation.Run(ctx, p, changed, nil)
			assert.ErrorContains(t, err, "reset-demo")
			if run == 0 {
				var claimsBefore int
				require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM claims`).Scan(&claimsBefore))
				liveCtx, stop := context.WithCancel(ctx)
				defer stop()
				liveCfg := cfg
				liveCfg.Live = true
				finished := make(chan error, 1)
				go func() { _, err := simulation.Run(liveCtx, p, liveCfg, nil); finished <- err }()
				require.Eventually(t, func() bool {
					var status string
					return p.QueryRow(ctx, `SELECT status FROM simulation_runs WHERE id=$1`, result.RunID).Scan(&status) == nil && status == "LIVE"
				}, 3*time.Second, 20*time.Millisecond)
				require.Eventually(t, func() bool {
					var count int
					return p.QueryRow(ctx, `SELECT count(*) FROM policies WHERE policy_no LIKE 'SIM-%'`).Scan(&count) == nil && count > 1000
				}, 8*time.Second, 50*time.Millisecond)
				stop()
				select {
				case err := <-finished:
					require.NoError(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("canlı mod kapanmadı")
				}
				var claimsAfter int
				require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM claims`).Scan(&claimsAfter))
				assert.Equal(t, claimsBefore, claimsAfter)
				var status string
				var fingerprint *string
				require.NoError(t, p.QueryRow(ctx, `SELECT status,fingerprint FROM simulation_runs WHERE id=$1`, result.RunID).Scan(&status, &fingerprint))
				assert.Equal(t, "COMPLETED", status)
				assert.Nil(t, fingerprint, "canlı mod deterministik parmak izini geçersiz kılar")
				trial, err := ledger.ReadTrialBalance(ctx, p)
				require.NoError(t, err)
				assert.True(t, trial.Balanced)
				assert.True(t, trial.ReceivableReconciliation.Matches)
			}
		}()
	}
	assert.Equal(t, fingerprints[0], fingerprints[1], "worker sayısı iş içeriğini değiştirmez")
	assert.NotEqual(t, fingerprints[0], fingerprints[2])
	var synchronous string
	require.NoError(t, pool.QueryRow(ctx, `SHOW synchronous_commit`).Scan(&synchronous))
	assert.Equal(t, "on", synchronous, "API/test havuzu etkilenmez")
}

func TestSimulatorResetRemovesDatasetAndRestoresTariffs(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_sim_reset")
	require.NoError(t, err)
	defer drop()
	require.NoError(t, db.Migrate(ctx, u, migrationsDir))
	cfg := simulation.DefaultConfig()
	cfg.Policies = 10
	p, err := simulation.OpenPool(ctx, u, cfg.Workers)
	require.NoError(t, err)
	defer p.Close()
	require.NoError(t, seed.Demo(ctx, p, demoPassword))
	_, err = simulation.Run(ctx, p, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	// reset-demo'nun aynı whitelist'i; yalnız bu testin taze veritabanına uygulanır.
	_, err = p.Exec(ctx, `
      DO $$ DECLARE tables text;
      BEGIN
        SELECT string_agg(format('%I.%I',schemaname,tablename),', ') INTO tables
          FROM pg_tables WHERE schemaname='public'
           AND tablename NOT IN ('goose_db_version','accounts','claim_transitions');
        EXECUTE 'TRUNCATE ' || tables || ' RESTART IDENTITY CASCADE';
      END $$`)
	require.NoError(t, err)
	require.NoError(t, seed.Demo(ctx, p, demoPassword))
	var simPolicies, runs, historical int
	require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM policies WHERE policy_no LIKE 'SIM-%'`).Scan(&simPolicies))
	require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM simulation_runs`).Scan(&runs))
	require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM tariff_versions WHERE version_no=0`).Scan(&historical))
	assert.Zero(t, simPolicies)
	assert.Zero(t, runs)
	assert.Equal(t, 3, historical)
	trial, err := ledger.ReadTrialBalance(ctx, p)
	require.NoError(t, err)
	assert.True(t, trial.Balanced)
	assert.True(t, trial.ReceivableReconciliation.Matches)
}
