package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/billing"
	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
)

type Payment struct {
	ID             int64     `json:"id"`
	PolicyID       int64     `json:"policy_id"`
	Kind           string    `json:"kind"`
	AmountKurus    int64     `json:"amount_kurus"`
	Method         string    `json:"method"`
	RefundOf       *int64    `json:"refund_of_payment_id"`
	PaidAt         time.Time `json:"paid_at"`
	JournalEntryID int64     `json:"journal_entry_id"`
	Reference      string    `json:"reference"`
}

func InsertPayment(ctx context.Context, tx pgx.Tx, policyID int64, kind string, amount int64, method string, refundOf *int64, at time.Time, actor int64, reference string) (Payment, error) {
	p := Payment{PolicyID: policyID, Kind: kind, AmountKurus: amount, Method: method, RefundOf: refundOf, PaidAt: at, Reference: reference}
	err := tx.QueryRow(ctx, `
  INSERT INTO payments
         (policy_id, kind, amount_kurus, method, refund_of_payment_id, paid_at, created_by, reference)
  VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, policyID, kind, amount, method, refundOf, at, actor, reference).Scan(&p.ID)
	if err != nil {
		return p, err
	}
	lines := []ledger.Line{{AccountCode: "1000", DebitKurus: amount}, {AccountCode: "1200", CreditKurus: amount}}
	if kind == "REFUND" {
		lines = []ledger.Line{{AccountCode: "1200", DebitKurus: amount}, {AccountCode: "1000", CreditKurus: amount}}
	}
	p.JournalEntryID, err = ledger.Post(ctx, tx, ledger.Entry{Date: at, Description: reference, RefType: kind, RefID: p.ID, CreatedBy: actor}, lines)
	if err != nil {
		return p, err
	}
	_, err = tx.Exec(ctx, `
  UPDATE payments SET journal_entry_id = $2 WHERE id = $1`, p.ID, p.JournalEntryID)
	return p, err
}

// ApplyAllocation para yönünü kind belirlediği için tahsis tutarı daima pozitiftir.
func ApplyAllocation(ctx context.Context, tx pgx.Tx, paymentID int64, a billing.Allocation, refund bool) error {
	delta := a.Amount
	if refund {
		delta = -delta
	}
	var installment, charge *int64
	if a.ChargeID != 0 {
		charge = &a.ChargeID
		if _, err := tx.Exec(ctx, `
   UPDATE late_interest_charges SET paid_kurus = paid_kurus + $2
    WHERE id = $1`, a.ChargeID, delta); err != nil {
			return err
		}
	} else {
		installment = &a.InstallmentID
		if _, err := tx.Exec(ctx, `
   UPDATE installments SET paid_kurus = paid_kurus + $2,
          status = CASE WHEN paid_kurus + $2 = amount_kurus THEN 'PAID'
                        WHEN paid_kurus + $2 = 0 THEN 'PENDING'
                        ELSE 'PARTIAL' END
    WHERE id = $1`, a.InstallmentID, delta); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `
  INSERT INTO payment_allocations
         (payment_id, installment_id, late_interest_charge_id, amount_kurus)
  VALUES ($1,$2,$3,$4)`, paymentID, installment, charge, a.Amount)
	return err
}
