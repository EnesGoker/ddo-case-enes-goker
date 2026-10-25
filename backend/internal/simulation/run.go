package simulation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPool ayarı her yeni oturuma uygulanır; API havuzu bu işlevi kullanmaz.
func OpenPool(ctx context.Context, url string, workers int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = int32(workers + 4)
	cfg.ConnConfig.RuntimeParams["synchronous_commit"] = "off"
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.RuntimeParams["application_name"] = "guvenpolice-simulator"
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

type Result struct {
	RunID       int64           `json:"run_id"`
	Fingerprint string          `json:"fingerprint"`
	Summary     json.RawMessage `json:"summary"`
	Reused      bool            `json:"reused"`
}

type runner struct {
	pool     *pgxpool.Pool
	cfg      Config
	id       int64
	refs     references
	plans    []policyPlan
	realtime bool
	log      *slog.Logger
}

func Run(ctx context.Context, pool *pgxpool.Pool, cfg Config, logger *slog.Logger) (out Result, err error) {
	if err = cfg.Validate(); err != nil {
		return out, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	// Tüm SIM ad uzayı tek koşuya ait; hash'i farklı koşular da eşzamanlı yazamaz.
	guard, err := pool.Acquire(ctx)
	if err != nil {
		return out, err
	}
	defer guard.Release()
	var locked bool
	if err = guard.QueryRow(ctx, `SELECT pg_try_advisory_lock(71007,1)`).Scan(&locked); err != nil {
		return out, err
	}
	if !locked {
		return out, fmt.Errorf("başka simülatör çalışıyor")
	}
	defer func() { _, _ = guard.Exec(context.Background(), `SELECT pg_advisory_unlock(71007,1)`) }()
	params, hash, err := cfg.parameters()
	if err != nil {
		return out, err
	}
	var status string
	err = pool.QueryRow(ctx, `
      SELECT id,status,coalesce(fingerprint,''),summary FROM simulation_runs WHERE params_hash=$1`, hash).Scan(&out.RunID, &status, &out.Fingerprint, &out.Summary)
	if err == nil && status == "COMPLETED" {
		out.Reused = true
		logger.Info("tamamlanmış koşu yeniden kullanıldı", "run_id", out.RunID, "fingerprint", out.Fingerprint)
		if cfg.Live {
			r := runner{pool: pool, cfg: cfg, id: out.RunID, log: logger}
			return out, r.live(ctx)
		}
		return out, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	var exists bool
	if err = pool.QueryRow(ctx, `
      SELECT EXISTS(SELECT 1 FROM policies WHERE policy_no LIKE 'SIM-%')
          OR EXISTS(SELECT 1 FROM simulation_runs)`).Scan(&exists); err != nil {
		return out, err
	}
	if exists {
		return out, fmt.Errorf("SIM veri kümesi veya yarım koşu var; kurtarma: make reset-demo, ardından aynı parametrelerle make simulate")
	}
	err = pool.QueryRow(ctx, `
      INSERT INTO simulation_runs(seed,parameters,params_hash,status,phase,target)
      VALUES($1,$2,$3,'RUNNING','REFERENCES',$4) RETURNING id`, cfg.Seed, params, hash, cfg.Policies).Scan(&out.RunID)
	if err != nil {
		return out, err
	}
	r := runner{pool: pool, cfg: cfg, id: out.RunID, log: logger}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = pool.Exec(cleanup, `
              UPDATE simulation_runs SET status='FAILED',finished_at=now(),updated_at=now() WHERE id=$1`, r.id)
		}
	}()
	started := time.Now()
	if r.refs, err = r.references(ctx); err != nil {
		return out, err
	}
	if err = r.policies(ctx); err != nil {
		return out, err
	}
	if err = r.claims(ctx); err != nil {
		return out, err
	}
	if out.Summary, out.Fingerprint, err = r.summarize(ctx); err != nil {
		return out, err
	}
	_, err = pool.Exec(ctx, `
      UPDATE simulation_runs SET status='COMPLETED',phase='DONE',processed=$4,target=$4,
             finished_at=now(),updated_at=now(),fingerprint=$2,summary=$3 WHERE id=$1`, r.id, out.Fingerprint, out.Summary, cfg.Policies)
	if err != nil {
		return out, err
	}
	logger.Info("simülasyon tamamlandı", "run_id", r.id, "policies", cfg.Policies, "seconds", time.Since(started).Seconds(), "fingerprint", out.Fingerprint, "summary", string(out.Summary))
	if cfg.Live {
		err = r.live(ctx)
	}
	return out, err
}

func (r *runner) progress(ctx context.Context, phase string, done, target int) error {
	_, err := r.pool.Exec(ctx, `
      UPDATE simulation_runs SET phase=$2,processed=$3,target=$4,updated_at=now() WHERE id=$1`, r.id, phase, done, target)
	if err == nil {
		r.log.Info("simülasyon ilerliyor", "phase", phase, "done", done, "target", target)
	}
	return err
}
