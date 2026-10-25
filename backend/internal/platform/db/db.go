// Package db PostgreSQL bağlantı havuzu ve goose migrasyonlarını yönetir.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // goose için database/sql sürücüsü
	"github.com/pressly/goose/v3"
)

// Connect havuzu açar ve veritabanı hazır olana kadar (en fazla wait süresi) bekler.
func Connect(ctx context.Context, url string, wait time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL çözümlenemedi: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = pool.Ping(pctx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("veritabanına bağlanılamadı: %w", err)
		}
		time.Sleep(time.Second)
	}
}

// Migrate dir içindeki goose migrasyonlarını en son sürüme uygular.
func Migrate(ctx context.Context, url, dir string) error {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetLogger(goose.NopLogger())
	return goose.UpContext(ctx, sqlDB, dir)
}
