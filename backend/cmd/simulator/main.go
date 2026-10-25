package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/EnesGoker/guvenpolice/backend/internal/config"
	"github.com/EnesGoker/guvenpolice/backend/internal/simulation"
)

func main() {
	if err := run(); err != nil {
		slog.Error("simülatör durdu", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := simulation.LoadConfig()
	if err != nil {
		return err
	}
	appConfig, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Fraud, cfg.Workflow, cfg.Rate = appConfig.Fraud, appConfig.Workflow, appConfig.LateInterestDailyPPM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := simulation.OpenPool(ctx, appConfig.DatabaseURL, cfg.Workers)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = simulation.Run(ctx, pool, cfg, slog.Default())
	return err
}
