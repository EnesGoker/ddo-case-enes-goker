//go:build integration

package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
)

// issuePolicy Ayşe için KASKO teklifi oluşturup poliçeye çevirir; poliçe yanıtını döner.
func issuePolicy(t *testing.T, agent string) map[string]any {
	t.Helper()
	q := createQuote(t, agent, kaskoBody(t, ayse, futureStart()))
	s, pol, _ := newClient(t).doH(http.MethodPost, fmt.Sprintf("/api/v1/quotes/%v/convert", q["id"]), agent, nil,
		map[string]string{idempotency.Header: fmt.Sprintf("issue-%v", q["id"])})
	require.Equal(t, 201, s, pol)
	return pol
}

func pgCode(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

// Sürümler değişmez: yalnızca açık sürümün effective_to'su bir kez kapatılabilir (00003 tetikleyicisi).
func TestPolicyVersionsAreImmutable(t *testing.T) {
	ctx := context.Background()
	pol := issuePolicy(t, tokenFor(t, "acente@guvenpolice.test"))
	vid := num(pol["version_id"])

	_, err := pool.Exec(ctx, `UPDATE policy_versions SET annual_premium_kurus = 1 WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "açık sürümde prim değiştirilemez")
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = effective_from + 10, vehicle_plate = '06ZZ99' WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "kapatırken başka alan değiştirilemez")
	_, err = pool.Exec(ctx, `UPDATE policy_version_coverages SET limit_kurus = 1 WHERE policy_version_id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "sürüm teminatı değiştirilemez")
	_, err = pool.Exec(ctx, `DELETE FROM policy_version_coverages WHERE policy_version_id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "sürüm teminatı silinemez")
	_, err = pool.Exec(ctx, `DELETE FROM policy_versions WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "sürüm silinemez")

	// Tek izinli geçiş: NULL → tarih
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = effective_from + 10 WHERE id = $1`, vid)
	require.NoError(t, err)
	// Kapanmış sürüm bir daha değiştirilemez (tarih dahil)
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = effective_from + 20 WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "kapanmış sürüm yeniden kapatılamaz")
	_, err = pool.Exec(ctx, `UPDATE policy_versions SET effective_to = NULL WHERE id = $1`, vid)
	assert.Equal(t, "55000", pgCode(err), "kapanmış sürüm yeniden açılamaz")
}
