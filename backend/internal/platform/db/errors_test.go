package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestViolates(t *testing.T) {
	unique := &pgconn.PgError{Code: "23505", ConstraintName: "policies_quote_id_key"}
	excl := &pgconn.PgError{Code: "23P01", ConstraintName: "policy_versions_policy_id_daterange_excl"}
	check := &pgconn.PgError{Code: "23514", ConstraintName: "policies_quote_id_key"}

	assert.True(t, Violates(unique, "x", "policies_quote_id_key"))
	assert.True(t, Violates(fmt.Errorf("poliçe: %w", unique), "policies_quote_id_key"), "sarılmış hata")
	assert.True(t, Violates(excl, "policy_versions_policy_id_daterange_excl"))
	assert.False(t, Violates(unique, "baska_kisit"))
	assert.False(t, Violates(check, "policies_quote_id_key"), "CHECK ihlali benzersizlik sayılmaz")
	assert.False(t, Violates(errors.New("düz hata"), "policies_quote_id_key"))
	assert.False(t, Violates(nil, "policies_quote_id_key"))
}
