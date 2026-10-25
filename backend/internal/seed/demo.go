// Package seed demo verisini (6 rol hesabı, acente, örnek müşteriler) idempotent olarak yükler.
// Tüm kişi/kimlik bilgileri kurgudur.
package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
)

type DemoUser struct {
	Email    string
	FullName string
	Role     auth.Role
}

// DemoUsers README ve TESTING.md'deki demo hesaplarıdır.
var DemoUsers = []DemoUser{
	{"musteri@guvenpolice.test", "Ayşe Demir", auth.RoleCustomer},
	{"acente@guvenpolice.test", "Kemal Acar", auth.RoleAgent},
	{"underwriter@guvenpolice.test", "Selin Uysal", auth.RoleUnderwriter},
	{"eksper@guvenpolice.test", "Murat Ekinci", auth.RoleAppraiser},
	{"hasar@guvenpolice.test", "Deniz Hasat", auth.RoleClaimsAdjuster},
	{"muhasebe@guvenpolice.test", "Ece Mutlu", auth.RoleAccounting},
}

type demoCustomer struct {
	nationalID string
	fullName   string
	birthDate  string
	region     string
	userEmail  string // boşsa kullanıcı hesabı yok
	withAgent  bool
}

var demoCustomers = []demoCustomer{
	{"10000000001", "Ayşe Demir", "1988-04-12", "34", "musteri@guvenpolice.test", true},
	{"10000000002", "Mehmet Yılmaz", "1975-02-28", "06", "", true},
	{"10000000003", "Zeynep Kaya", "2000-02-29", "35", "", true},
	{"10000000004", "Ali Çelik", "1962-11-30", "16", "", false},
}

// Demo yükler. Var olan kullanıcıların parolası DEMO_PASSWORD ile eşleşmiyorsa güncellenir;
// böylece .env'deki parola değişince seed tekrar tutarlı hale gelir.
func Demo(ctx context.Context, pool *pgxpool.Pool, password string) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		ids := map[string]int64{}
		for _, u := range DemoUsers {
			var id int64
			var current string
			err := tx.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE lower(email) = lower($1)`, u.Email).
				Scan(&id, &current)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.BcryptCost)
				if err != nil {
					return err
				}
				err = tx.QueryRow(ctx,
					`INSERT INTO users (email, password_hash, full_name, role) VALUES ($1, $2, $3, $4) RETURNING id`,
					u.Email, string(hash), u.FullName, string(u.Role)).Scan(&id)
				if err != nil {
					return fmt.Errorf("demo kullanıcı %s: %w", u.Email, err)
				}
			case err != nil:
				return err
			case bcrypt.CompareHashAndPassword([]byte(current), []byte(password)) != nil:
				hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.BcryptCost)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, string(hash)); err != nil {
					return err
				}
			}
			ids[u.Email] = id
		}

		var agentID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO agents (user_id, agency_name, commission_bp) VALUES ($1, 'Örnek Sigorta Acentesi', 1500)
			ON CONFLICT (user_id) DO UPDATE SET agency_name = EXCLUDED.agency_name
			RETURNING id`, ids["acente@guvenpolice.test"]).Scan(&agentID)
		if err != nil {
			return fmt.Errorf("demo acente: %w", err)
		}

		for _, c := range demoCustomers {
			var userID, agent *int64
			if c.userEmail != "" {
				id := ids[c.userEmail]
				userID = &id
			}
			if c.withAgent {
				agent = &agentID
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO customers (user_id, agent_id, national_id, full_name, birth_date, region_code)
				VALUES ($1, $2, $3, $4, $5::date, $6)
				ON CONFLICT (national_id) DO NOTHING`,
				userID, agent, c.nationalID, c.fullName, c.birthDate, c.region)
			if err != nil {
				return fmt.Errorf("demo müşteri %s: %w", c.fullName, err)
			}
		}
		return nil
	})
}
