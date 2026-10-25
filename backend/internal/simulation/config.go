// Package simulation mevcut iş servisleriyle ayrık, kurgusal bir veri kümesi üretir.
package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
)

type Config struct {
	Seed     int64           `json:"seed"`
	Policies int             `json:"policies"`
	Years    int             `json:"years"`
	Anchor   time.Time       `json:"anchor"`
	Workers  int             `json:"-"`
	Live     bool            `json:"-"`
	Fraud    fraud.Config    `json:"fraud"`
	Workflow workflow.Config `json:"workflow"`
	Rate     int64           `json:"late_interest_daily_ppm"`
}

func DefaultConfig() Config {
	return Config{Seed: 42, Policies: 100000, Years: 5, Anchor: caldate.Today(time.Now()), Workers: 8, Fraud: fraud.DefaultConfig(), Workflow: workflow.DefaultConfig(), Rate: 1000}
}

func LoadConfig() (Config, error) {
	c := DefaultConfig()
	for _, v := range []struct {
		key   string
		value *int
	}{{"SIM_POLICIES", &c.Policies}, {"SIM_YEARS", &c.Years}, {"SIM_WORKERS", &c.Workers}} {
		if raw := os.Getenv(v.key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return c, fmt.Errorf("%s tam sayı olmalı", v.key)
			}
			*v.value = n
		}
	}
	var err error
	if raw := os.Getenv("SIM_SEED"); raw != "" {
		c.Seed, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return c, fmt.Errorf("SIM_SEED: %w", err)
		}
	}
	if raw := os.Getenv("SIM_ANCHOR_DATE"); raw != "" {
		c.Anchor, err = caldate.Parse(raw)
		if err != nil {
			return c, err
		}
	}
	if raw := os.Getenv("SIM_LIVE"); raw != "" {
		c.Live, err = strconv.ParseBool(raw)
		if err != nil {
			return c, fmt.Errorf("SIM_LIVE: %w", err)
		}
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.Policies < 1 || c.Policies > 810000 || c.Workers < 1 || c.Workers > 64 || c.Years < 1 || c.Years > 20 || c.Anchor.IsZero() || c.Rate <= 0 {
		return fmt.Errorf("simülasyon: policies 1..810000, workers 1..64, years 1..20, anchor ve pozitif faiz gerekli")
	}
	if c.Anchor.After(caldate.Today(time.Now())) {
		return fmt.Errorf("SIM_ANCHOR_DATE gelecekte olamaz")
	}
	if c.Anchor.AddDate(-c.Years, 0, 0).Year() < 1 {
		return fmt.Errorf("simülasyon başlangıcı 0001 yılından önce olamaz")
	}
	if err := c.Fraud.Validate(); err != nil {
		return err
	}
	return c.Workflow.Validate()
}

func (c Config) parameters() ([]byte, string, error) {
	raw, err := json.Marshal(struct {
		Version int `json:"version"`
		Config
	}{1, c})
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), err
}
