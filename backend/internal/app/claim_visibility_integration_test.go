//go:build integration

package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerAndAgentClaimBodiesHideReviewInternals(t *testing.T) {
	defer assertBillingInvariant(t)
	adjuster := tokenFor(t, "hasar@guvenpolice.test")
	for _, email := range []string{"musteri@guvenpolice.test", "acente@guvenpolice.test"} {
		t.Run(email, func(t *testing.T) {
			pol := billingPolicy(t, 5, 1, 100000)
			token := tokenFor(t, email)
			var c map[string]any
			assertPublic := func(body map[string]any) {
				raw, err := json.Marshal(body)
				require.NoError(t, err)
				assert.NotContains(t, strings.ToLower(string(raw)), "fraud")
			}
			for range 3 {
				status, body, _ := postKey(t, token, "/api/v1/claims", map[string]any{
					"policy_id": pol["id"], "coverage_code": "KASKO_ANA", "loss_date": caldate.Format(caldate.Today(time.Now())),
					"description": "Dış rol görünürlük kabulü", "claimed_amount_kurus": 100,
				}, fmt.Sprintf("visibility-%s-%d", email, time.Now().UnixNano()))
				require.Equal(t, http.StatusCreated, status, body)
				assertPublic(body)
				c = body
			}
			assert.Equal(t, "UNDER_REVIEW", c["status"])
			for _, reader := range []string{"musteri@guvenpolice.test", "acente@guvenpolice.test"} {
				s, body := get(t, tokenFor(t, reader), "/api/v1/claims?status=UNDER_REVIEW&limit=100")
				require.Equal(t, http.StatusOK, s, body)
				assertPublic(body)
				assert.NotEmpty(t, body["items"])
			}
			s, internal := get(t, adjuster, claimPath(c))
			require.Equal(t, http.StatusOK, s, internal)
			assert.Equal(t, "FRAUD_REVIEW", internal["status"])
			assert.NotNil(t, internal["fraud"])
			c = internal
			change := func(to, note string) {
				s, body := newClient(t).do(http.MethodPost, claimPath(c)+"/transition", adjuster, map[string]any{
					"to_status": to, "note": note, "expected_row_version": c["row_version"],
				})
				require.Equal(t, http.StatusOK, s, body)
				c = body
			}
			checkDetail := func() map[string]any {
				s, body := get(t, token, claimPath(c))
				require.Equal(t, http.StatusOK, s, body)
				assertPublic(body)
				return body
			}
			checkDetail()
			change("APPRAISAL", "fraud iç temize çıkış notu")
			checkDetail()
			s, body := newClient(t).do(http.MethodPost, claimPath(c)+"/appraisals", tokenFor(t, "eksper@guvenpolice.test"), map[string]any{
				"damage_amount_kurus": 100, "inspected_at": time.Now().UTC().Format(time.RFC3339),
			})
			require.Equal(t, http.StatusCreated, s, body)
			change("ADJUSTMENT", "Değerlendirme")
			change("FRAUD_REVIEW", "fraud iç inceleme notu")
			checkDetail()
			change("REJECTED", "Teminat kapsamı dışında")
			body = checkDetail()
			history := body["history"].([]any)
			assert.Equal(t, "Teminat kapsamı dışında", history[len(history)-1].(map[string]any)["note"])
		})
	}
}
