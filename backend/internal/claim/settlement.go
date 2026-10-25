package claim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/settlement"
)

type balance struct {
	ID          int64
	Code        *string
	Limit, Used int64
}

// Claim kilidinden sonra bakiyeler her zaman id artan sırada kilitlenir.
func calculate(ctx context.Context, q db.DBTX, c Claim, lock bool) (settlement.Result, []balance, error) {
	var in settlement.Input
	err := q.QueryRow(ctx, `SELECT a.damage_amount_kurus,pc.deductible_type,pc.deductible_value FROM appraisals a JOIN policy_version_coverages pc ON pc.policy_version_id=$2 AND pc.coverage_code=$3 WHERE a.claim_id=$1 ORDER BY a.id DESC LIMIT 1`, c.ID, c.PolicyVersionID, c.CoverageCode).Scan(&in.GrossDamageKurus, &in.DeductibleType, &in.DeductibleValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return settlement.Result{}, nil, httpx.NewError(422, "APPRAISAL_REQUIRED", "tazminat için ekspertiz gerekli")
	}
	if err != nil {
		return settlement.Result{}, nil, err
	}
	sql := `SELECT id,coverage_code,limit_kurus,used_kurus FROM policy_limit_balances WHERE policy_id=$1 AND (coverage_code=$2 OR coverage_code IS NULL) AND period_start <= $3::date AND period_end > $3::date ORDER BY id`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, sql, c.PolicyID, c.CoverageCode, c.LossDate)
	if err != nil {
		return settlement.Result{}, nil, err
	}
	balances, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (balance, error) {
		var b balance
		err := r.Scan(&b.ID, &b.Code, &b.Limit, &b.Used)
		return b, err
	})
	if err != nil {
		return settlement.Result{}, nil, err
	}
	var versionLimit int64
	if err := q.QueryRow(ctx, `SELECT limit_kurus FROM policy_version_coverages WHERE policy_version_id=$1 AND coverage_code=$2`, c.PolicyVersionID, c.CoverageCode).Scan(&versionLimit); err != nil {
		return settlement.Result{}, nil, err
	}
	found := false
	for _, b := range balances {
		remaining := b.Limit - b.Used
		if b.Code == nil {
			v := remaining
			in.AnnualRemainingKurus = &v
		} else {
			found = true
			in.CoverageRemainingKurus = max(int64(0), min(b.Limit, versionLimit)-b.Used)
		}
	}
	if !found {
		return settlement.Result{}, nil, httpx.NewError(422, "LIMIT_BALANCE_MISSING", "teminat limit bakiyesi bulunamadı")
	}
	r, err := settlement.Calculate(in)
	return r, balances, err
}
func (s *Service) Preview(ctx context.Context, id int64) (settlement.Result, error) {
	c, err := s.visible(ctx, s.pool, id, false)
	if err != nil {
		return settlement.Result{}, err
	}
	if c.Status != "ADJUSTMENT" {
		return settlement.Result{}, httpx.NewError(409, "INVALID_CLAIM_STATUS", "tazminat için dosya ADJUSTMENT durumunda olmalı")
	}
	r, _, err := calculate(ctx, s.pool, c, false)
	return r, err
}
func (s *Service) Approve(ctx context.Context, id int64) (*Settlement, error) {
	var out *Settlement
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		c, err := s.visible(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if c.Status != "ADJUSTMENT" {
			return httpx.NewError(409, "INVALID_CLAIM_STATUS", "onay için dosya ADJUSTMENT durumunda olmalı")
		}
		r, balances, err := calculate(ctx, tx, c, true)
		if err != nil {
			return err
		}
		if r.PayableKurus == 0 {
			return httpx.NewError(422, "LIMIT_EXHAUSTED", "muafiyet/limit sonrası ödenecek tutar sıfır")
		}
		for _, b := range balances {
			tag, err := tx.Exec(ctx, `UPDATE policy_limit_balances SET used_kurus=used_kurus+$2 WHERE id=$1 AND used_kurus+$2<=limit_kurus`, b.ID, r.PayableKurus)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return httpx.NewError(http.StatusConflict, "LIMIT_CHANGED", "limit eşzamanlı değişti; yeniden önizleyin")
			}
		}
		steps, err := json.Marshal(r.CalcSteps)
		if err != nil {
			return err
		}
		p, _ := auth.FromContext(ctx)
		var sid int64
		if err := tx.QueryRow(ctx, `INSERT INTO claim_settlements (claim_id,gross_damage_kurus,deductible_kurus,payable_kurus,calc_steps,approved_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, id, r.GrossDamageKurus, r.DeductibleKurus, r.PayableKurus, steps, p.UserID).Scan(&sid); err != nil {
			return err
		}
		for _, b := range balances {
			if _, err := tx.Exec(ctx, `INSERT INTO limit_consumptions (settlement_id,balance_id,amount_kurus) VALUES($1,$2,$3)`, sid, b.ID, r.PayableKurus); err != nil {
				return err
			}
		}
		if _, err := Transition(ctx, tx, id, "APPROVED", p, "Tazminat onaylandı", c.RowVersion); err != nil {
			return err
		}
		jid, err := ledger.Post(ctx, tx, ledger.Entry{Date: caldate.Today(s.now()), Description: "Tazminat onayı " + c.ClaimNo, RefType: "CLAIM_APPROVAL", RefID: id, CreatedBy: p.UserID}, []ledger.Line{{AccountCode: "5000", DebitKurus: r.PayableKurus}, {AccountCode: "2000", CreditKurus: r.PayableKurus}})
		if err != nil {
			return err
		}
		out, err = loadSettlement(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "CLAIM_SETTLEMENT_APPROVED", EntityType: "claim", EntityID: id, New: map[string]any{"settlement": out, "journal_entry_id": jid}})
	})
	if db.Violates(err, "claim_settlements_one_active_uq", "journal_entries_one_per_event_uq") {
		return nil, httpx.NewError(409, "CLAIM_ALREADY_SETTLED", "hasar için mali kayıt zaten var")
	}
	return out, err
}
func (s *Service) Pay(ctx context.Context, id int64) (*Settlement, error) {
	var out *Settlement
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		c, err := s.visible(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if c.Status != "APPROVED" {
			return httpx.NewError(409, "INVALID_CLAIM_STATUS", "ödeme için dosya APPROVED durumunda olmalı")
		}
		out, err = loadSettlement(ctx, tx, id)
		if err != nil {
			return err
		}
		if out == nil || out.Status != "APPROVED" {
			return httpx.NewError(409, "INVALID_SETTLEMENT_STATUS", "onaylı tazminat bulunamadı")
		}
		p, _ := auth.FromContext(ctx)
		jid, err := ledger.Post(ctx, tx, ledger.Entry{Date: caldate.Today(s.now()), Description: "Tazminat ödemesi " + c.ClaimNo, RefType: "CLAIM_PAYMENT", RefID: id, CreatedBy: p.UserID}, []ledger.Line{{AccountCode: "2000", DebitKurus: out.PayableKurus}, {AccountCode: "1000", CreditKurus: out.PayableKurus}})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE claim_settlements SET status='PAID',paid_at=now(),payment_journal_entry_id=$2 WHERE id=$1`, out.ID, jid); err != nil {
			return err
		}
		if _, err := Transition(ctx, tx, id, "PAID", p, "Tazminat ödendi", c.RowVersion); err != nil {
			return err
		}
		out, err = loadSettlement(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "CLAIM_PAID", EntityType: "claim", EntityID: id, Old: map[string]any{"status": "APPROVED"}, New: out})
	})
	if db.Violates(err, "journal_entries_one_per_event_uq") {
		return nil, httpx.NewError(409, "CLAIM_ALREADY_PAID", "hasar için ödeme kaydı zaten var")
	}
	return out, err
}
