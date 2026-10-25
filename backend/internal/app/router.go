// Package app HTTP router'ını bağımlılıklardan kurar (main ve entegrasyon testleri ortak kullanır).
package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/customer"
	"github.com/EnesGoker/guvenpolice/backend/internal/health"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/logging"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/metrics"
)

type Deps struct {
	Pool            *pgxpool.Pool
	Logger          *slog.Logger
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CookieSecure    bool
	RequestTimeout  time.Duration
}

func NewRouter(d Deps) http.Handler {
	if d.RequestTimeout == 0 {
		d.RequestTimeout = 15 * time.Second
	}
	reg := metrics.NewRegistry()
	tokens := auth.NewTokenIssuer(d.JWTSecret, d.AccessTokenTTL)
	authSvc := auth.NewService(d.Pool, tokens, d.RefreshTokenTTL)
	authH := auth.NewHandler(authSvc, tokens, d.CookieSecure, reg)
	customers := customer.NewRepo(d.Pool)

	r := chi.NewRouter()
	r.Use(logging.RequestIDMiddleware)
	r.Use(logging.AccessLog(d.Logger))
	r.Use(reg.HTTPMiddleware)
	r.Use(recoverer)
	r.Use(middleware.Timeout(d.RequestTimeout))

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusNotFound, httpx.CodeNotFound, "kaynak bulunamadı")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "yöntem desteklenmiyor")
	})

	r.Get("/health", health.Handler(d.Pool))
	r.Get("/metrics", reg.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", authH.Routes)
		r.Group(func(r chi.Router) {
			r.Use(auth.Authenticate(tokens))
			r.With(auth.RequireRoles(auth.RoleAgent, auth.RoleUnderwriter)).
				Get("/customers", customer.ListHandler(customers))
		})
	})
	return r
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				httpx.Internal(w, r, fmt.Errorf("panic: %v", v))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
