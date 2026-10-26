//go:build integration && simulation

package app_test

import (
	"context"
	"fmt"
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

func TestSimulatorFraudRateAt1kAnd10k(t *testing.T) {
	for _, policies := range []int{1000, 10000} {
		t.Run(fmt.Sprint(policies), func(t *testing.T) {
			ctx := context.Background()
			u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_sim_fraud")
			require.NoError(t, err)
			defer drop()
			require.NoError(t, db.Migrate(ctx, u, migrationsDir))
			cfg := simulation.DefaultConfig()
			cfg.Anchor = time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
			cfg.Policies, cfg.Workers = policies, 16
			p, err := simulation.OpenPool(ctx, u, cfg.Workers)
			require.NoError(t, err)
			defer p.Close()
			require.NoError(t, seed.Demo(ctx, p, demoPassword))
			started := time.Now()
			result, err := simulation.Run(ctx, p, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.NoError(t, err)
			var total, flagged, workshops, bursts int64
			require.NoError(t, p.QueryRow(ctx, `
                SELECT count(*),count(*) FILTER(WHERE c.fraud_flagged)
                  FROM claims c JOIN policies p ON p.id=c.policy_id WHERE p.policy_no LIKE 'SIM-%'`).Scan(&total, &flagged))
			require.NoError(t, p.QueryRow(ctx, `SELECT count(*) FROM workshops WHERE name LIKE 'Kurgusal SIM%'`).Scan(&workshops))
			assert.Equal(t, int64(300), workshops)
			require.Positive(t, total)
			assert.Positive(t, flagged)
			assert.Less(t, flagged*10, total, "işaretli oranı kesin olarak %%10 altında kalmalı")
			require.NoError(t, p.QueryRow(ctx, `
                SELECT count(*) FROM fraud_signals f JOIN claims c ON c.id=f.claim_id
                  JOIN policies p ON p.id=c.policy_id
                 WHERE p.policy_no LIKE 'SIM-%' AND f.rule_code='WORKSHOP_CONCENTRATION'`).Scan(&bursts))
			assert.Positive(t, bursts, "kasıtlı atölye patlaması korunmalı")
			trial, err := ledger.ReadTrialBalance(ctx, p)
			require.NoError(t, err)
			assert.True(t, trial.Balanced)
			assert.True(t, trial.ReceivableReconciliation.Matches)
			var mismatch, oldOpen, open, breached, review int64
			require.NoError(t, p.QueryRow(ctx, `
                SELECT count(*) FROM claim_tasks t JOIN appraisals a ON a.claim_id=t.claim_id
                  JOIN claims c ON c.id=t.claim_id JOIN policies p ON p.id=c.policy_id
                 WHERE p.policy_no LIKE 'SIM-%' AND t.task_type='APPRAISAL' AND t.status='DONE'
                   AND t.assigned_to IS DISTINCT FROM a.appraiser_id`).Scan(&mismatch))
			assert.Zero(t, mismatch)
			asOf := cfg.Anchor.Add(24 * time.Hour)
			require.NoError(t, p.QueryRow(ctx, `
                SELECT count(*) FILTER(WHERE t.created_at<$1),count(*),count(*) FILTER(WHERE t.sla_due_at<$2)
                  FROM claim_tasks t JOIN claims c ON c.id=t.claim_id JOIN policies p ON p.id=c.policy_id
                 WHERE p.policy_no LIKE 'SIM-%' AND t.status IN ('OPEN','IN_PROGRESS')`, asOf.Add(-cfg.OpenWindow), asOf).Scan(&oldOpen, &open, &breached))
			assert.Zero(t, oldOpen, "pencere dışındaki işler tamamlanır")
			if open > 0 {
				assert.Less(t, breached*2, open, "açık görev ihlali %%50 altında")
			}
			require.NoError(t, p.QueryRow(ctx, `
                SELECT count(*) FROM claims c JOIN policies p ON p.id=c.policy_id
                 WHERE p.policy_no LIKE 'SIM-%' AND c.status='FRAUD_REVIEW'`).Scan(&review))
			t.Logf("window=%s open_review=%d open_tasks=%d breached=%d mismatched_appraisers=%d", cfg.OpenWindow, review, open, breached, mismatch)
			t.Logf("N=%d workshops=%d claims=%d flagged=%d rate_bp=%d workshop_signals=%d seconds=%.3f fingerprint=%s", policies, workshops, total, flagged, flagged*10000/total, bursts, time.Since(started).Seconds(), result.Fingerprint)
			rows, err := p.Query(ctx, `
                SELECT f.rule_code,count(DISTINCT c.id) FROM fraud_signals f JOIN claims c ON c.id=f.claim_id
                  JOIN policies p ON p.id=c.policy_id WHERE p.policy_no LIKE 'SIM-%' AND c.fraud_flagged
                 GROUP BY f.rule_code ORDER BY f.rule_code`)
			require.NoError(t, err)
			defer rows.Close()
			for rows.Next() {
				var rule string
				var count int
				require.NoError(t, rows.Scan(&rule, &count))
				t.Logf("flagged rule=%s claims=%d", rule, count)
			}
			require.NoError(t, rows.Err())
		})
	}
}
