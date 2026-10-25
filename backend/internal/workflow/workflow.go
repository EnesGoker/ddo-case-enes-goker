// Package workflow DB'siz görev atama ve takvim saatli SLA kurallarıdır.
package workflow

import (
	"fmt"
	"time"
)

type Config struct {
	Appraisal   time.Duration
	Adjustment  time.Duration
	FraudReview time.Duration
}

func DefaultConfig() Config {
	return Config{Appraisal: 72 * time.Hour, Adjustment: 48 * time.Hour, FraudReview: 24 * time.Hour}
}

func (c Config) Validate() error {
	if c.Appraisal <= 0 || c.Adjustment <= 0 || c.FraudReview <= 0 {
		return fmt.Errorf("bütün SLA süreleri pozitif olmalı")
	}
	return nil
}

func DueAt(created time.Time, kind string, cfg Config) (time.Time, error) {
	if err := cfg.Validate(); err != nil {
		return time.Time{}, err
	}
	if created.IsZero() {
		return time.Time{}, fmt.Errorf("görev oluşturma zamanı gerekli")
	}
	var duration time.Duration
	switch kind {
	case "APPRAISAL":
		duration = cfg.Appraisal
	case "ADJUSTMENT":
		duration = cfg.Adjustment
	case "FRAUD_REVIEW":
		duration = cfg.FraudReview
	default:
		return time.Time{}, fmt.Errorf("bilinmeyen görev tipi: %s", kind)
	}
	return created.Add(duration), nil
}

func Open(status string) bool {
	return status == "OPEN" || status == "IN_PROGRESS"
}

func Breached(status string, due time.Time, completed *time.Time, now time.Time) bool {
	if Open(status) {
		return now.After(due)
	}
	return completed != nil && completed.After(due)
}

type Load struct {
	UserID int64
	Open   int64
}

func PickAssignee(loads []Load) *int64 {
	if len(loads) == 0 {
		return nil
	}
	picked := loads[0]
	for _, candidate := range loads[1:] {
		if candidate.Open < picked.Open || (candidate.Open == picked.Open && candidate.UserID < picked.UserID) {
			picked = candidate
		}
	}
	return &picked.UserID
}

func RemainingSeconds(due, now time.Time) int64 {
	d := due.Sub(now)
	if d < 0 {
		return -int64((-d + time.Second - 1) / time.Second)
	}
	return int64(d / time.Second)
}
