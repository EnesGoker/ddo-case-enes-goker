package claim

import (
	"context"
	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

type TransitionRequest struct {
	ToStatus           string `json:"to_status"`
	Note               string `json:"note"`
	ExpectedRowVersion int    `json:"expected_row_version"`
}

// Transition tüm durum değişikliklerinin tek noktasıdır; çağıranın transaction'ını kullanır.
func Transition(ctx context.Context, tx pgx.Tx, claimID int64, to string, actor auth.Principal, note string, expectedRowVersion int) (Claim, error) {
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
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM claim_transitions WHERE from_status=$1 AND to_status=$2)`, c.Status, to).Scan(&allowed); err != nil {
		return c, err
	}
	if !allowed {
		return c, httpx.NewError(http.StatusConflict, "INVALID_TRANSITION", "bu durum geçişine izin verilmiyor")
	}
	from := c.Status
	if _, err := tx.Exec(ctx, `UPDATE claims SET status=$2,row_version=row_version+1,updated_at=now() WHERE id=$1`, claimID, to); err != nil {
		return c, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO claim_status_history (claim_id,from_status,to_status,actor_user_id,note) VALUES ($1,$2,$3,$4,$5)`, claimID, from, to, actor.UserID, note); err != nil {
		return c, err
	}
	c.Status = to
	c.RowVersion++
	err = audit.Write(ctx, tx, audit.Entry{Action: "CLAIM_TRANSITIONED", EntityType: "claim", EntityID: c.ID, Old: map[string]any{"status": from, "row_version": expectedRowVersion}, New: map[string]any{"status": to, "row_version": c.RowVersion, "note": note}})
	return c, err
}
