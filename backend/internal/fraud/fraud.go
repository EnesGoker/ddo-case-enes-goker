// Package fraud hasar tarihine göre deterministik, kurgusal risk sinyalleri hesaplar.
package fraud

import (
	"fmt"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
)

type Config struct {
	Threshold       int
	EarlyDays       int
	EarlyPoints     int
	RepeatDays      int
	RepeatMinimum   int
	RepeatPoints    int
	WorkshopDays    int
	WorkshopMinimum int
	WorkshopPoints  int
}

func DefaultConfig() Config {
	return Config{Threshold: 60, EarlyDays: 15, EarlyPoints: 40, RepeatDays: 365, RepeatMinimum: 2, RepeatPoints: 35, WorkshopDays: 30, WorkshopMinimum: 4, WorkshopPoints: 30}
}

func (c Config) Validate() error {
	for _, v := range []int{c.Threshold, c.EarlyDays, c.EarlyPoints, c.RepeatDays, c.RepeatMinimum, c.RepeatPoints, c.WorkshopDays, c.WorkshopMinimum, c.WorkshopPoints} {
		if v <= 0 || v > 2147483647 {
			return fmt.Errorf("fraud ayarları 1..2147483647 aralığında olmalı")
		}
	}
	if int64(c.EarlyPoints)+int64(c.RepeatPoints)+int64(c.WorkshopPoints) > 2147483647 {
		return fmt.Errorf("fraud skoru int32 sınırını aşıyor")
	}
	return nil
}

type Input struct {
	PolicyStart    time.Time
	Loss           time.Time
	HasPlate       bool
	HasWorkshop    bool
	PlateClaims    []string
	WorkshopClaims []string
}

type Detail struct {
	Days           int64    `json:"days"`
	Threshold      int      `json:"threshold"`
	Count          int      `json:"count,omitempty"`
	From           string   `json:"from,omitempty"`
	To             string   `json:"to,omitempty"`
	PreviousClaims []string `json:"previous_claim_nos,omitempty"`
}

type Signal struct {
	RuleCode    string `json:"rule_code"`
	Points      int    `json:"points"`
	Detail      Detail `json:"detail"`
	Explanation string `json:"explanation"`
}

type Result struct {
	Score     int      `json:"score"`
	Threshold int      `json:"threshold"`
	Flagged   bool     `json:"flagged"`
	Signals   []Signal `json:"signals"`
}

// Window kapalı [loss-days,loss] aralığıdır; SQL aynı sınırları kullanır.
func Window(loss time.Time, days int) (time.Time, time.Time) {
	end := caldate.Of(loss)
	return end.AddDate(0, 0, -days), end
}

func Evaluate(in Input, cfg Config) (Result, error) {
	out := Result{Threshold: cfg.Threshold, Signals: []Signal{}}
	if err := cfg.Validate(); err != nil {
		return out, err
	}
	days := proration.DaysBetween(in.PolicyStart, in.Loss)
	if days < 0 {
		return out, fmt.Errorf("hasar başlangıçtan önce olamaz")
	}
	if days < int64(cfg.EarlyDays) {
		out.Signals = append(out.Signals, Signal{"EARLY_CLAIM", cfg.EarlyPoints, Detail{Days: days, Threshold: cfg.EarlyDays}, fmt.Sprintf("Hasar başlangıçtan %d gün sonra; eşik %d günden önce.", days, cfg.EarlyDays)})
	}
	for _, r := range []struct {
		enabled                 bool
		names                   []string
		minimum, window, points int
		code, label             string
	}{
		{in.HasPlate, in.PlateClaims, cfg.RepeatMinimum, cfg.RepeatDays, cfg.RepeatPoints, "REPEAT_PLATE", "Aynı plakada"},
		{in.HasWorkshop, in.WorkshopClaims, cfg.WorkshopMinimum, cfg.WorkshopDays, cfg.WorkshopPoints, "WORKSHOP_CONCENTRATION", "Aynı atölyede"},
	} {
		if !r.enabled || len(r.names) < r.minimum {
			continue
		}
		from, to := Window(in.Loss, r.window)
		out.Signals = append(out.Signals, Signal{r.code, r.points, Detail{Threshold: r.minimum, Count: len(r.names), From: caldate.Format(from), To: caldate.Format(to), PreviousClaims: append([]string(nil), r.names...)}, fmt.Sprintf("%s [%s, %s] aralığında %d önceki hasar; eşik %d.", r.label, caldate.Format(from), caldate.Format(to), len(r.names), r.minimum)})
	}
	for _, s := range out.Signals {
		out.Score += s.Points
	}
	out.Flagged = out.Score >= cfg.Threshold
	return out, nil
}
