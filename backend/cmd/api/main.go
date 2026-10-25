// Command api GüvenPoliçe HTTP API sunucusudur. Açılışta migrasyonları uygular ve
// (SEED_DEMO=true ise) demo verisini yükler.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/app"
	"github.com/EnesGoker/guvenpolice/backend/internal/config"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/logging"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "yerel /health'i çağır ve çık (Docker HEALTHCHECK için)")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}
	if err := run(); err != nil {
		slog.Error("sunucu durdu", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 60*time.Second)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, cfg.DatabaseURL, cfg.MigrationsDir); err != nil {
		return fmt.Errorf("migrasyon: %w", err)
	}
	logger.Info("migrasyonlar uygulandı", "dir", cfg.MigrationsDir)

	if cfg.SeedDemo {
		if err := seed.Demo(ctx, pool, cfg.DemoPassword, seed.Options{Fraud: cfg.Fraud, Workflow: cfg.Workflow, LateInterestDailyPPM: cfg.LateInterestDailyPPM}); err != nil {
			return fmt.Errorf("demo seed: %w", err)
		}
		logger.Info("demo seed yüklendi", "users", len(seed.DemoUsers))
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: app.NewRouter(app.Deps{
			Pool:                 pool,
			FraudConfig:          cfg.Fraud,
			WorkflowConfig:       cfg.Workflow,
			LateInterestDailyPPM: cfg.LateInterestDailyPPM,
			Logger:               logger,
			JWTSecret:            cfg.JWTSecret,
			AccessTokenTTL:       cfg.AccessTokenTTL,
			RefreshTokenTTL:      cfg.RefreshTokenTTL,
			CookieSecure:         cfg.CookieSecure,

			UWPremiumThresholdKurus: cfg.UWPremiumThresholdKurus,
			IdempotencyStaleAfter:   cfg.IdempotencyStaleAfter,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("HTTP dinleniyor", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	logger.Info("kapanış sinyali alındı, açık istekler bekleniyor")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func runHealthcheck() int {
	addr := os.Getenv("HTTP_ADDR")
	switch {
	case addr == "":
		addr = "127.0.0.1:8080"
	case addr[0] == ':':
		addr = "127.0.0.1" + addr
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
