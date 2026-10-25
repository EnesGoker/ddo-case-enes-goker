// Package system yalnız sayısal sistem canlılığı ve simülasyon ilerlemesini sunar.
package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Minute struct {
	At    time.Time `json:"at"`
	Count int64     `json:"count"`
}
type Simulation struct {
	Status      string          `json:"status"`
	Phase       string          `json:"phase"`
	Done        int64           `json:"done"`
	Target      int64           `json:"target"`
	Seed        int64           `json:"seed"`
	Fingerprint *string         `json:"fingerprint"`
	StartedAt   time.Time       `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
	Summary     json.RawMessage `json:"summary"`
}

type Activity struct {
	DBOK        bool        `json:"db_ok"`
	LastEventAt *time.Time  `json:"last_event_at"`
	Minutes     []Minute    `json:"minutes"`
	Simulation  *Simulation `json:"simulation"`
}

func Read(ctx context.Context, pool *pgxpool.Pool) (Activity, error) {
	out := Activity{Minutes: []Minute{}}
	end := time.Now().UTC().Truncate(time.Minute)
	start := end.Add(-29 * time.Minute)
	var at time.Time
	err := pool.QueryRow(ctx, `SELECT occurred_at FROM audit_log ORDER BY occurred_at DESC LIMIT 1`).Scan(&at)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err == nil {
		out.LastEventAt = &at
	}
	rows, err := pool.Query(ctx, `
      SELECT date_trunc('minute',occurred_at),count(*) FROM audit_log
       WHERE occurred_at >= $1 AND occurred_at < $2 GROUP BY 1`, start, end.Add(time.Minute))
	if err != nil {
		return out, err
	}
	counts := map[int64]int64{}
	for rows.Next() {
		var at time.Time
		var count int64
		if err := rows.Scan(&at, &count); err != nil {
			rows.Close()
			return out, err
		}
		counts[at.Unix()] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for i := range 30 {
		at := start.Add(time.Duration(i) * time.Minute)
		out.Minutes = append(out.Minutes, Minute{At: at, Count: counts[at.Unix()]})
	}
	var run Simulation
	err = pool.QueryRow(ctx, `
      SELECT status,phase,processed,target,seed,fingerprint,started_at,finished_at,summary
        FROM simulation_runs ORDER BY started_at DESC,id DESC LIMIT 1`).Scan(&run.Status, &run.Phase, &run.Done, &run.Target, &run.Seed, &run.Fingerprint, &run.StartedAt, &run.FinishedAt, &run.Summary)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err == nil {
		out.Simulation = &run
	}
	out.DBOK = true
	return out, nil
}

func ActivityHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		out, err := Read(ctx, pool)
		if err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "veritabanına ulaşılamıyor")
			return
		}
		httpx.JSON(w, http.StatusOK, out)
	}
}
