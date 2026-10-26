//go:build integration

package app_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsRequiresFinancialRole(t *testing.T) {
	for _, tc := range []struct {
		name   string
		email  string
		status int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"customer", "musteri", http.StatusForbidden},
		{"agent", "acente", http.StatusForbidden},
		{"appraiser", "eksper", http.StatusForbidden},
		{"adjuster", "hasar", http.StatusForbidden},
		{"accounting", "muhasebe", http.StatusOK},
		{"underwriter", "underwriter", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/metrics", nil)
			require.NoError(t, err)
			if tc.email != "" {
				req.Header.Set("Authorization", "Bearer "+tokenFor(t, tc.email+"@guvenpolice.test"))
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tc.status, resp.StatusCode, string(body))
			if tc.status == http.StatusOK {
				assert.Contains(t, string(body), "http_requests_total")
			} else {
				assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
				assert.NotContains(t, string(body), "FRAUD_REVIEW")
				assert.NotContains(t, string(body), "guvenpolice_claim_tasks_")
			}
		})
	}
}
