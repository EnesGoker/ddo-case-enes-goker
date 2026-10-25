package idempotency

import (
	"context"
	"net/http"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

type keyContext struct{}

func WithKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, keyContext{}, key)
}

func Key(ctx context.Context) *string {
	key, _ := ctx.Value(keyContext{}).(string)
	if key == "" {
		return nil
	}
	return &key
}

func Conflict(err error) error {
	if db.Violates(err, "payments_request_key_uq", "endorsements_request_key_uq", "claims_request_key_uq") {
		return httpx.NewError(http.StatusConflict, "IDEMPOTENCY_CONFLICT", "istek anahtarı eşzamanlı kaydedildi")
	}
	return err
}
