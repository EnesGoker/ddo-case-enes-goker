package policy

import (
	"context"
	"errors"
	"net/http"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
)

func (s *ChangeService) replayChange(ctx context.Context, tx pgx.Tx, id int64, kind string) (*Detail, *EndorsementView, error) {
	h, err := LoadHeader(ctx, tx, id, true)
	if errors.Is(err, ErrNotFound) {
		return nil, nil, httpx.NotFound("poliçe")
	}
	if err != nil {
		return nil, nil, err
	}
	actor, _ := auth.FromContext(ctx)
	scope, err := access.Resolve(ctx, s.pool, actor)
	if err != nil {
		return nil, nil, access.AsHTTP(err)
	}
	if !scope.Allows(h.AgentID, h.CustomerID) {
		return nil, nil, httpx.NotFound("poliçe")
	}
	key := idempotency.Key(ctx)
	if key == nil {
		return nil, nil, nil
	}
	var eid int64
	var previousKind string
	err = tx.QueryRow(ctx, `
  SELECT id,type FROM endorsements WHERE policy_id=$1 AND created_by=$2 AND request_key=$3`, id, actor.UserID, key).Scan(&eid, &previousKind)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if previousKind != kind {
		return nil, nil, httpx.NewError(http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "iş anahtarındaki zeyil türü farklı")
	}
	d, err := LoadDetail(ctx, tx, h)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range d.Endorsements {
		if e.ID == eid {
			return &d, &e, nil
		}
	}
	return nil, nil, httpx.NotFound("zeyil")
}
