package payment

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/billing"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment/store"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
)

type InstallmentView struct {
	ID          int64  `json:"id"`
	Seq         int    `json:"seq"`
	DueDate     string `json:"due_date"`
	AmountKurus int64  `json:"amount_kurus"`
	PaidKurus   int64  `json:"paid_kurus"`
	Status      string `json:"status"`
	OverdueDays int64  `json:"overdue_days"`
}

type InterestView struct {
	ID             int64  `json:"id"`
	InstallmentID  int64  `json:"installment_id"`
	From           string `json:"accrued_from"`
	To             string `json:"accrued_to"`
	PrincipalKurus int64  `json:"overdue_principal_kurus"`
	Rate           int64  `json:"daily_rate_ppm"`
	AmountKurus    int64  `json:"amount_kurus"`
	PaidKurus      int64  `json:"paid_kurus"`
}

type AllocationView struct {
	ID            int64  `json:"id"`
	PaymentID     int64  `json:"payment_id"`
	InstallmentID *int64 `json:"installment_id"`
	InterestID    *int64 `json:"late_interest_charge_id"`
	AmountKurus   int64  `json:"amount_kurus"`
}

type PaymentView struct {
	store.Payment
	RefundableKurus int64 `json:"refundable_kurus"`
}

type BillingView struct {
	PolicyID         int64             `json:"policy_id"`
	PolicyStatus     string            `json:"policy_status"`
	PlanID           int64             `json:"plan_id"`
	TotalKurus       int64             `json:"total_kurus"`
	InstallmentCount int               `json:"installment_count"`
	OpenBalanceKurus int64             `json:"open_balance_kurus"`
	DailyRatePPM     int64             `json:"daily_rate_ppm"`
	Installments     []InstallmentView `json:"installments"`
	Interest         []InterestView    `json:"interest"`
	Payments         []PaymentView     `json:"payments"`
	Allocations      []AllocationView  `json:"allocations"`
}

func (s *Service) Billing(ctx context.Context, id int64) (BillingView, error) {
	var out BillingView
	// Başlık ve mali görünüm aynı anlık görüntüden okunur; diğer faturalama girişleriyle aynı kilit.
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		h, err := s.target(ctx, tx, id, true)
		if err != nil {
			return err
		}
		state, err := store.Load(ctx, tx, id)
		if err != nil {
			return err
		}
		out = BillingView{PolicyID: id, PolicyStatus: h.Status, PlanID: state.ID, TotalKurus: state.Total,
			InstallmentCount: state.Count, OpenBalanceKurus: store.OpenBalance(state), DailyRatePPM: s.rate,
			Installments: []InstallmentView{}, Interest: []InterestView{}, Payments: []PaymentView{}, Allocations: []AllocationView{},
		}
		for _, r := range state.Rows {
			days := int64(0)
			if billing.Open(r) {
				days = max(int64(0), proration.DaysBetween(r.Due, caldate.Today(s.now())))
			}
			out.Installments = append(out.Installments, InstallmentView{r.ID, r.Seq, caldate.Format(r.Due), r.Amount, r.Paid, r.Status, days})
		}
		for _, c := range state.Interest {
			out.Interest = append(out.Interest, InterestView{c.ID, c.InstallmentID, caldate.Format(c.From), caldate.Format(c.To), c.Principal, c.Rate, c.Amount, c.Paid})
		}
		rows, err := tx.Query(ctx, `
   SELECT p.id, p.policy_id, p.kind, p.amount_kurus, p.method, p.refund_of_payment_id,
          p.paid_at, p.journal_entry_id, coalesce(p.reference,''),
          CASE WHEN p.kind = 'PAYMENT' THEN p.amount_kurus - coalesce((
           SELECT sum(r.amount_kurus) FROM payments r WHERE r.refund_of_payment_id = p.id
          ),0) ELSE 0 END
     FROM payments p WHERE p.policy_id = $1 ORDER BY p.paid_at DESC, p.id DESC`, id)
		if err != nil {
			return err
		}
		out.Payments, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (PaymentView, error) {
			var p PaymentView
			err := r.Scan(&p.ID, &p.PolicyID, &p.Kind, &p.AmountKurus, &p.Method, &p.RefundOf, &p.PaidAt, &p.JournalEntryID, &p.Reference, &p.RefundableKurus)
			return p, err
		})
		if err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `
   SELECT a.id, a.payment_id, a.installment_id, a.late_interest_charge_id, a.amount_kurus
     FROM payment_allocations a JOIN payments p ON p.id = a.payment_id
    WHERE p.policy_id = $1 ORDER BY a.id`, id)
		if err != nil {
			return err
		}
		out.Allocations, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AllocationView, error) {
			var a AllocationView
			err := r.Scan(&a.ID, &a.PaymentID, &a.InstallmentID, &a.InterestID, &a.AmountKurus)
			return a, err
		})
		return err
	})
	return out, err
}
