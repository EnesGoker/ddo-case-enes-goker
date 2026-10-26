package policy

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/customer"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment/store"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/policypdf"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

func pdfPrivateFields(role auth.Role) bool {
	return role == auth.RoleCustomer || role == auth.RoleAgent || role == auth.RoleUnderwriter || role == auth.RoleAccounting
}

// loadPDFSnapshot does not call pool-based helpers while a transaction is held.
// Authorization, versions and billing all observe the same read-only snapshot.
func (h *Handler) loadPDFSnapshot(ctx context.Context, id int64, at time.Time) (policypdf.Document, error) {
	var out policypdf.Document
	err := pgx.BeginTxFunc(ctx, h.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		actor, _ := auth.FromContext(ctx)
		scope, err := access.Resolve(ctx, tx, actor)
		if err != nil {
			return access.AsHTTP(err)
		}
		header, err := LoadHeader(ctx, tx, id, false)
		if errors.Is(err, ErrNotFound) || (err == nil && !scope.Allows(header.AgentID, header.CustomerID)) {
			return httpx.NotFound("poliçe")
		}
		if err != nil {
			return err
		}
		d, err := LoadDetail(ctx, tx, header)
		if err != nil {
			return err
		}
		if len(d.Versions) == 0 {
			return errors.New("poliçe sürümü bulunamadı")
		}
		v := d.Versions[len(d.Versions)-1]
		lines, err := pdfPremiumLines(ctx, tx, d, v)
		if err != nil {
			return err
		}
		var maskedID string
		var plan *policypdf.Plan
		if pdfPrivateFields(actor.Role) {
			var nationalID string
			if err := tx.QueryRow(ctx, `SELECT national_id FROM customers WHERE id = $1`, d.CustomerID).Scan(&nationalID); err != nil {
				return err
			}
			maskedID = customer.MaskNationalID(nationalID)
			state, err := store.Load(ctx, tx, id)
			var appErr *httpx.AppError
			if errors.As(err, &appErr) && appErr.Code == "BILLING_PLAN_MISSING" {
				plan = &policypdf.Plan{Missing: true}
			} else if err != nil {
				return err
			} else {
				plan = &policypdf.Plan{Total: state.Total}
				for _, row := range state.Rows {
					plan.Rows = append(plan.Rows, policypdf.Installment{Seq: row.Seq, Due: caldate.Format(row.Due), Amount: row.Amount, Paid: row.Paid, Status: row.Status})
				}
			}
		}
		out = pdfDocument(d, v, actor.Role, maskedID, plan, at)
		for _, l := range lines {
			out.Lines = append(out.Lines, policypdf.Line{Coverage: l.CoverageCode, Label: l.Label, Amount: l.AmountKurus, BP: l.MultiplierBP})
		}
		return nil
	})
	return out, err
}

func pdfPremiumLines(ctx context.Context, q db.DBTX, d Detail, v VersionView) ([]pricing.Line, error) {
	if v.VersionNo == 1 && d.QuoteID != nil {
		rows, err := q.Query(ctx, `
			SELECT coalesce(coverage_code,''), coalesce(factor_label,''),
			       coalesce(multiplier_bp,0), amount_kurus
			  FROM quote_premium_lines WHERE quote_id = $1 ORDER BY seq`, *d.QuoteID)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, func(row pgx.CollectableRow) (pricing.Line, error) {
			var line pricing.Line
			err := row.Scan(&line.CoverageCode, &line.Label, &line.MultiplierBP, &line.AmountKurus)
			return line, err
		})
	}
	for _, e := range d.Endorsements {
		if e.ToVersionID != nil && *e.ToVersionID == v.ID {
			var calc CalcDetail
			if err := json.Unmarshal(e.CalcDetail, &calc); err != nil {
				return nil, err
			}
			return calc.NewPremiumLines, nil
		}
	}
	// SIM and seed issuance may have no quote. Never reconstruct historical lines.
	return nil, nil
}

func pdfDocument(d Detail, v VersionView, role auth.Role, maskedID string, plan *policypdf.Plan, at time.Time) policypdf.Document {
	out := policypdf.Document{PolicyNo: d.PolicyNo, Product: d.Product, Customer: d.CustomerName, Status: d.Status,
		Start: d.StartDate, End: d.EndDate, VersionNo: v.VersionNo, TariffNo: v.TariffVersionNo,
		VersionFrom: v.EffectiveFrom, Annual: v.AnnualPremium, AnnualLimit: v.AnnualLimit,
		Initial: d.Premium.InitialKurus, Delta: d.Premium.EndorsementDelta, Written: d.Premium.WrittenKurus,
		Refund: d.Premium.RefundKurus, Net: d.Premium.NetKurus, GeneratedAt: at}
	if d.CancelEffectiveDate != nil {
		out.CancelDate = *d.CancelEffectiveDate
	}
	if d.CancelReason != nil {
		out.CancelReason = *d.CancelReason
	}
	if v.EffectiveTo != nil {
		out.VersionTo = *v.EffectiveTo
	}
	if v.VehiclePlate != nil {
		out.Plate = *v.VehiclePlate
	}
	for _, c := range v.Coverages {
		out.Coverages = append(out.Coverages, policypdf.Coverage{Code: c.Code, Name: c.Name, Limit: c.LimitKurus,
			EventLimit: c.EventLimitKurus, DeductibleType: string(c.DeductibleType), Deductible: c.DeductibleValue, Premium: c.PremiumKurus})
	}
	if pdfPrivateFields(role) {
		out.MaskedID, out.Billing = maskedID, plan
	}
	return out
}
