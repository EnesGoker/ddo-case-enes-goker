package store

import (
	"context"
	"errors"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/jackc/pgx/v5"
)

var ErrRequestKeyMismatch = errors.New("iş anahtarı farklı ödeme veya iade için kullanılmış")

func FindPayment(ctx context.Context, q db.DBTX, policyID, actor int64, key *string, kind string, refundOf *int64) (Payment, bool, error) {
	var p Payment
	if key == nil {
		return p, false, nil
	}
	err := q.QueryRow(ctx, `
  SELECT id,policy_id,kind,amount_kurus,method,refund_of_payment_id,paid_at,journal_entry_id,coalesce(reference,'')
    FROM payments WHERE policy_id=$1 AND created_by=$2 AND request_key=$3`, policyID, actor, key).Scan(&p.ID, &p.PolicyID, &p.Kind, &p.AmountKurus, &p.Method, &p.RefundOf, &p.PaidAt, &p.JournalEntryID, &p.Reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, false, nil
	}
	if err == nil {
		sameRefund := p.RefundOf == nil && refundOf == nil || p.RefundOf != nil && refundOf != nil && *p.RefundOf == *refundOf
		if p.Kind != kind || !sameRefund {
			return Payment{}, false, ErrRequestKeyMismatch
		}
	}
	return p, err == nil, err
}
