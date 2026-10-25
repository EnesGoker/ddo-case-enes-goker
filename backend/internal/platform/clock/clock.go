// Package clock alt katman INSERT'lerine işlem anını taşır. Üretimde yalnız
// seed ve simülatör WithTime çağırır; HTTP yolları gerçek saati kullanır.
package clock

import (
	"context"
	"time"
)

type timeKey struct{}

func WithTime(ctx context.Context, at time.Time) context.Context {
	return context.WithValue(ctx, timeKey{}, at)
}

func Now(ctx context.Context) time.Time {
	if at, ok := ctx.Value(timeKey{}).(time.Time); ok && !at.IsZero() {
		return at
	}
	return time.Now()
}
