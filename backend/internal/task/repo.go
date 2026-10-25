package task

import (
	"context"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/lossratio"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type View struct {
	ID               int64      `json:"id"`
	ClaimID          int64      `json:"claim_id"`
	ClaimNo          string     `json:"claim_no"`
	Type             string     `json:"task_type"`
	Status           string     `json:"status"`
	AssignedTo       *User      `json:"assigned_to"`
	CreatedAt        time.Time  `json:"created_at"`
	DueAt            time.Time  `json:"sla_due_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	Breached         bool       `json:"breached"`
	RemainingSeconds int64      `json:"remaining_seconds"`
	FraudScore       *int       `json:"fraud_score,omitempty"`
	Signals          []string   `json:"signal_codes,omitempty"`
}

type Filter struct {
	Status     string
	Type       string
	ClaimID    int64
	AssignedTo int64
	Breached   bool
}

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, now: now}
}

func List(ctx context.Context, q db.DBTX, params page.Params, f Filter, now time.Time) ([]View, error) {
	rows, err := q.Query(ctx, `
  SELECT t.id,t.claim_id,c.claim_no,t.task_type,t.status,u.id,u.full_name,
         t.created_at,t.sla_due_at,t.completed_at,c.fraud_score,
         ARRAY(SELECT rule_code FROM fraud_signals f WHERE f.claim_id=c.id ORDER BY f.id)
    FROM claim_tasks t JOIN claims c ON c.id=t.claim_id LEFT JOIN users u ON u.id=t.assigned_to
   WHERE ($1::bigint=0 OR t.id<$1)
     AND ($2='all' OR ($2='open' AND t.status IN ('OPEN','IN_PROGRESS'))
                      OR ($2='done' AND t.status IN ('DONE','CANCELLED')))
     AND ($3='' OR t.task_type=$3) AND ($4::bigint=0 OR t.claim_id=$4)
     AND ($5::bigint=0 OR t.assigned_to=$5)
     AND (NOT $6::boolean OR CASE WHEN t.status IN ('OPEN','IN_PROGRESS')
           THEN t.sla_due_at<$7 ELSE t.completed_at>t.sla_due_at END)
   ORDER BY t.id DESC LIMIT $8`, params.AfterID, f.Status, f.Type, f.ClaimID, f.AssignedTo, f.Breached, now, params.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (View, error) {
		var v View
		var uid *int64
		var name *string
		var score int
		var signals []string
		err := r.Scan(&v.ID, &v.ClaimID, &v.ClaimNo, &v.Type, &v.Status, &uid, &name, &v.CreatedAt, &v.DueAt, &v.CompletedAt, &score, &signals)
		if uid != nil && name != nil {
			v.AssignedTo = &User{*uid, *name}
		}
		v.Breached = workflow.Breached(v.Status, v.DueAt, v.CompletedAt, now)
		clock := now
		if !workflow.Open(v.Status) && v.CompletedAt != nil {
			clock = *v.CompletedAt
		}
		v.RemainingSeconds = workflow.RemainingSeconds(v.DueAt, clock)
		if v.Type == "FRAUD_REVIEW" {
			v.FraudScore = &score
			v.Signals = signals
		}
		return v, err
	})
}

type Workload struct {
	User
	Role string `json:"role"`
	Open int64  `json:"open_tasks"`
}

func (s *Service) Workload(ctx context.Context) ([]Workload, error) {
	rows, err := s.pool.Query(ctx, `
  SELECT u.id,u.full_name,u.role,count(t.id) FROM users u
   LEFT JOIN claim_tasks t ON t.assigned_to=u.id AND t.status IN ('OPEN','IN_PROGRESS')
   WHERE u.is_active AND u.role IN ('APPRAISER','CLAIMS_ADJUSTER')
   GROUP BY u.id ORDER BY u.role,u.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Workload, error) {
		var v Workload
		err := r.Scan(&v.ID, &v.FullName, &v.Role, &v.Open)
		return v, err
	})
}

type Summary struct {
	Type         string `json:"task_type"`
	Open         int64  `json:"open"`
	OpenBreached int64  `json:"open_breached"`
	Done         int64  `json:"done"`
	DoneLate     int64  `json:"done_late"`
	OnTimeBP     *int64 `json:"on_time_bp"`
}

func (s *Service) Summary(ctx context.Context) ([]Summary, error) {
	rows, err := s.pool.Query(ctx, `
  SELECT kinds.kind,
         count(t.id) FILTER (WHERE t.status IN ('OPEN','IN_PROGRESS')),
         count(t.id) FILTER (WHERE t.status IN ('OPEN','IN_PROGRESS') AND t.sla_due_at<$1),
         count(t.id) FILTER (WHERE t.status='DONE'),
         count(t.id) FILTER (WHERE t.status='DONE' AND t.completed_at>t.sla_due_at)
    FROM (VALUES ('APPRAISAL'),('ADJUSTMENT'),('FRAUD_REVIEW')) kinds(kind)
    LEFT JOIN claim_tasks t ON t.task_type=kinds.kind GROUP BY kinds.kind ORDER BY kinds.kind`, s.now())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Summary, error) {
		var v Summary
		err := r.Scan(&v.Type, &v.Open, &v.OpenBreached, &v.Done, &v.DoneLate)
		v.OnTimeBP = lossratio.LossRatioBP(v.Done-v.DoneLate, v.Done)
		return v, err
	})
}
