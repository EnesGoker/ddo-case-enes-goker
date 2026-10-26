package report

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentPerformanceGolden(t *testing.T) {
	// Issue + positive/negative endorsement; issue minus cancellation refund.
	// Cash payments/refunds are not 4000/5000 movements and are excluded by SQL.
	rows := []Row{
		{Key: "1", Label: "A", PolicyCount: 1, WrittenPremium: 1110900 + 240838 - 353582, IncurredClaims: 50000 + 10000},
		{Key: "2", Label: "B", PolicyCount: 1, WrittenPremium: 20000 - 18301},
		{Key: "3", Label: "Hareketsiz"},
		{Key: "direct", Label: "Doğrudan", PolicyCount: 1, WrittenPremium: 30000},
	}
	out, err := agentResult(AgentFilter{}, rows)
	require.NoError(t, err)
	require.NotNil(t, out.Total)
	assert.Equal(t, int64(1029855), out.Total.WrittenPremium)
	assert.Equal(t, int64(3), out.Total.PolicyCount)
	assert.Equal(t, int64(60000), out.Total.IncurredClaims)
	assert.Equal(t, int64(583), *out.Total.LossRatioBP)
	assert.Equal(t, int64(601), *out.Items[0].LossRatioBP)
	assert.Nil(t, out.Items[2].LossRatioBP)
	zero, err := agentResult(AgentFilter{}, []Row{{Key: "1", IncurredClaims: 50000}})
	require.NoError(t, err)
	assert.Nil(t, zero.Items[0].LossRatioBP)
	assert.Nil(t, zero.Total.LossRatioBP)
}

func TestAgentPerformanceProjectionAndOverflow(t *testing.T) {
	id := int64(12)
	out, err := agentResult(AgentFilter{AgentID: &id}, []Row{
		{Key: "12", WrittenPremium: 100, IncurredClaims: 25},
		{Key: "13", WrittenPremium: 900},
		{Key: "direct", WrittenPremium: 1000},
	})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	assert.Equal(t, "12", out.Items[0].Key)
	assert.Nil(t, out.Total)
	body, err := json.Marshal(out)
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"total"`)
	assert.NotContains(t, string(body), `"direct"`)
	_, err = agentResult(AgentFilter{}, []Row{{WrittenPremium: math.MaxInt64}, {WrittenPremium: 1}})
	require.Error(t, err)
}

func TestAgentPeriodDefaultsAndOpenBounds(t *testing.T) {
	now := time.Date(2024, 2, 28, 22, 0, 0, 0, time.UTC) // TRT February 29.
	for _, tc := range []struct{ query, from, to string }{
		{"", "2023-03-01", "2024-03-01"},
		{"from=&to=", "", ""},
		{"from=2024-02-29", "2024-02-29", ""},
		{"to=2024-03-01", "", "2024-03-01"},
		{"from=2024-02-29&to=2024-03-01", "2024-02-29", "2024-03-01"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			query, err := url.ParseQuery(tc.query)
			require.NoError(t, err)
			filter, err := AgentPeriod(query, now)
			require.NoError(t, err)
			if tc.from == "" {
				assert.Nil(t, filter.From)
			} else {
				assert.Equal(t, tc.from, caldate.Format(*filter.From))
			}
			if tc.to == "" {
				assert.Nil(t, filter.To)
			} else {
				assert.Equal(t, tc.to, caldate.Format(*filter.To))
			}
		})
	}
	for _, query := range []string{"from=bad", "to=2026-02-30", "from=2026-10-26&to=2026-10-26", "from=2026-10-27&to=2026-10-26"} {
		values, err := url.ParseQuery(query)
		require.NoError(t, err)
		_, err = AgentPeriod(values, now)
		require.Error(t, err)
	}
}

func TestAgentPerformanceDeniedRolesWithoutDatabase(t *testing.T) {
	for _, role := range []auth.Role{"", auth.RoleCustomer, auth.RoleAppraiser, auth.RoleClaimsAdjuster} {
		t.Run(string(role), func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/reports/agents", nil)
			want := http.StatusUnauthorized
			if role != "" {
				request = request.WithContext(auth.WithPrincipal(request.Context(), auth.Principal{Role: role, UserID: 1}))
				want = http.StatusForbidden
			}
			response := httptest.NewRecorder()
			AgentPerformanceHandler(nil, time.Now).ServeHTTP(response, request)
			assert.Equal(t, want, response.Code)
		})
	}
}
