// Package access, rol + kullanıcıdan veri görünürlük kapsamını çözer.
// Kural (tutarlı): rol bir uca hiç erişemiyorsa 403; erişebiliyor ama kayıt kapsam dışındaysa 404
// (başkasına ait kaydın varlığı sızdırılmaz).
package access

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

var ErrNoProfile = errors.New("kullanıcının rolüne bağlı profil (acente/müşteri) kaydı yok")

// Scope: All=true ise kısıt yok; aksi halde AgentID veya CustomerID ile daraltılır.
type Scope struct {
	All        bool
	AgentID    *int64
	CustomerID *int64
}

// Allows kaydın (agentID, customerID) kapsama girip girmediğini söyler.
func (s Scope) Allows(agentID *int64, customerID int64) bool {
	switch {
	case s.All:
		return true
	case s.AgentID != nil:
		return agentID != nil && *agentID == *s.AgentID
	case s.CustomerID != nil:
		return customerID == *s.CustomerID
	}
	return false
}

func Resolve(ctx context.Context, pool *pgxpool.Pool, p auth.Principal) (Scope, error) {
	switch p.Role {
	case auth.RoleAgent:
		id, err := lookup(ctx, pool, `SELECT id FROM agents WHERE user_id = $1`, p.UserID)
		return Scope{AgentID: id}, err
	case auth.RoleCustomer:
		id, err := lookup(ctx, pool, `SELECT id FROM customers WHERE user_id = $1`, p.UserID)
		return Scope{CustomerID: id}, err
	default:
		return Scope{All: true}, nil
	}
}

func lookup(ctx context.Context, pool *pgxpool.Pool, q string, userID int64) (*int64, error) {
	var id int64
	err := pool.QueryRow(ctx, q, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoProfile
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// AsHTTP profil eksikliğini 403'e çevirir; diğer hataları olduğu gibi bırakır.
func AsHTTP(err error) error {
	if errors.Is(err, ErrNoProfile) {
		return httpx.NewError(http.StatusForbidden, httpx.CodeForbidden, err.Error())
	}
	return err
}
