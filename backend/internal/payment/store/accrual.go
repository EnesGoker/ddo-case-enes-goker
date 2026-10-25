package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/billing"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
)

type Accrued struct {
	Count       int   `json:"count"`
	AmountKurus int64 `json:"amount_kurus"`
}

func Accrue(ctx context.Context, tx pgx.Tx, s State, asOf, journalDate time.Time, rate, actor int64) (Accrued, error) {
	var out Accrued
	for _, r := range s.Rows {
		if !billing.Open(r) || r.Paid >= r.Amount {
			continue
		}
		from := r.Due
		for _, c := range s.Interest {
			if c.InstallmentID == r.ID && c.To.After(from) {
				from = c.To
			}
		}
		// İade ile yeniden açılan anapara geçmişe dönük faiz yaratmaz.
		var reopened *time.Time
		err := tx.QueryRow(ctx, `
   SELECT max(p.paid_at::date)
     FROM payment_allocations a
     JOIN payments p ON p.id = a.payment_id
    WHERE a.installment_id = $1 AND p.kind = 'REFUND'`, r.ID).Scan(&reopened)
		if err != nil {
			return out, err
		}
		if reopened != nil && reopened.After(from) {
			from = *reopened
		}
		if !asOf.After(from) {
			continue
		}
		amount, err := billing.InterestAmount(r.Amount-r.Paid, rate, proration.DaysBetween(from, asOf))
		if err != nil {
			return out, err
		}
		if amount == 0 {
			continue
		}
		var id int64
		err = tx.QueryRow(ctx, `
   INSERT INTO late_interest_charges
          (installment_id, accrued_from, accrued_to, overdue_principal_kurus, daily_rate_ppm, amount_kurus)
   VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, r.ID, from, asOf, r.Amount-r.Paid, rate, amount).Scan(&id)
		if err != nil {
			return out, err
		}
		_, err = ledger.Post(ctx, tx, ledger.Entry{Date: caldate.Of(journalDate), Description: fmt.Sprintf("Gecikme faizi [%s,%s)", caldate.Format(from), caldate.Format(asOf)), RefType: "LATE_INTEREST", RefID: id, CreatedBy: actor}, []ledger.Line{
			{AccountCode: "1200", DebitKurus: amount},
			{AccountCode: "4100", CreditKurus: amount},
		})
		if err != nil {
			return out, err
		}
		out.Count++
		out.AmountKurus += amount
	}
	return out, nil
}
