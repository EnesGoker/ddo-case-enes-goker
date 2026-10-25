// Package audit kritik işlemleri audit_log'a, iş verisiyle aynı transaction içinde yazar.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/logging"
)

type Entry struct {
	Action     string // ör. QUOTE_CREATED, QUOTE_REVIEWED, POLICY_ISSUED
	EntityType string // ör. quote, policy
	EntityID   int64
	Old        any // nil olabilir
	New        any
}

// Write kaydı tx içinde ekler; aktör ve rol bağlamdaki kimlikten, request-id log bağlamından gelir.
func Write(ctx context.Context, tx pgx.Tx, e Entry) error {
	var actorID *int64
	var role *string
	if p, ok := auth.FromContext(ctx); ok {
		actorID = &p.UserID
		r := string(p.Role)
		role = &r
	}
	oldJSON, err := marshal(e.Old)
	if err != nil {
		return err
	}
	newJSON, err := marshal(e.New)
	if err != nil {
		return err
	}
	var reqID *string
	if id := logging.RequestID(ctx); id != "" {
		reqID = &id
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_log (actor_user_id, actor_role, action, entity_type, entity_id, old_value, new_value, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		actorID, role, e.Action, e.EntityType, fmt.Sprint(e.EntityID), oldJSON, newJSON, reqID)
	return err
}

func marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
