package payment

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

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

type RunResult struct {
	AsOf          string    `json:"as_of"`
	AccrualCount  int       `json:"accrual_count"`
	InterestKurus int64     `json:"interest_kurus"`
	Cancelled     []int64   `json:"cancelled"`
	Skipped       []Skipped `json:"skipped"`
}

func (s *Service) Run(ctx context.Context, date string) (RunResult, error) {
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
	out := RunResult{AsOf: caldate.Format(asOf), Cancelled: []int64{}, Skipped: []Skipped{}}
	rows, err := s.pool.Query(ctx, `
  SELECT DISTINCT p.id
    FROM policies p JOIN installment_plans pl ON pl.policy_id = p.id
    JOIN installments i ON i.plan_id = pl.id
   WHERE pl.status = 'ACTIVE' AND i.status IN ('PENDING','PARTIAL')
   ORDER BY p.id`)
	if err != nil {
		return out, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return out, err
	}
	actor, _ := auth.FromContext(ctx)
	for _, id := range ids {
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
			return audit.Write(ctx, tx, audit.Entry{Action: "BILLING_RUN", EntityType: "policy", EntityID: id, New: map[string]any{
				"as_of": out.AsOf, "accrual": accrued, "cancelled": cancelled, "skipped": reason,
			}})
		})
		if err != nil {
			return out, err
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
	return out, nil
}
