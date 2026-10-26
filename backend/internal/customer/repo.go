// Package customer müşteri listesini sunar (A1: RBAC'ın gerçek bir uçta kanıtı).
package customer

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Customer struct {
	ID         int64   `json:"id"`
	FullName   string  `json:"full_name"`
	NationalID string  `json:"national_id_masked"`
	BirthDate  string  `json:"birth_date"`
	RegionCode string  `json:"region_code"`
	AgentID    *int64  `json:"agent_id"`
	Email      *string `json:"email"`
	Phone      *string `json:"phone"`
}

// ListFilter: AgentID nil ise tüm müşteriler; AfterID keyset imleci.
type ListFilter struct {
	AgentID *int64
	AfterID int64
	Limit   int
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

var ErrNoAgentProfile = errors.New("kullanıcının acente kaydı yok")

func (r *Repo) AgentIDForUser(ctx context.Context, userID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM agents WHERE user_id = $1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoAgentProfile
	}
	return id, err
}

func (r *Repo) List(ctx context.Context, f ListFilter) ([]Customer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, full_name, national_id, to_char(birth_date, 'YYYY-MM-DD'), region_code, agent_id, email, phone
		  FROM customers
		 WHERE id > $1 AND ($2::bigint IS NULL OR agent_id = $2)
		 ORDER BY id
		 LIMIT $3`, f.AfterID, f.AgentID, f.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Customer, error) {
		var c Customer
		err := row.Scan(&c.ID, &c.FullName, &c.NationalID, &c.BirthDate, &c.RegionCode, &c.AgentID, &c.Email, &c.Phone)
		c.NationalID = maskNationalID(c.NationalID)
		return c, err
	})
}

// maskNationalID TCKN'nin yalnızca son 4 hanesini gösterir (kişisel veri).
func maskNationalID(s string) string {
	if len(s) <= 4 {
		return s
	}
	b := []byte(s)
	for i := 0; i < len(b)-4; i++ {
		b[i] = '*'
	}
	return string(b)
}
