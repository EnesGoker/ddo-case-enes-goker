// Package health GET /health uç noktasıdır; veritabanı erişimini de denetler.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

// Pinger *pgxpool.Pool tarafından karşılanır.
type Pinger interface {
	Ping(ctx context.Context) error
}

type response struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

func Handler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, response{Status: "unavailable", DB: "down"})
			return
		}
		httpx.JSON(w, http.StatusOK, response{Status: "ok", DB: "ok"})
	}
}
