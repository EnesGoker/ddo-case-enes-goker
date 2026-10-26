// Package payment tahsilat, iade ve gün sonu uygulama işlemleridir.
package payment

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/billing"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment/store"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
)

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
	rate int64
}

func NewService(pool *pgxpool.Pool, now func() time.Time, rate int64) *Service {
	if rate <= 0 {
		rate = 1000
	}
	return &Service{pool: pool, now: now, rate: rate}
}

func (s *Service) target(ctx context.Context, q db.DBTX, id int64, lock bool) (policy.Header, error) {
	h, err := policy.LoadHeader(ctx, q, id, lock)
	if errors.Is(err, policy.ErrNotFound) {
		return h, httpx.NotFound("poliçe")
	}
	if err != nil {
		return h, err
	}
	p, _ := auth.FromContext(ctx)
	scope, err := access.Resolve(ctx, s.pool, p)
	if err != nil {
		return h, access.AsHTTP(err)
	}
	if !scope.Allows(h.AgentID, h.CustomerID) {
		return h, httpx.NotFound("poliçe")
	}
	return h, nil
}

func accrualDate(h policy.Header, at time.Time) time.Time {
	if h.CancelEffectiveDate != nil {
		cancel, _ := caldate.Parse(*h.CancelEffectiveDate)
		if cancel.Before(at) {
			return cancel
		}
	}
	return at
}

func concurrencyError(err error) error {
	if errors.Is(err, store.ErrRequestKeyMismatch) {
		return httpx.NewError(http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "anahtar farklı ödeme veya iade için kullanılmış")
	}
	err = idempotency.Conflict(err)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23514" && (pg.TableName == "installments" || pg.TableName == "late_interest_charges") {
		return httpx.NewError(http.StatusConflict, "CONCURRENT_PAYMENT", "ödenmiş tutar eşzamanlı değişti; güncel bakiyeyi okuyun")
	}
	return err
}

type PayRequest struct {
	AmountKurus *int64 `json:"amount_kurus"`
	Method      string `json:"method"`
	PaymentDate string `json:"payment_date"`
}

func (s *Service) Pay(ctx context.Context, id int64, req PayRequest) (store.Payment, error) {
	var out store.Payment
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if req.AmountKurus == nil || *req.AmountKurus <= 0 {
		return out, httpx.Invalid(map[string]string{"amount_kurus": "pozitif kuruş tutarı zorunlu"})
	}
	if req.Method != "CASH" && req.Method != "CARD" && req.Method != "TRANSFER" {
		return out, httpx.Invalid(map[string]string{"method": "CASH, CARD veya TRANSFER olmalı"})
	}
	p, _ := auth.FromContext(ctx)
	at := caldate.Today(s.now())
	if req.PaymentDate != "" {
		if p.Role != auth.RoleAccounting {
			return out, httpx.NewError(http.StatusForbidden, "PAYMENT_DATE_FORBIDDEN", "ödeme tarihini yalnızca muhasebe belirleyebilir")
		}
		var err error
		at, err = caldate.Parse(req.PaymentDate)
		if err != nil {
			return out, httpx.Invalid(map[string]string{"payment_date": err.Error()})
		}
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		h, err := s.target(ctx, tx, id, true)
		if err != nil {
			return err
		}

		previous, found, err := store.FindPayment(ctx, tx, h.ID, p.UserID, idempotency.Key(ctx), "PAYMENT", nil)
		if err != nil {
			return err
		}
		if found {
			if previous.AmountKurus != *req.AmountKurus {
				return httpx.NewError(http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "iş anahtarındaki ödeme tutarı farklı")
			}
			out = previous
			return nil
		}
		if at.After(caldate.Today(s.now())) {
			return httpx.NewError(http.StatusUnprocessableEntity, "PAYMENT_DATE_IN_FUTURE", "ödeme tarihi gelecekte olamaz")
		}
		var issued time.Time
		if err := tx.QueryRow(ctx, `SELECT created_at FROM policies WHERE id=$1`, id).Scan(&issued); err != nil {
			return err
		}
		start, _ := caldate.Parse(h.StartDate)
		minimum := caldate.Today(issued)
		if start.Before(minimum) {
			minimum = start
		}
		if at.Before(minimum) {
			return httpx.NewError(http.StatusUnprocessableEntity, "PAYMENT_DATE_BEFORE_POLICY", "ödeme tarihi poliçe başlangıcı ve düzenleme gününün en küçüğünden önce olamaz")
		}
		state, err := store.Load(ctx, tx, id)
		if err != nil {
			return err
		}
		if at.Before(store.LastAccrual(state)) {
			return httpx.NewError(http.StatusUnprocessableEntity, "PAYMENT_DATE_BEFORE_ACCRUAL", "ödeme tarihi son faiz tahakkukunun öncesinde olamaz")
		}
		if _, err := store.Accrue(ctx, tx, state, accrualDate(h, at), at, s.rate, p.UserID); err != nil {
			return err
		}
		state, err = store.Load(ctx, tx, id)
		if err != nil {
			return err
		}
		charges := make([]billing.Charge, len(state.Interest))
		for i, c := range state.Interest {
			charges[i] = c.Charge
		}
		allocations, err := billing.Allocate(state.Rows, charges, *req.AmountKurus)
		if errors.Is(err, billing.ErrOverpayment) {
			return httpx.NewError(http.StatusUnprocessableEntity, "OVERPAYMENT", err.Error())
		}
		if err != nil {
			return err
		}
		out, err = store.InsertPayment(ctx, tx, id, "PAYMENT", *req.AmountKurus, req.Method, nil, at, p.UserID, "Prim tahsilatı", idempotency.Key(ctx))
		if err != nil {
			return err
		}
		for _, a := range allocations {
			if err := store.ApplyAllocation(ctx, tx, out.ID, a, false); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "PAYMENT_RECEIVED", EntityType: "payment", EntityID: out.ID, New: out})
	})
	return out, concurrencyError(err)
}

