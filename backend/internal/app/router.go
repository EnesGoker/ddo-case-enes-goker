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
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/logging"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/metrics"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/quote"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
)

type Deps struct {
	Pool            *pgxpool.Pool
	Logger          *slog.Logger
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CookieSecure    bool
	RequestTimeout  time.Duration
	// UWPremiumThresholdKurus 0 ise kural kapalı
	UWPremiumThresholdKurus int64
	IdempotencyStaleAfter   time.Duration
	Now                     func() time.Time
}

func NewRouter(d Deps) http.Handler {
	if d.RequestTimeout == 0 {
		d.RequestTimeout = 15 * time.Second
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	reg := metrics.NewRegistry()
	tokens := auth.NewTokenIssuer(d.JWTSecret, d.AccessTokenTTL)
	authSvc := auth.NewService(d.Pool, tokens, d.RefreshTokenTTL)
	authH := auth.NewHandler(authSvc, tokens, d.CookieSecure, reg)
	customers := customer.NewRepo(d.Pool)
	quotes := quote.NewHandler(quote.NewService(d.Pool, d.Now, d.UWPremiumThresholdKurus))
	idem := idempotency.NewStore(d.Pool, d.IdempotencyStaleAfter)
	policies := policy.NewHandler(d.Pool)

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
			r.With(auth.RequireRoles(auth.RoleAgent, auth.RoleUnderwriter)).
				Get("/tariffs/active", tariff.ActiveHandler(d.Pool, d.Now))

			r.Route("/quotes", func(r chi.Router) {
				agent := auth.RequireRoles(auth.RoleAgent)
				readers := auth.RequireRoles(auth.RoleAgent, auth.RoleUnderwriter, auth.RoleCustomer)
				r.With(agent).Post("/preview", quotes.Preview)
				r.With(agent).Post("/", quotes.Create)
				r.With(readers).Get("/", quotes.List)
				r.With(readers).Get("/{id}", quotes.Get)
				r.With(auth.RequireRoles(auth.RoleUnderwriter)).Post("/{id}/review", quotes.Review)
				r.With(agent, idem.Middleware).Post("/{id}/convert", quotes.Convert)
			})

			r.Route("/policies", func(r chi.Router) {
				readers := auth.RequireRoles(auth.RoleCustomer, auth.RoleAgent, auth.RoleUnderwriter,
					auth.RoleClaimsAdjuster, auth.RoleAccounting)
				writers := auth.RequireRoles(auth.RoleAgent, auth.RoleUnderwriter)
				r.With(readers).Get("/", policy.ListHandler(d.Pool))
				r.With(readers).Get("/{id}", policies.Detail)
				r.With(readers).Get("/{id}/coverage", policies.Coverage)
				r.With(writers).Post("/{id}/endorsements/preview", policies.PreviewVehicleChange)
				r.With(writers, idem.Middleware).Post("/{id}/endorsements", policies.VehicleChange)
				r.With(writers).Post("/{id}/cancel/preview", policies.PreviewCancel)
				r.With(writers, idem.Middleware).Post("/{id}/cancel", policies.Cancel)
			})
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
