package claim

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

type Claim struct {
	ID                 int64     `json:"id"`
	ClaimNo            string    `json:"claim_no"`
	PolicyID           int64     `json:"policy_id"`
	PolicyNo           string    `json:"policy_no"`
	PolicyVersionID    int64     `json:"policy_version_id"`
	CoverageCode       string    `json:"coverage_code"`
	LossDate           string    `json:"loss_date"`
	ReportedAt         time.Time `json:"reported_at"`
	Plate              *string   `json:"plate"`
	WorkshopID         *int64    `json:"workshop_id"`
	Description        string    `json:"description"`
	ClaimedAmountKurus int64     `json:"claimed_amount_kurus"`
	Status             string    `json:"status"`
	RowVersion         int       `json:"row_version"`
	CreatedBy          int64     `json:"created_by"`
	CustomerID         int64     `json:"customer_id"`
	AgentID            *int64    `json:"agent_id"`
}

type History struct {
	ID          int64     `json:"id"`
	FromStatus  *string   `json:"from_status"`
	ToStatus    string    `json:"to_status"`
	ActorUserID *int64    `json:"actor_user_id"`
	Note        *string   `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

type Appraisal struct {
	ID                int64     `json:"id"`
	AppraiserID       int64     `json:"appraiser_id"`
	InspectedAt       time.Time `json:"inspected_at"`
	DamageAmountKurus int64     `json:"damage_amount_kurus"`
	IsTotalLoss       bool      `json:"is_total_loss"`
	Notes             *string   `json:"notes"`
	CreatedAt         time.Time `json:"created_at"`
}

type Settlement struct {
	ID                    int64           `json:"id"`
	ClaimID               int64           `json:"claim_id"`
	GrossDamageKurus      int64           `json:"gross_damage_kurus"`
	DeductibleKurus       int64           `json:"deductible_kurus"`
	PayableKurus          int64           `json:"payable_kurus"`
	CalcSteps             json.RawMessage `json:"calc_steps"`
	Status                string          `json:"status"`
	ApprovedBy            int64           `json:"approved_by"`
	ApprovedAt            time.Time       `json:"approved_at"`
	PaidAt                *time.Time      `json:"paid_at"`
	PaymentJournalEntryID *int64          `json:"payment_journal_entry_id"`
}

type Detail struct {
	Claim
	History    []History   `json:"history"`
	Appraisals []Appraisal `json:"appraisals"`
	Settlement *Settlement `json:"settlement"`
}

const claimCols = `c.id,c.claim_no,c.policy_id,p.policy_no,c.policy_version_id,c.coverage_code,to_char(c.loss_date,'YYYY-MM-DD'),c.reported_at,c.plate,c.workshop_id,c.description,c.claimed_amount_kurus,c.status,c.row_version,c.created_by,p.customer_id,p.agent_id`
const claimFrom = ` FROM claims c JOIN policies p ON p.id=c.policy_id `

func scanClaim(row pgx.Row) (Claim, error) {
	var c Claim
	err := row.Scan(&c.ID, &c.ClaimNo, &c.PolicyID, &c.PolicyNo, &c.PolicyVersionID, &c.CoverageCode, &c.LossDate, &c.ReportedAt, &c.Plate, &c.WorkshopID, &c.Description, &c.ClaimedAmountKurus, &c.Status, &c.RowVersion, &c.CreatedBy, &c.CustomerID, &c.AgentID)
	return c, err
}
func load(ctx context.Context, q db.DBTX, id int64, lock bool) (Claim, error) {
	sql := `SELECT ` + claimCols + claimFrom + ` WHERE c.id=$1`
	if lock {
		sql += ` FOR UPDATE OF c`
	}
	c, err := scanClaim(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, httpx.NotFound("hasar")
	}
	return c, err
}
func loadSettlement(ctx context.Context, q db.DBTX, id int64) (*Settlement, error) {
	var s Settlement
	err := q.QueryRow(ctx, `SELECT id,claim_id,gross_damage_kurus,deductible_kurus,payable_kurus,calc_steps,status,approved_by,approved_at,paid_at,payment_journal_entry_id FROM claim_settlements WHERE claim_id=$1 AND status<>'CANCELLED'`, id).Scan(&s.ID, &s.ClaimID, &s.GrossDamageKurus, &s.DeductibleKurus, &s.PayableKurus, &s.CalcSteps, &s.Status, &s.ApprovedBy, &s.ApprovedAt, &s.PaidAt, &s.PaymentJournalEntryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}
func loadDetail(ctx context.Context, q db.DBTX, c Claim) (Detail, error) {
	d := Detail{Claim: c, History: []History{}, Appraisals: []Appraisal{}}
	rows, err := q.Query(ctx, `SELECT id,from_status,to_status,actor_user_id,note,created_at FROM claim_status_history WHERE claim_id=$1 ORDER BY id`, c.ID)
	if err != nil {
		return d, err
	}
	d.History, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (History, error) {
		var h History
		err := r.Scan(&h.ID, &h.FromStatus, &h.ToStatus, &h.ActorUserID, &h.Note, &h.CreatedAt)
		return h, err
	})
	if err != nil {
		return d, err
	}
	rows, err = q.Query(ctx, `SELECT id,appraiser_id,inspected_at,damage_amount_kurus,is_total_loss,notes,created_at FROM appraisals WHERE claim_id=$1 ORDER BY id`, c.ID)
	if err != nil {
		return d, err
	}
	d.Appraisals, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Appraisal, error) {
		var a Appraisal
		err := r.Scan(&a.ID, &a.AppraiserID, &a.InspectedAt, &a.DamageAmountKurus, &a.IsTotalLoss, &a.Notes, &a.CreatedAt)
		return a, err
	})
	if err != nil {
		return d, err
	}
	d.Settlement, err = loadSettlement(ctx, q, c.ID)
	return d, err
}