type RefundRequest struct {
	AmountKurus *int64 `json:"amount_kurus"`
	Reason      string `json:"reason"`
}

func (s *Service) Refund(ctx context.Context, id int64, req RefundRequest) (store.Payment, error) {
	var out store.Payment
	if req.AmountKurus == nil || *req.AmountKurus <= 0 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		return out, httpx.Invalid(map[string]string{"refund": "pozitif tutar ve 1–1000 karakter gerekçe gerekli"})
	}
	var policyID int64
	err := s.pool.QueryRow(ctx, `
  SELECT policy_id FROM payments WHERE id = $1 AND kind = 'PAYMENT'`, id).Scan(&policyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, httpx.NotFound("ödeme")
	}
	if err != nil {
		return out, err
	}
	p, _ := auth.FromContext(ctx)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		h, err := s.target(ctx, tx, policyID, true)
		if err != nil {
			return err
		}

		previous, found, err := store.FindPayment(ctx, tx, h.ID, p.UserID, idempotency.Key(ctx), "REFUND", &id)
		if err != nil {
			return err
		}
		if found {
			if previous.AmountKurus != *req.AmountKurus {
				return httpx.NewError(http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "iş anahtarındaki iade tutarı farklı")
			}
			out = previous
			return nil
		}
		if h.Status != "ACTIVE" {
			return httpx.NewError(http.StatusConflict, "REFUND_REQUIRES_ACTIVE_POLICY", "manuel iade yalnızca ACTIVE poliçede yapılabilir")
		}
		var remaining int64
		var method string
		err = tx.QueryRow(ctx, `
   SELECT p.method, p.amount_kurus - coalesce(sum(r.amount_kurus),0)
     FROM payments p LEFT JOIN payments r ON r.refund_of_payment_id = p.id
    WHERE p.id = $1 GROUP BY p.id`, id).Scan(&method, &remaining)
		if err != nil {
			return err
		}
		if *req.AmountKurus > remaining {
			return httpx.NewError(http.StatusUnprocessableEntity, "REFUND_EXCEEDS_PAYMENT", "iade ödemenin kalanını aşıyor")
		}
		state, err := store.Load(ctx, tx, policyID)
		if err != nil {
			return err
		}
		at := caldate.Today(s.now())
		// Önce mevcut anaparanın faizi kapanış tarihine kadar hesaplanır; iade geçmişi değiştirmez.
		if _, err := store.Accrue(ctx, tx, state, at, at, s.rate, p.UserID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
   SELECT a.installment_id, a.late_interest_charge_id,
          a.amount_kurus - coalesce((
           SELECT sum(ra.amount_kurus)
             FROM payment_allocations ra JOIN payments rp ON rp.id = ra.payment_id
            WHERE rp.refund_of_payment_id = $1
              AND ra.installment_id IS NOT DISTINCT FROM a.installment_id
              AND ra.late_interest_charge_id IS NOT DISTINCT FROM a.late_interest_charge_id
          ),0)
     FROM payment_allocations a WHERE a.payment_id = $1 ORDER BY a.id DESC`, id)
		if err != nil {
			return err
		}
		allocations, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (billing.Allocation, error) {
			var installment, charge *int64
			var amount int64
			err := r.Scan(&installment, &charge, &amount)
			a := billing.Allocation{Amount: amount}
			if installment != nil {
				a.InstallmentID = *installment
			}
			if charge != nil {
				a.ChargeID = *charge
			}
			return a, err
		})
		if err != nil {
			return err
		}
		out, err = store.InsertPayment(ctx, tx, policyID, "REFUND", *req.AmountKurus, method, &id, at, p.UserID, req.Reason, idempotency.Key(ctx))
		if err != nil {
			return err
		}
		left := *req.AmountKurus
		for _, a := range allocations {
			a.Amount = min(a.Amount, left)
			if a.Amount <= 0 {
				continue
			}
			if err := store.ApplyAllocation(ctx, tx, out.ID, a, true); err != nil {
				return err
			}
			left -= a.Amount
		}
		if left != 0 {
			return httpx.NewError(http.StatusConflict, "REFUND_ALLOCATION_MISSING", "geri alınabilir tahsis yetersiz")
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "PAYMENT_REFUNDED", EntityType: "payment", EntityID: out.ID, New: out})
	})
	return out, concurrencyError(err)
}
