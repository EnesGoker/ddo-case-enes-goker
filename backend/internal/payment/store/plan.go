// Package store faturalama verisini çağıranın transaction'ında yönetir.
// Yazma girişleri çağıranın poliçe satırını önceden kilitlemesini gerektirir.
// policy paketini içe aktarmaz; böylece policy → store → billing/ledger yönü döngüsüzdür.
package store

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/billing"
	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/clock"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

type Interest struct {
	billing.Charge
	From      time.Time
	To        time.Time
	Principal int64
	Rate      int64
}

type State struct {
	ID       int64
	PolicyID int64
	Total    int64
	Count    int
	Status   string
	Rows     []billing.Installment
	Interest []Interest
}

func Load(ctx context.Context, q db.DBTX, policyID int64) (State, error) {
	s := State{PolicyID: policyID, Rows: []billing.Installment{}, Interest: []Interest{}}
	err := q.QueryRow(ctx, `
  SELECT id, total_kurus, installment_count, status
    FROM installment_plans
   WHERE policy_id = $1 AND status = 'ACTIVE'`, policyID).Scan(&s.ID, &s.Total, &s.Count, &s.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, httpx.NewError(http.StatusConflict, "BILLING_PLAN_MISSING", "ödeme planı yok; A5 öncesi demo için reset-demo gerekli")
	}
	if err != nil {
		return s, err
	}
	rows, err := q.Query(ctx, `
  SELECT id, seq, due_date, amount_kurus, paid_kurus, status
    FROM installments WHERE plan_id = $1
   ORDER BY due_date, seq`, s.ID)
	if err != nil {
		return s, err
	}
	s.Rows, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (billing.Installment, error) {
		var it billing.Installment
		err := r.Scan(&it.ID, &it.Seq, &it.Due, &it.Amount, &it.Paid, &it.Status)
		return it, err
	})
	if err != nil {
		return s, err
	}
	rows, err = q.Query(ctx, `
  SELECT c.id, c.installment_id, c.amount_kurus, c.paid_kurus,
         c.accrued_from, c.accrued_to, c.overdue_principal_kurus, c.daily_rate_ppm
    FROM late_interest_charges c
    JOIN installments i ON i.id = c.installment_id
   WHERE i.plan_id = $1 ORDER BY c.id`, s.ID)
	if err != nil {
		return s, err
	}
	s.Interest, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Interest, error) {
		var c Interest
		err := r.Scan(&c.ID, &c.InstallmentID, &c.Amount, &c.Paid, &c.From, &c.To, &c.Principal, &c.Rate)
		return c, err
	})
	return s, err
}

func Create(ctx context.Context, tx pgx.Tx, policyID, total int64, count int, start time.Time, actor int64) error {
	rows, err := billing.Split(total, count, start)
	if err != nil {
		return httpx.Invalid(map[string]string{"installment_count": err.Error()})
	}
	var planID int64
	err = tx.QueryRow(ctx, `
  INSERT INTO installment_plans (policy_id, total_kurus, installment_count, created_at)
  VALUES ($1,$2,$3,$4) RETURNING id`, policyID, total, count, clock.Now(ctx)).Scan(&planID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.Exec(ctx, `
   INSERT INTO installments (plan_id, seq, due_date, amount_kurus)
   VALUES ($1,$2,$3,$4)`, planID, r.Seq, r.Due, r.Amount); err != nil {
			return err
		}
	}
	_, err = ledger.Post(ctx, tx, ledger.Entry{Date: start, Description: "Poliçe prim tahakkuku", RefType: "POLICY_ISSUE", RefID: policyID, CreatedBy: actor}, []ledger.Line{
		{AccountCode: "1200", DebitKurus: total},
		{AccountCode: "4000", CreditKurus: total},
	})
	return err
}

func OpenBalance(s State) int64 {
	var total int64
	for _, r := range s.Rows {
		if billing.Open(r) {
			total += r.Amount - r.Paid
		}
	}
	for _, c := range s.Interest {
		total += c.Amount - c.Paid
	}
	return total
}

func LastAccrual(s State) time.Time {
	var last time.Time
	for _, c := range s.Interest {
		if c.To.After(last) {
			last = c.To
		}
	}
	return last
}
