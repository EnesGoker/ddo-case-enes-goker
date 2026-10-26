// Package task görevlerin transaction, atama ve HTTP uygulama katmanıdır.
package task

import (
	"context"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5"
)

const AssignmentLockNamespace int32 = 71006

// Complete ilk kapanış zamanını korur; Appraise ve Transition aynı yardımcıyı kullanır.
func Complete(ctx context.Context, tx pgx.Tx, claimID int64, kind, status string, at time.Time, actorID *int64) error {
	rows, err := tx.Query(ctx, `
  WITH previous AS (
    SELECT id,assigned_to FROM claim_tasks
     WHERE claim_id=$1 AND task_type=$2 AND status IN ('OPEN','IN_PROGRESS') FOR UPDATE
  )
  UPDATE claim_tasks t SET status=$3,completed_at=$4,assigned_to=coalesce($5::bigint,t.assigned_to)
    FROM previous p WHERE t.id=p.id RETURNING t.id,p.assigned_to`, claimID, kind, status, at, actorID)
	if err != nil {
		return err
	}
	previous, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct {
		ID       int64
		Assigned *int64
	}, error) { var value struct {
		ID       int64
		Assigned *int64
	}; err := row.Scan(&value.ID, &value.Assigned); return value, err })
	if err != nil {
		return err
	}
	for _, old := range previous {
		if actorID != nil && (old.Assigned == nil || *old.Assigned != *actorID) {
			if err := audit.Write(ctx, tx, audit.Entry{
				Action: "CLAIM_TASK_TAKEN", EntityType: "claim_task", EntityID: old.ID,
				Old: map[string]any{"assigned_to": old.Assigned}, New: map[string]any{"assigned_to": *actorID},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// Advance çağıranın hasar kilidi altında aynı transactionda çalışır. claim paketini import etmez.
func Advance(ctx context.Context, tx pgx.Tx, claimID int64, from, to string, at time.Time, cfg workflow.Config, actorID *int64) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	status := "DONE"
	if to == "REJECTED" && from == "APPRAISAL" {
		status = "CANCELLED"
	}
	if err := Complete(ctx, tx, claimID, from, status, at, actorID); err != nil {
		return err
	}
	role := "CLAIMS_ADJUSTER"
	switch to {
	case "APPRAISAL":
		role = "APPRAISER"
	case "ADJUSTMENT", "FRAUD_REVIEW":
	default:
		return nil
	}
	due, err := workflow.DueAt(at, to, cfg)
	if err != nil {
		return err
	}
	// Sayımdan önce rol kilidi: farklı hasarların atamaları da aynı havuzda sıralanır.
	if _, err := tx.Exec(ctx, `
  SELECT pg_advisory_xact_lock($1,hashtext($2))`, AssignmentLockNamespace, role); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
  SELECT u.id,count(t.id) FROM users u
   LEFT JOIN claim_tasks t ON t.assigned_to=u.id AND t.status IN ('OPEN','IN_PROGRESS')
   WHERE u.role=$1 AND u.is_active GROUP BY u.id ORDER BY u.id`, role)
	if err != nil {
		return err
	}
	loads, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (workflow.Load, error) {
		var l workflow.Load
		err := r.Scan(&l.UserID, &l.Open)
		return l, err
	})
	if err != nil {
		return err
	}
	assigned := workflow.PickAssignee(loads)
	_, err = tx.Exec(ctx, `
  INSERT INTO claim_tasks(claim_id,task_type,assigned_to,status,created_at,sla_due_at)
  VALUES($1,$2,$3,'OPEN',$4,$5)`, claimID, to, assigned, at, due)
	return err
}
