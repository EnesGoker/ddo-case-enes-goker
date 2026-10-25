package simulation

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/tariff"
)

type agentRef struct{ ID, UserID int64 }
type customerRef struct {
	ID     int64
	Agent  int
	Region string
	Birth  time.Time
}
type references struct {
	agents     []agentRef
	customers  []customerRef
	workshops  []int64
	appraisers []int64
	adjusters  []int64
	account    int64
	tariffs    map[pricing.Product][]pricing.Tariff
}

func (r *runner) references(ctx context.Context) (out references, err error) {
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return out, err
	}
	hash, err := bcrypt.GenerateFromPassword(secret, auth.BcryptCost)
	if err != nil {
		return out, err
	}
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		user := func(label string, ordinal int, role auth.Role) (int64, error) {
			var id int64
			err := tx.QueryRow(ctx, `
              INSERT INTO users(email,password_hash,full_name,role,is_active)
              VALUES($1,$2,$3,$4,true) RETURNING id`, fmt.Sprintf("sim-%s-%02d@guvenpolice.test", label, ordinal), string(hash), fmt.Sprintf("Kurgusal Simülasyon %s %02d", label, ordinal), role).Scan(&id)
			return id, err
		}
		for i := 0; i < 20; i++ {
			uid, err := user("agent", i, auth.RoleAgent)
			if err != nil {
				return err
			}
			var aid int64
			err = tx.QueryRow(ctx, `
              INSERT INTO agents(user_id,agency_name,commission_bp)
              VALUES($1,$2,1500) RETURNING id`, uid, fmt.Sprintf("Kurgusal SIM Acente %02d", i)).Scan(&aid)
			if err != nil {
				return err
			}
			out.agents = append(out.agents, agentRef{aid, uid})
		}
		for _, group := range []struct {
			label string
			role  auth.Role
			count int
			dest  *[]int64
		}{{"appraiser", auth.RoleAppraiser, 5, &out.appraisers}, {"adjuster", auth.RoleClaimsAdjuster, 8, &out.adjusters}} {
			for i := 0; i < group.count; i++ {
				id, err := user(group.label, i, group.role)
				if err != nil {
					return err
				}
				*group.dest = append(*group.dest, id)
			}
		}
		var err error
		if out.account, err = user("accounting", 0, auth.RoleAccounting); err != nil {
			return err
		}
		if _, err = user("underwriter", 0, auth.RoleUnderwriter); err != nil {
			return err
		}
		customerUser, err := user("customer", 0, auth.RoleCustomer)
		if err != nil {
			return err
		}
		n := max(1, r.cfg.Policies*9/10)
		rows := make([][]any, n)
		regions := []string{"34", "06", "35", "16"}
		for i := range n {
			var uid *int64
			if i == 0 {
				uid = &customerUser
			}
			birth := time.Date(r.cfg.Anchor.Year()-r.cfg.Years-25-i%40, time.Month(1+i%12), 1+i%27, 0, 0, 0, 0, time.UTC)
			rows[i] = []any{uid, out.agents[i%20].ID, fmt.Sprintf("%011d", 90000000000+int64(i)), fmt.Sprintf("Kurgusal SIM Müşteri %06d", i+1), birth, regions[i%len(regions)]}
		}
		if _, err = tx.CopyFrom(ctx, pgx.Identifier{"customers"}, []string{"user_id", "agent_id", "national_id", "full_name", "birth_date", "region_code"}, pgx.CopyFromRows(rows)); err != nil {
			return err
		}
		result, err := tx.Query(ctx, `
          SELECT id,birth_date,region_code FROM customers
           WHERE national_id >= '90000000000' AND national_id < '90000810000' ORDER BY national_id`)
		if err != nil {
			return err
		}
		out.customers, err = pgx.CollectRows(result, func(row pgx.CollectableRow) (customerRef, error) {
			var c customerRef
			err := row.Scan(&c.ID, &c.Birth, &c.Region)
			return c, err
		})
		if err != nil {
			return err
		}
		for i := range out.customers {
			out.customers[i].Agent = i % 20
		}
		for i := 0; i < 30; i++ {
			var id int64
			err := tx.QueryRow(ctx, `
              INSERT INTO workshops(name,region_code) VALUES($1,$2) RETURNING id`, fmt.Sprintf("Kurgusal SIM Atölye %02d", i), regions[i%len(regions)]).Scan(&id)
			if err != nil {
				return err
			}
			out.workshops = append(out.workshops, id)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	out.tariffs = map[pricing.Product][]pricing.Tariff{}
	for _, product := range []pricing.Product{pricing.Kasko, pricing.Trafik, pricing.Konut} {
		out.tariffs[product], err = tariff.LoadActive(ctx, r.pool, product)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
