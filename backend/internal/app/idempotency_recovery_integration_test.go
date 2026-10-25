//go:build integration

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
)

func rawPost(t *testing.T, token, path, key string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, data
}

func financialCounts(t *testing.T) []int64 {
	t.Helper()
	var p, e, c, j, a int64
	require.NoError(t, pool.QueryRow(context.Background(), `
 SELECT (SELECT count(*) FROM payments), (SELECT count(*) FROM endorsements),
        (SELECT count(*) FROM claims), (SELECT count(*) FROM journal_entries),
        (SELECT count(*) FROM audit_log)`).Scan(&p, &e, &c, &j, &a))
	return []int64{p, e, c, j, a}
}

func TestBusinessRequestKeyRecovery(t *testing.T) {
	account, agent, uw, adjuster := tokenFor(t, "muhasebe@guvenpolice.test"), tokenFor(t, "acente@guvenpolice.test"), tokenFor(t, "underwriter@guvenpolice.test"), tokenFor(t, "hasar@guvenpolice.test")
	for _, kind := range []string{"payment", "refund", "endorsement", "cancel", "claim"} {
		t.Run(kind, func(t *testing.T) {
			defer assertBillingInvariant(t)
			var pol map[string]any
			var path, token string
			var body map[string]any
			switch kind {
			case "payment", "refund":
				pol = billingPolicy(t, 0, 1, 30000)
				token = account
				path = policyPath(pol) + "/payments"
				body = map[string]any{"amount_kurus": 20000, "method": "TRANSFER"}
				if kind == "refund" {
					p := payBilling(t, account, pol, 30000, "recovery-source")
					path = fmt.Sprintf("/api/v1/payments/%v/refund", p["id"])
					body = map[string]any{"amount_kurus": 20000, "reason": "tekrar denemesi"}
				}
			case "endorsement", "cancel":
				pol = issuePolicy(t, agent)
				path = policyPath(pol) + "/endorsements"
				token = agent
				body = map[string]any{"effective_date": day(dateOf(t, pol["start_date"]), 10), "vehicle_group": "D", "base_version_no": 1}
				if kind == "cancel" {
					path = policyPath(pol) + "/cancel"
					token = uw
					body = map[string]any{"effective_date": pol["start_date"], "reason": "UNDERWRITER", "base_version_no": 1}
				}
			case "claim":
				pol = claimPolicy(t)
				token = adjuster
				path = "/api/v1/claims"
				body = map[string]any{"policy_id": pol["id"], "coverage_code": "KASKO_ANA", "loss_date": caldate.Format(caldate.Today(time.Now())), "description": "Çökme penceresi", "claimed_amount_kurus": 1000}
			}
			key := fmt.Sprintf("recover-%s-%v", kind, pol["id"])
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			status, first := rawPost(t, token, path, key, encoded)
			require.Equal(t, http.StatusCreated, status, string(first))
			status, repeated := rawPost(t, token, path, key, encoded)
			require.Equal(t, http.StatusCreated, status, string(repeated))
			assert.Equal(t, first, repeated, "normal tekrar bayt bayt aynı")
			baseline := financialCounts(t)
			var original map[string]any
			require.NoError(t, json.Unmarshal(first, &original))
			// İş commit edilmiş, middleware yanıtı saklanmadan süreç çökmüş gibi.
			_, err = pool.Exec(context.Background(), `
    UPDATE idempotency_keys SET status='IN_PROGRESS',response_body=NULL,response_status=NULL,
           completed_at=NULL,created_at=now()-interval '5 minutes' WHERE idem_key=$1`, key)
			require.NoError(t, err)
			// Bayat anahtar farklı gövdeyle devralınamaz.
			status, different := rawPost(t, token, path, key, append(encoded, ' '))
			assert.Equal(t, http.StatusUnprocessableEntity, status, string(different))
			status, recovered := rawPost(t, token, path, key, encoded)
			require.Equal(t, http.StatusCreated, status, string(recovered))
			var canonical map[string]any
			require.NoError(t, json.Unmarshal(recovered, &canonical))
			if kind == "endorsement" || kind == "cancel" {
				assert.Equal(t, original["endorsement"].(map[string]any)["id"], canonical["endorsement"].(map[string]any)["id"])
			} else {
				assert.Equal(t, original["id"], canonical["id"])
			}
			assert.Equal(t, baseline, financialCounts(t), "ikinci kaynak, audit veya yevmiye yok")
		})
	}
}
