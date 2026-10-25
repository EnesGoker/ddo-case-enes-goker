package simulation

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
)

func (r *runner) live(ctx context.Context) (resultErr error) {
	r.realtime = true
	if len(r.refs.customers) == 0 {
		rows, err := r.pool.Query(ctx, `
          SELECT c.id,c.birth_date,c.region_code,a.id,a.user_id FROM customers c
            JOIN agents a ON a.id=c.agent_id JOIN users u ON u.id=a.user_id
           WHERE u.email LIKE 'sim-agent-%@guvenpolice.test' ORDER BY c.national_id`)
		if err != nil {
			return err
		}
		customers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct {
			customer customerRef
			agent    agentRef
		}, error) {
			var v struct {
				customer customerRef
				agent    agentRef
			}
			err := row.Scan(&v.customer.ID, &v.customer.Birth, &v.customer.Region, &v.agent.ID, &v.agent.UserID)
			return v, err
		})
		if err != nil {
			return err
		}
		for _, v := range customers {
			v.customer.Agent = len(r.refs.agents)
			r.refs.customers = append(r.refs.customers, v.customer)
			r.refs.agents = append(r.refs.agents, v.agent)
		}
		if err = r.pool.QueryRow(ctx, `SELECT id FROM users WHERE email='sim-accounting-00@guvenpolice.test'`).Scan(&r.refs.account); err != nil {
			return err
		}
		r.refs.tariffs = map[pricing.Product][]pricing.Tariff{}
		for _, product := range []pricing.Product{pricing.Kasko, pricing.Trafik, pricing.Konut} {
			r.refs.tariffs[product], err = tariff.LoadActive(ctx, r.pool, product)
			if err != nil {
				return err
			}
		}
	}
	if len(r.refs.customers) == 0 {
		return fmt.Errorf("canlı mod için SIM referans verisi yok")
	}
	_, err := r.pool.Exec(ctx, `
      UPDATE simulation_runs SET status='LIVE',phase='LIVE',updated_at=now(),finished_at=NULL WHERE id=$1`, r.id)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		status := "COMPLETED"
		if resultErr != nil {
			status = "FAILED"
		}
		_, _ = r.pool.Exec(cleanup, `
          UPDATE simulation_runs SET status=$2,phase='LIVE_STOPPED',finished_at=now(),updated_at=now(),fingerprint=NULL WHERE id=$1 AND status='LIVE'`, r.id, status)
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			var ordinal int
			if err := r.pool.QueryRow(ctx, `
              SELECT substring(policy_no from 5)::int FROM policies
               WHERE policy_no LIKE 'SIM-%' ORDER BY policy_no DESC LIMIT 1`).Scan(&ordinal); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if ordinal >= 810000 {
				return fmt.Errorf("SIM plaka uzayı doldu")
			}
			r.cfg.Anchor = caldate.Today(time.Now())
			p := planPolicy(r.cfg, ordinal)
			p.Start = r.cfg.Anchor
			p.End = caldate.AddYear(p.Start)
			p.CancelAt = nil
			p.Losses = nil
			p.Unpaid = false
			p.Count = 3
			p.Customer = ordinal % len(r.refs.customers)
			// Yeni poliçenin ilk vadesi gerçek gününde tahsil edilir; hasar/atama üretimi yok.
			if err := r.issueAndPay(ctx, &p); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if _, err := r.pool.Exec(ctx, `
              UPDATE simulation_runs SET updated_at=now(),processed=processed+1,target=target+1 WHERE id=$1`, r.id); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}
