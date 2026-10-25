package policy

import (
	"context"
	"errors"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
)

func (s *ChangeService) replayChange(ctx context.Context, tx pgx.Tx, id int64) (*Detail, *EndorsementView, error) {
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
	err = tx.QueryRow(ctx, `
  SELECT id FROM endorsements WHERE policy_id=$1 AND created_by=$2 AND request_key=$3`, id, actor.UserID, key).Scan(&eid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
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
