package payment

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/billing"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment/store"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
)

type Skipped struct {
	PolicyID int64  `json:"policy_id"`
	Reason   string `json:"reason"`
}

type Failed struct {
	PolicyID int64  `json:"policy_id"`
	Code     string `json:"code"`
}

type RunOptions struct {
	AfterPolicyID int64 `json:"after_policy_id"`
	Limit         int   `json:"limit"`
}

type RunResult struct {
	NextAfterPolicyID int64     `json:"next_after_policy_id"`
	Done              bool      `json:"done"`
	Failed            []Failed  `json:"failed"`
	AsOf              string    `json:"as_of"`
	AccrualCount      int       `json:"accrual_count"`
	InterestKurus     int64     `json:"interest_kurus"`
	Cancelled         []int64   `json:"cancelled"`
	Skipped           []Skipped `json:"skipped"`
}

func (s *Service) Run(ctx context.Context, date string, options ...RunOptions) (RunResult, error) {
	started := time.Now() // Gerçek duvar saati bütçesi; tarihsel iş saatinden bağımsız.
	opts := RunOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.AfterPolicyID < 0 || opts.Limit < 0 || opts.Limit > 2000 {
		return RunResult{}, httpx.Invalid(map[string]string{"pagination": "after_policy_id >= 0; limit 1–2000 olmalı"})
	}
	if opts.Limit == 0 {
		opts.Limit = 500
	}
	asOf := caldate.Today(s.now())
	if date != "" {
		parsed, err := caldate.Parse(date)
		if err != nil {
			return RunResult{}, httpx.Invalid(map[string]string{"as_of": err.Error()})
		}
		asOf = parsed
	}
	if asOf.After(caldate.Today(s.now())) {
		return RunResult{}, httpx.NewError(http.StatusUnprocessableEntity, "AS_OF_IN_FUTURE", "gün sonu tarihi gelecekte olamaz")
	}
	out := RunResult{AsOf: caldate.Format(asOf), Cancelled: []int64{}, Skipped: []Skipped{}, Failed: []Failed{}, NextAfterPolicyID: opts.AfterPolicyID}
	rows, err := s.pool.Query(ctx, `
  SELECT policy_id FROM (
    SELECT pl.policy_id
      FROM installments i JOIN installment_plans pl ON pl.id = i.plan_id
     WHERE i.status IN ('PENDING','PARTIAL') AND i.due_date < $1
       AND pl.status = 'ACTIVE' AND pl.policy_id > $2
    UNION
    SELECT pl.policy_id
      FROM late_interest_charges c JOIN installments i ON i.id = c.installment_id
      JOIN installment_plans pl ON pl.id = i.plan_id
     WHERE c.paid_kurus < c.amount_kurus AND pl.status = 'ACTIVE' AND pl.policy_id > $2
  ) candidates ORDER BY policy_id LIMIT $3`, asOf, opts.AfterPolicyID, opts.Limit+1)
	if err != nil {
		return out, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return out, err
	}
	actor, _ := auth.FromContext(ctx)
	visited := 0
	for _, id := range ids {
		if visited == opts.Limit || time.Since(started) >= 8*time.Second {
			break
		}
		visited++
		out.NextAfterPolicyID = id
		var accrued store.Accrued
		var cancelled bool
		reason := ""
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			var locked int64
			err := tx.QueryRow(ctx, `
    SELECT id FROM policies WHERE id = $1 FOR UPDATE SKIP LOCKED`, id).Scan(&locked)
			if errors.Is(err, pgx.ErrNoRows) {
				reason = "LOCKED"
				return nil
			}
			if err != nil {
				return err
			}
			h, err := policy.LoadHeader(ctx, tx, id, true)
			if err != nil {
				return err
			}
			state, err := store.Load(ctx, tx, id)
			if err != nil {
				return err
			}
			accrued, err = store.Accrue(ctx, tx, state, accrualDate(h, asOf), asOf, s.rate, actor.UserID)
			if err != nil {
				return err
			}
			if h.Status == "ACTIVE" && billing.Overdue30(state.Rows, asOf) {
				_, err = policy.NewChangeService(s.pool, s.now, s.rate).CancelTx(ctx, tx, id, policy.CancelRequest{
					EffectiveDate: caldate.Format(asOf), Reason: "NON_PAYMENT", Note: "30 gün ödenmeyen taksit: gün sonu",
				})
				if err != nil {
					var ae *httpx.AppError
					if errors.As(err, &ae) && (ae.Code == "CLAIMS_AFTER_EFFECTIVE_DATE" || ae.Code == "INVALID_EFFECTIVE_DATE") {
						reason = ae.Code
					} else {
						return err
					}
				} else {
					cancelled = true
				}
			}
			if accrued.Count == 0 && !cancelled {
				return nil
			}
			return audit.Write(ctx, tx, audit.Entry{Action: "BILLING_RUN", EntityType: "policy", EntityID: id, New: map[string]any{
				"as_of": out.AsOf, "accrual": accrued, "cancelled": cancelled, "skipped": reason,
			}})
		})
		if err != nil {
			if runSystemError(ctx, err) {
				return out, err
			}
			code := "POLICY_ERROR"
			var ae *httpx.AppError
			var pe *pgconn.PgError
			if errors.As(err, &ae) {
				code = ae.Code
			} else if errors.As(err, &pe) {
				code = "DATABASE_" + pe.Code
			}
			out.Failed = append(out.Failed, Failed{PolicyID: id, Code: code})
			continue
		}
		out.AccrualCount += accrued.Count
		out.InterestKurus += accrued.AmountKurus
		if cancelled {
			out.Cancelled = append(out.Cancelled, id)
		}
		if reason != "" {
			out.Skipped = append(out.Skipped, Skipped{PolicyID: id, Reason: reason})
		}
	}
	out.Done = visited == len(ids)
	return out, nil
}

func runSystemError(ctx context.Context, err error) bool {
	var network net.Error
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) || pgconn.SafeToRetry(err) || errors.As(err, &network) {
		return true
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		for _, class := range []string{"08", "53", "57", "58", "XX"} {
			if strings.HasPrefix(pe.Code, class) {
				return true
			}
		}
	}
	return false
}
