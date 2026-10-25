package claim

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/task"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
)

type TransitionRequest struct {
	ToStatus           string `json:"to_status"`
	Note               string `json:"note"`
	ExpectedRowVersion int    `json:"expected_row_version"`
}

type TransitionOptions struct {
	At        time.Time
	SLA       workflow.Config
	Automatic bool
}

func (o TransitionOptions) Validate() error {
	if err := o.SLA.Validate(); err != nil {
		return fmt.Errorf("TransitionOptions: %w", err)
	}
	if o.At.IsZero() {
		return fmt.Errorf("TransitionOptions işlem zamanı gerekli")
	}
	return nil
}

// Transition tüm durum değişikliklerinin tek noktasıdır; çağıranın transaction'ını kullanır.
func Transition(ctx context.Context, tx pgx.Tx, claimID int64, to string, actor auth.Principal, note string, expectedRowVersion int, options TransitionOptions) (Claim, error) {
	if err := options.Validate(); err != nil {
		return Claim{}, err
	}
	c, err := load(ctx, tx, claimID, true)
	if err != nil {
		return c, err
	}
	if c.RowVersion != expectedRowVersion {
		return c, httpx.NewError(http.StatusConflict, "STALE_CLAIM", "hasar değişti; güncel dosyayı yükleyin")
	}
	if to == "REJECTED" && strings.TrimSpace(note) == "" {
		return c, httpx.NewError(http.StatusUnprocessableEntity, "REJECTION_REASON_REQUIRED", "red gerekçesi zorunlu")
	}
	if c.Status == "FRAUD_REVIEW" && to == "ADJUSTMENT" {
		var exists bool
		if err := tx.QueryRow(ctx, `
   SELECT EXISTS (SELECT 1 FROM appraisals WHERE claim_id=$1)`, claimID).Scan(&exists); err != nil {
			return c, err
		}
		if !exists {
			return c, httpx.NewError(http.StatusConflict, "APPRAISAL_REQUIRED", "incelemeden değerlendirmeye geçiş için ekspertiz gerekli")
		}
	}
	if to == "FRAUD_REVIEW" && strings.TrimSpace(note) == "" {
		return c, httpx.NewError(http.StatusUnprocessableEntity, "FRAUD_REVIEW_REASON_REQUIRED", "incelemeye gönderme gerekçesi zorunlu")
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM claim_transitions WHERE from_status=$1 AND to_status=$2)`, c.Status, to).Scan(&allowed); err != nil {
		return c, err
	}
	if !allowed {
		return c, httpx.NewError(http.StatusConflict, "INVALID_TRANSITION", "bu durum geçişine izin verilmiyor")
	}
	from := c.Status
	if _, err := tx.Exec(ctx, `UPDATE claims SET status=$2,row_version=row_version+1,updated_at=$3 WHERE id=$1`, claimID, to, options.At); err != nil {
		return c, err
	}
	var historyActor *int64
	if !options.Automatic {
		historyActor = &actor.UserID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO claim_status_history (claim_id,from_status,to_status,actor_user_id,note,created_at) VALUES ($1,$2,$3,$4,$5,$6)`, claimID, from, to, historyActor, note, options.At); err != nil {
		return c, err
	}
	if err := task.Advance(ctx, tx, claimID, from, to, options.At, options.SLA); err != nil {
		return c, err
	}
	c.Status = to
	c.RowVersion++
	err = audit.Write(ctx, tx, audit.Entry{Action: "CLAIM_TRANSITIONED", EntityType: "claim", EntityID: c.ID, Old: map[string]any{"status": from, "row_version": expectedRowVersion}, New: map[string]any{"status": to, "row_version": c.RowVersion, "note": note, "auto": options.Automatic}})
	return c, err
}
