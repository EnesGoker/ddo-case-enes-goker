// Package config tüm yapılandırmayı ortam değişkenlerinden okur.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
)

type Config struct {
	Fraud                fraud.Config
	Workflow             workflow.Config
	LateInterestDailyPPM int64
	HTTPAddr             string
	DatabaseURL          string
	MigrationsDir        string
	JWTSecret            []byte
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	CookieSecure         bool
	SeedDemo             bool
	DemoPassword         string
	LogLevel             string
	// UWPremiumThresholdKurus bu tutarı aşan teklif underwriter onayına düşer (0 = kapalı)
	UWPremiumThresholdKurus int64
	// IdempotencyStaleAfter IN_PROGRESS anahtarın yeniden işlenebileceği süre
	IdempotencyStaleAfter time.Duration
}

// Load ortamı okur ve doğrular. Eksik/zayıf secret ile servis açılmaz.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		MigrationsDir: getenv("MIGRATIONS_DIR", "../db/migrations"),
		JWTSecret:     []byte(os.Getenv("JWT_SECRET")),
		DemoPassword:  os.Getenv("DEMO_PASSWORD"),
		LogLevel:      getenv("LOG_LEVEL", "info"),
	}
	var err error
	if c.AccessTokenTTL, err = duration("ACCESS_TOKEN_TTL", 15*time.Minute); err != nil {
		return c, err
	}
	if c.RefreshTokenTTL, err = duration("REFRESH_TOKEN_TTL", 7*24*time.Hour); err != nil {
		return c, err
	}
	if c.CookieSecure, err = boolean("COOKIE_SECURE", false); err != nil {
		return c, err
	}
	if c.SeedDemo, err = boolean("SEED_DEMO", true); err != nil {
		return c, err
	}
	if c.IdempotencyStaleAfter, err = duration("IDEMPOTENCY_STALE_AFTER", 60*time.Second); err != nil {
		return c, err
	}
	c.LateInterestDailyPPM = 1000
	if v := os.Getenv("LATE_INTEREST_DAILY_PPM"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("LATE_INTEREST_DAILY_PPM pozitif tam sayı olmalı")
		}
		c.LateInterestDailyPPM = n
	}
	c.UWPremiumThresholdKurus = 3_000_000
	if v := os.Getenv("UW_PREMIUM_THRESHOLD_KURUS"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return c, fmt.Errorf("UW_PREMIUM_THRESHOLD_KURUS geçersiz: %q", v)
		}
		c.UWPremiumThresholdKurus = n
	}

	c.Fraud = fraud.DefaultConfig()
	for _, setting := range []struct {
		key    string
		target *int
	}{
		{"FRAUD_THRESHOLD", &c.Fraud.Threshold},
		{"FRAUD_EARLY_DAYS", &c.Fraud.EarlyDays},
		{"FRAUD_EARLY_POINTS", &c.Fraud.EarlyPoints},
		{"FRAUD_REPEAT_DAYS", &c.Fraud.RepeatDays},
		{"FRAUD_REPEAT_MINIMUM", &c.Fraud.RepeatMinimum},
		{"FRAUD_REPEAT_POINTS", &c.Fraud.RepeatPoints},
		{"FRAUD_WORKSHOP_DAYS", &c.Fraud.WorkshopDays},
		{"FRAUD_WORKSHOP_MINIMUM", &c.Fraud.WorkshopMinimum},
		{"FRAUD_WORKSHOP_POINTS", &c.Fraud.WorkshopPoints},
	} {
		if raw := os.Getenv(setting.key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return c, fmt.Errorf("%s tam sayı olmalı", setting.key)
			}
			*setting.target = n
		}
	}
	if err := c.Fraud.Validate(); err != nil {
		return c, err
	}
	defaults := workflow.DefaultConfig()
	if c.Workflow.Appraisal, err = duration("SLA_APPRAISAL", defaults.Appraisal); err != nil {
		return c, err
	}
	if c.Workflow.Adjustment, err = duration("SLA_ADJUSTMENT", defaults.Adjustment); err != nil {
		return c, err
	}
	if c.Workflow.FraudReview, err = duration("SLA_FRAUD_REVIEW", defaults.FraudReview); err != nil {
		return c, err
	}
	if err := c.Workflow.Validate(); err != nil {
		return c, err
	}

	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL zorunlu")
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET en az 32 bayt olmalı")
	}
	if c.SeedDemo && len(c.DemoPassword) < 8 {
		return c, errors.New("SEED_DEMO=true iken DEMO_PASSWORD en az 8 karakter olmalı")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func duration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s geçersiz süre: %q", key, v)
	}
	return d, nil
}

func boolean(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s geçersiz bool: %q", key, v)
	}
	return b, nil
}
