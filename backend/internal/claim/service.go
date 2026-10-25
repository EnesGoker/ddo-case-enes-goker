// Package claim hasar bildirimi, dosya takibi ve ekspertiz işlemleridir.
package claim

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
)

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, now: now}
}

var validStatuses = map[string]bool{"REPORTED": true, "TRIAGED": true, "APPRAISAL": true, "ADJUSTMENT": true, "FRAUD_REVIEW": true, "APPROVED": true, "REJECTED": true, "PAID": true, "CLOSED": true}

func (s *Service) visible(ctx context.Context, q db.DBTX, id int64, lock bool) (Claim, error) {
	p, _ := auth.FromContext(ctx)
	sc, err := access.Resolve(ctx, s.pool, p)
	if err != nil {
		return Claim{}, access.AsHTTP(err)
	}
	c, err := load(ctx, q, id, lock)
	if err != nil {
		return c, err
	}
	if !sc.Allows(c.AgentID, c.CustomerID) {
		return c, httpx.NotFound("hasar")
	}
	if p.Role == auth.RoleAppraiser && c.Status != "APPRAISAL" {
		var own bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM appraisals WHERE claim_id=$1 AND appraiser_id=$2)`, id, p.UserID).Scan(&own); err != nil {
			return c, err
		}
		if !own {
			return c, httpx.NotFound("hasar")
		}
	}
	return c, nil
}

type ReportRequest struct {
	PolicyID           int64  `json:"policy_id"`
	CoverageCode       string `json:"coverage_code"`
	LossDate           string `json:"loss_date"`
	Description        string `json:"description"`
	ClaimedAmountKurus *int64 `json:"claimed_amount_kurus"`
	WorkshopID         *int64 `json:"workshop_id"`
}

func (s *Service) Report(ctx context.Context, req ReportRequest) (Claim, error) {
	var out Claim
	loss, err := caldate.Parse(req.LossDate)
	bad := map[string]string{}
	if err != nil {
		bad["loss_date"] = err.Error()
	}
	if req.PolicyID <= 0 {
		bad["policy_id"] = "zorunlu"
	}
	req.CoverageCode = strings.ToUpper(strings.TrimSpace(req.CoverageCode))
	req.Description = strings.TrimSpace(req.Description)
	if req.CoverageCode == "" {
		bad["coverage_code"] = "zorunlu"
	}
	if len(req.Description) == 0 || len(req.Description) > 4000 {
		bad["description"] = "1-4000 karakter olmalı"
	}
	if req.ClaimedAmountKurus == nil || *req.ClaimedAmountKurus < 0 {
		bad["claimed_amount_kurus"] = "zorunlu, negatif olamaz"
	}
	if req.WorkshopID != nil && *req.WorkshopID <= 0 {
		bad["workshop_id"] = "geçersiz atölye"
	}
	if len(bad) > 0 {
		return out, httpx.Invalid(bad)
	}
	if loss.After(caldate.Today(s.now())) {
		return out, httpx.NewError(422, "LOSS_DATE_IN_FUTURE", "hasar tarihi gelecekte olamaz")
	}
	p, _ := auth.FromContext(ctx)
	sc, err := access.Resolve(ctx, s.pool, p)
	if err != nil {
		return out, access.AsHTTP(err)
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Zeyil/iptal ile aynı poliçe kilidi: sürüm çözümü ve hasar kaydı atomiktir.
		h, err := policy.LoadHeader(ctx, tx, req.PolicyID, true)
		if errors.Is(err, policy.ErrNotFound) || (err == nil && !sc.Allows(h.AgentID, h.CustomerID)) {
			return httpx.NotFound("poliçe")
		}
		if err != nil {
			return err
		}
		if key := idempotency.Key(ctx); key != nil {
			var existing int64
			err := tx.QueryRow(ctx, `
    SELECT id FROM claims WHERE created_by=$1 AND request_key=$2 AND policy_id=$3`, p.UserID, key, h.ID).Scan(&existing)
			if err == nil {
				out, err = load(ctx, tx, existing, false)
				return err
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		v, err := policy.VersionOn(ctx, tx, h.ID, h.Product, loss)
		if errors.Is(err, policy.ErrNoCoverage) {
			return httpx.NewError(422, "NO_COVERAGE_ON_DATE", "hasar tarihinde geçerli teminat yok")
		}
		if err != nil {
			return err
		}
		found := false
		for _, c := range v.Coverages {
			if c.Code == req.CoverageCode {
				found = true
				break
			}
		}
		if !found {
			return httpx.NewError(422, "COVERAGE_NOT_IN_VERSION", "teminat hasar tarihindeki sürümde yok")
		}
		if req.WorkshopID != nil {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workshops WHERE id=$1)`, *req.WorkshopID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.NewError(422, "WORKSHOP_NOT_FOUND", "atölye bulunamadı")
			}
		}
		var id int64
		err = tx.QueryRow(ctx, `WITH n AS (SELECT nextval(pg_get_serial_sequence('claims','id')) AS id)
   INSERT INTO claims (id,claim_no,policy_id,policy_version_id,coverage_code,loss_date,plate,workshop_id,description,claimed_amount_kurus,created_by,request_key)
   OVERRIDING SYSTEM VALUE SELECT n.id,'GP-H-'||lpad(n.id::text,greatest(8,length(n.id::text)),'0'),$1,$2,$3,$4,$5,$6,$7,$8,$9,$10 FROM n RETURNING id`, h.ID, v.ID, req.CoverageCode, loss, v.VehiclePlate, req.WorkshopID, req.Description, *req.ClaimedAmountKurus, p.UserID, idempotency.Key(ctx)).Scan(&id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO claim_status_history(claim_id,from_status,to_status,actor_user_id,note) VALUES($1,NULL,'REPORTED',$2,'Hasar bildirildi')`, id, p.UserID); err != nil {
			return err
		}
		out, err = load(ctx, tx, id, false)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "CLAIM_REPORTED", EntityType: "claim", EntityID: id, New: out})
	})
	return out, idempotency.Conflict(err)
}
func (s *Service) Get(ctx context.Context, id int64) (Detail, error) {
	c, err := s.visible(ctx, s.pool, id, false)
	if err != nil {
		return Detail{}, err
	}
	return loadDetail(ctx, s.pool, c)
}
func (s *Service) List(ctx context.Context, params page.Params, status string) ([]Claim, error) {
	p, _ := auth.FromContext(ctx)
	sc, err := access.Resolve(ctx, s.pool, p)
	if err != nil {
		return nil, access.AsHTTP(err)
	}
	rows, err := s.pool.Query(ctx, `SELECT `+claimCols+claimFrom+` WHERE ($1::bigint=0 OR c.id<$1)
 AND ($2::bigint IS NULL OR p.agent_id=$2) AND ($3::bigint IS NULL OR p.customer_id=$3)
 AND ($4='' OR c.status=$4)
 AND ($5::boolean=false OR c.status='APPRAISAL' OR EXISTS(SELECT 1 FROM appraisals a WHERE a.claim_id=c.id AND a.appraiser_id=$6)) ORDER BY c.id DESC LIMIT $7`, params.AfterID, sc.AgentID, sc.CustomerID, status, p.Role == auth.RoleAppraiser, p.UserID, params.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Claim, error) { return scanClaim(r) })
}
func (s *Service) ChangeStatus(ctx context.Context, id int64, req TransitionRequest) (Claim, error) {
	var out Claim
	if req.ExpectedRowVersion < 1 || len(req.Note) > 4000 {
		return out, httpx.Invalid(map[string]string{"expected_row_version": "pozitif sürüm ve en fazla 4000 karakter not gerekli"})
	}
	req.ToStatus = strings.ToUpper(strings.TrimSpace(req.ToStatus))
	switch req.ToStatus {
	case "TRIAGED", "APPRAISAL", "ADJUSTMENT", "REJECTED", "CLOSED":
	default:
		return out, httpx.NewError(409, "INVALID_TRANSITION", "bu uçtan istenen duruma geçilemez")
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.visible(ctx, tx, id, true); err != nil {
			return err
		}
		p, _ := auth.FromContext(ctx)
		var err error
		out, err = Transition(ctx, tx, id, req.ToStatus, p, req.Note, req.ExpectedRowVersion)
		return err
	})
	return out, err
}

type AppraisalRequest struct {
	DamageAmountKurus *int64 `json:"damage_amount_kurus"`
	IsTotalLoss       bool   `json:"is_total_loss"`
	Notes             string `json:"notes"`
	InspectedAt       string `json:"inspected_at"`
}

func (s *Service) Appraise(ctx context.Context, id int64, req AppraisalRequest) (Appraisal, error) {
	var out Appraisal
	inspected, err := time.Parse(time.RFC3339, req.InspectedAt)
	if err != nil || (req.DamageAmountKurus == nil || *req.DamageAmountKurus < 0) || len(req.Notes) > 4000 {
		return out, httpx.Invalid(map[string]string{"appraisal": "RFC3339 inspected_at, negatif olmayan hasar ve en fazla 4000 karakter not gerekli"})
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		c, err := s.visible(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if c.Status != "APPRAISAL" {
			return httpx.NewError(http.StatusConflict, "INVALID_CLAIM_STATUS", "ekspertiz yalnızca APPRAISAL durumunda girilebilir")
		}
		p, _ := auth.FromContext(ctx)
		out = Appraisal{AppraiserID: p.UserID, InspectedAt: inspected, DamageAmountKurus: *req.DamageAmountKurus, IsTotalLoss: req.IsTotalLoss, Notes: &req.Notes}
		err = tx.QueryRow(ctx, `INSERT INTO appraisals (claim_id,appraiser_id,inspected_at,damage_amount_kurus,is_total_loss,notes) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at`, id, p.UserID, inspected, *req.DamageAmountKurus, req.IsTotalLoss, req.Notes).Scan(&out.ID, &out.CreatedAt)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "CLAIM_APPRAISED", EntityType: "claim", EntityID: id, New: out})
	})
	return out, err
}
