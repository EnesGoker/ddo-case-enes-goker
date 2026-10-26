//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/claim"
	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/task"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCompletionTakesOwnershipAndRejectionIsWork(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := context.Background()
	adjuster := tokenFor(t, "hasar@guvenpolice.test")
	actorCtx := principalContext(t, "hasar@guvenpolice.test", auth.RoleClaimsAdjuster)
	var appraiser, otherAppraiser int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM users WHERE email='eksper@guvenpolice.test'`).Scan(&appraiser))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM users WHERE email='eksper2@guvenpolice.test'`).Scan(&otherAppraiser))
	for _, kind := range []string{"APPRAISAL", "ADJUSTMENT", "FRAUD_REVIEW"} {
		t.Run(kind, func(t *testing.T) {
			c := reportClaim(t, adjuster, claimPolicy(t), "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
			c = transitionClaim(t, adjuster, c, "TRIAGED")
			if kind == "ADJUSTMENT" {
				c = transitionClaim(t, adjuster, c, "APPRAISAL")
				s, body := postAppraisal(t, claimPath(c)+"/appraisals", tokenFor(t, "eksper@guvenpolice.test"), map[string]any{
					"damage_amount_kurus": 100, "inspected_at": time.Now().UTC().Format(time.RFC3339),
				})
				require.Equal(t, http.StatusCreated, s, body)
			}
			past := time.Now().Add(-5 * 24 * time.Hour)
			svc := claim.NewService(pool, func() time.Time { return past }, fraud.DefaultConfig(), workflow.DefaultConfig())
			entered, err := svc.ChangeStatus(actorCtx, num(c["id"]), claim.TransitionRequest{
				ToStatus: kind, Note: "Geçmiş görev", ExpectedRowVersion: int(num(c["row_version"])),
			})
			require.NoError(t, err)
			_, err = pool.Exec(ctx, `UPDATE claim_tasks SET assigned_to=$2 WHERE claim_id=$1 AND status='OPEN'`, entered.ID, otherAppraiser)
			require.NoError(t, err)
			c["row_version"] = entered.RowVersion
			c = transitionClaim(t, adjuster, c, "REJECTED")
			var status string
			var assigned int64
			require.NoError(t, pool.QueryRow(ctx, `
              SELECT status,assigned_to FROM claim_tasks WHERE claim_id=$1 AND task_type=$2 ORDER BY id DESC LIMIT 1`, entered.ID, kind).Scan(&status, &assigned))
			actor, _ := auth.FromContext(actorCtx)
			assert.Equal(t, actor.UserID, assigned)
			expected := "DONE"
			if kind == "APPRAISAL" {
				expected = "CANCELLED"
			}
			assert.Equal(t, expected, status)
			s, list := get(t, adjuster, fmt.Sprintf("/api/v1/claim-tasks?claim_id=%d&status=all&breached=true", entered.ID))
			require.Equal(t, http.StatusOK, s, list)
			if expected == "DONE" {
				assert.Len(t, list["items"], 1)
			} else {
				assert.Empty(t, list["items"], "iş yapılmadan iptal SLA başarısı/ihlali sayılmaz")
			}
		})
	}
	// Ekspertizi giren görev sahibini devralır ve eski atanan audit'te kalır.
	c := reportClaim(t, adjuster, claimPolicy(t), "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
	c = transitionClaim(t, adjuster, c, "TRIAGED")
	c = transitionClaim(t, adjuster, c, "APPRAISAL")
	_, err := pool.Exec(ctx, `UPDATE claim_tasks SET assigned_to=$2 WHERE claim_id=$1 AND status='OPEN'`, num(c["id"]), otherAppraiser)
	require.NoError(t, err)
	s, body := postAppraisal(t, claimPath(c)+"/appraisals", tokenFor(t, "eksper@guvenpolice.test"), map[string]any{
		"damage_amount_kurus": 100, "inspected_at": time.Now().UTC().Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, s, body)
	var assigned, oldAssigned, newAssigned int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT assigned_to FROM claim_tasks WHERE claim_id=$1 AND task_type='APPRAISAL'`, num(c["id"])).Scan(&assigned))
	assert.Equal(t, appraiser, assigned)
	require.NoError(t, pool.QueryRow(ctx, `
      SELECT (a.old_value->>'assigned_to')::bigint,(a.new_value->>'assigned_to')::bigint
        FROM audit_log a JOIN claim_tasks t ON a.entity_id=t.id::text
       WHERE a.entity_type='claim_task' AND a.action='CLAIM_TASK_TAKEN' AND t.claim_id=$1`, num(c["id"])).Scan(&oldAssigned, &newAssigned))
	assert.Equal(t, otherAppraiser, oldAssigned)
	assert.Equal(t, appraiser, newAssigned)
	// Sistem geçişi kapatır ama kullanıcının görevini üstlenmez.
	c = transitionClaim(t, adjuster, c, "ADJUSTMENT")
	_, err = pool.Exec(ctx, `UPDATE claim_tasks SET assigned_to=$2 WHERE claim_id=$1 AND status='OPEN'`, num(c["id"]), otherAppraiser)
	require.NoError(t, err)
	require.NoError(t, pgx.BeginFunc(actorCtx, pool, func(tx pgx.Tx) error {
		actor, _ := auth.FromContext(actorCtx)
		_, err := claim.Transition(actorCtx, tx, num(c["id"]), "REJECTED", actor, "Sistem kararı", int(num(c["row_version"])), claim.TransitionOptions{
			At: time.Now(), SLA: workflow.DefaultConfig(), Automatic: true,
		})
		return err
	}))
	require.NoError(t, pool.QueryRow(ctx, `SELECT assigned_to FROM claim_tasks WHERE claim_id=$1 AND task_type='ADJUSTMENT'`, num(c["id"])).Scan(&assigned))
	assert.Equal(t, otherAppraiser, assigned)
	summary, err := task.NewService(pool, time.Now).Summary(ctx)
	require.NoError(t, err)
	for _, row := range summary {
		if row.Type == "FRAUD_REVIEW" || row.Type == "ADJUSTMENT" {
			assert.Positive(t, row.DoneLate)
		}
	}
}
