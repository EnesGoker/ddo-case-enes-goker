package store

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/billing"
	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

func PreviewAdjustment(ctx context.Context, q db.DBTX, policyID, delta int64, effective time.Time) (billing.Adjustment, error) {
	s, err := Load(ctx, q, policyID)
	if err != nil {
		return billing.Adjustment{}, err
	}
	return billing.Adjust(s.Rows, delta, effective)
}

func Adjust(ctx context.Context, tx pgx.Tx, policyID, delta int64, effective time.Time, endorsementID, actor int64) (int64, error) {
	s, err := Load(ctx, tx, policyID)
	if err != nil {
		return 0, err
	}
	result, err := billing.Adjust(s.Rows, delta, effective)
	if err != nil {
		return 0, err
	}
	for _, r := range result.Rows {
		if r.ID == 0 {
			_, err = tx.Exec(ctx, `
    INSERT INTO installments (plan_id, seq, due_date, amount_kurus)
    VALUES ($1,$2,$3,$4)`, s.ID, r.Seq, r.Due, r.Amount)
		} else {
			_, err = tx.Exec(ctx, `
    UPDATE installments SET amount_kurus = $2, status = $3
     WHERE id = $1 AND (amount_kurus <> $2 OR status <> $3)`, r.ID, r.Amount, r.Status)
		}
		if err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `
  UPDATE installment_plans SET total_kurus = $2, endorsement_id = $3
   WHERE id = $1`, s.ID, result.Total, endorsementID); err != nil {
		return 0, err
	}
	if delta != 0 {
		lines := []ledger.Line{{AccountCode: "1200", DebitKurus: delta}, {AccountCode: "4000", CreditKurus: delta}}
		if delta < 0 {
			lines = []ledger.Line{{AccountCode: "4000", DebitKurus: -delta}, {AccountCode: "1200", CreditKurus: -delta}}
		}
		if _, err := ledger.Post(ctx, tx, ledger.Entry{Date: effective, Description: "Zeyil prim ayarı", RefType: "ENDORSEMENT", RefID: endorsementID, CreatedBy: actor}, lines); err != nil {
			return 0, err
		}
	}
	if err := CashRefund(ctx, tx, policyID, result.CashRefund, effective, actor); err != nil {
		return 0, err
	}
	return result.CashRefund, nil
}

// CashRefund prim azalışının açık anaparayı aşan kısmını tahsissiz nakit iadeye çevirir.
func CashRefund(ctx context.Context, tx pgx.Tx, policyID, amount int64, at time.Time, actor int64) error {
	if amount == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
  SELECT p.id, p.method, p.amount_kurus - coalesce(sum(r.amount_kurus),0)
    FROM payments p
    LEFT JOIN payments r ON r.refund_of_payment_id = p.id
   WHERE p.policy_id = $1 AND p.kind = 'PAYMENT'
   GROUP BY p.id ORDER BY p.paid_at DESC, p.id DESC`, policyID)
	if err != nil {
		return err
	}
	type refundable struct {
		ID        int64
		Method    string
		Remaining int64
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (refundable, error) {
		var p refundable
		err := r.Scan(&p.ID, &p.Method, &p.Remaining)
		return p, err
	})
	if err != nil {
		return err
	}
	remaining := amount
	for _, p := range list {
		part := min(remaining, p.Remaining)
		if part <= 0 {
			continue
		}
		if _, err := InsertPayment(ctx, tx, policyID, "REFUND", part, p.Method, &p.ID, at, actor, "Prim azalışından nakit iade"); err != nil {
			return err
		}
		remaining -= part
		if remaining == 0 {
			return nil
		}
	}
	return httpx.NewError(http.StatusConflict, "REFUND_BALANCE_INSUFFICIENT", "nakit iade için tahsilat bakiyesi yetersiz")
}
