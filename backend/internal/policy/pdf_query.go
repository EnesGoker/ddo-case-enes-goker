package policy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/policypdf"
)

// loadPDFSnapshot applies Detail's authorization and reads Detail's data in one read-only
// snapshot; no pool-based helper is called while the transaction is held.
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
		out = pdfDocument(d, d.Versions[len(d.Versions)-1], at)
		return nil
	})
	return out, err
}

func pdfDocument(d Detail, v VersionView, at time.Time) policypdf.Document {
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
	return out
}
