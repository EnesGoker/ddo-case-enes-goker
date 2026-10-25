// Package task görevlerin transaction, atama ve HTTP uygulama katmanıdır.
package task

import (
	"context"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5"
)

const AssignmentLockNamespace int32 = 71006

// Complete ilk kapanış zamanını korur; Appraise ve Transition aynı yardımcıyı kullanır.
func Complete(ctx context.Context, tx pgx.Tx, claimID int64, kind, status string, at time.Time) error {
	_, err := tx.Exec(ctx, `
  UPDATE claim_tasks SET status=$3,completed_at=$4
   WHERE claim_id=$1 AND task_type=$2 AND status IN ('OPEN','IN_PROGRESS')`, claimID, kind, status, at)
	return err
}

// Advance çağıranın hasar kilidi altında aynı transactionda çalışır. claim paketini import etmez.
func Advance(ctx context.Context, tx pgx.Tx, claimID int64, from, to string, at time.Time, cfg workflow.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	status := "DONE"
	if to == "REJECTED" {
		status = "CANCELLED"
	}
	if err := Complete(ctx, tx, claimID, from, status, at); err != nil {
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
