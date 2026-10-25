//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
)

func TestBillingRunChunksMatchSingle(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := principalContext(t, "muhasebe@guvenpolice.test", auth.RoleAccounting)
	svc := payment.NewService(pool, time.Now, 1000)
	var after int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT coalesce(max(id),0) FROM policies`).Scan(&after))
	for range 3 {
		billingPolicy(t, 20, 3, 300000)
	}
	var total int64
	for i := 0; i < 3; i++ {
		out, err := svc.Run(ctx, "", payment.RunOptions{AfterPolicyID: after, Limit: 1})
		require.NoError(t, err)
		require.Empty(t, out.Failed)
		assert.Equal(t, i == 2, out.Done)
		assert.Greater(t, out.NextAfterPolicyID, after)
		after = out.NextAfterPolicyID
		total += out.InterestKurus
	}
	for range 3 {
		billingPolicy(t, 20, 3, 300000)
	}
	out, err := svc.Run(ctx, "", payment.RunOptions{AfterPolicyID: after, Limit: 10})
	require.NoError(t, err)
	assert.True(t, out.Done)
	assert.Equal(t, total, out.InterestKurus)
	// 3 × (100000 × 1000 ppm × 20 gün / 1e6) = 6000 kuruş.
	assert.Equal(t, int64(6000), total)
	replay, err := svc.Run(ctx, "", payment.RunOptions{AfterPolicyID: after, Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, replay.InterestKurus)
	assert.Empty(t, replay.Cancelled)
}

func TestBillingRunFailureIsolationAndCursor(t *testing.T) {
	defer assertBillingInvariant(t)
	ctx := principalContext(t, "muhasebe@guvenpolice.test", auth.RoleAccounting)
	good1 := billingPolicy(t, 20, 3, 300000)
	bad := billingPolicy(t, 20, 3, 300000)
	good2 := billingPolicy(t, 20, 3, 300000)
	// Yalnız test DB'sindeki INSERT kancası; başarısız transaction'ın faizini de geri alır.
	_, err := pool.Exec(ctx, fmt.Sprintf(`
      CREATE FUNCTION a7_fail_one_policy() RETURNS trigger LANGUAGE plpgsql AS $$
      BEGIN
        IF NEW.action='BILLING_RUN' AND NEW.entity_id='%d' THEN
          RAISE EXCEPTION 'test policy failure' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
      END $$;
      CREATE TRIGGER a7_fail_one_policy BEFORE INSERT ON audit_log
      FOR EACH ROW EXECUTE FUNCTION a7_fail_one_policy()`, num(bad["id"])))
	require.NoError(t, err)
	defer func() {
		_, err := pool.Exec(context.Background(), `DROP TRIGGER a7_fail_one_policy ON audit_log; DROP FUNCTION a7_fail_one_policy()`)
		require.NoError(t, err)
	}()
	out, err := payment.NewService(pool, time.Now, 1000).Run(ctx, "", payment.RunOptions{AfterPolicyID: num(good1["id"]) - 1, Limit: 3})
	require.NoError(t, err)
	assert.True(t, out.Done)
	assert.Equal(t, num(good2["id"]), out.NextAfterPolicyID)
	assert.Equal(t, []payment.Failed{{PolicyID: num(bad["id"]), Code: "DATABASE_23514"}}, out.Failed)
	assert.Equal(t, 2, out.AccrualCount)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	assert.Empty(t, billingDetail(t, account, bad)["interest"])
	for _, pol := range []map[string]any{good1, good2} {
		assert.Len(t, billingDetail(t, account, pol)["interest"], 1)
	}
}

func TestBillingRunExcludesFutureDuesAndValidatesCursor(t *testing.T) {
	defer assertBillingInvariant(t)
	pol := billingPolicy(t, -5, 3, 300000)
	account := tokenFor(t, "muhasebe@guvenpolice.test")
	s, out := newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, map[string]any{"after_policy_id": num(pol["id"]) - 1, "limit": 1})
	require.Equal(t, http.StatusOK, s, out)
	assert.Equal(t, true, out["done"])
	assert.Equal(t, num(pol["id"])-1, num(out["next_after_policy_id"]))
	assert.Empty(t, billingDetail(t, account, pol)["interest"])
	for _, body := range []map[string]any{{"after_policy_id": -1}, {"limit": 2001}, {"limit": -1}} {
		s, out = newClient(t).do(http.MethodPost, "/api/v1/billing/run", account, body)
		assert.Equal(t, http.StatusBadRequest, s, out)
	}
	ctx, cancel := context.WithCancel(principalContext(t, "muhasebe@guvenpolice.test", auth.RoleAccounting))
	cancel()
	_, err := payment.NewService(pool, time.Now, 1000).Run(ctx, "")
	assert.ErrorIs(t, err, context.Canceled)
}
