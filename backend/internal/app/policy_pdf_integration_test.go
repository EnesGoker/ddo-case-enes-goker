//go:build integration

package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/app"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

func readPolicyPDF(t *testing.T, baseURL, path, token string) (int, []byte, http.Header) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path+"/pdf", nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := newClient(t).http.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, raw, res.Header
}

func requirePolicyPDF(t *testing.T, baseURL, path, token string) {
	t.Helper()
	s, raw, headers := readPolicyPDF(t, baseURL, path, token)
	require.Equal(t, http.StatusOK, s, string(raw[:min(len(raw), 120)]))
	require.Equal(t, "application/pdf", headers.Get("Content-Type"))
	require.Contains(t, headers.Get("Content-Disposition"), "attachment; filename=\"police-")
	require.Equal(t, "private, no-store", headers.Get("Cache-Control"))
	require.True(t, bytes.HasPrefix(raw, []byte("%PDF-")))
	require.Contains(t, string(raw), "/ToUnicode")
}

func TestPolicyPDFAccessAndRealForeignPortfolio(t *testing.T) {
	defer assertBillingInvariant(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	pol := issuePolicy(t, agent)
	path := policyPath(pol)
	for _, email := range []string{"musteri@guvenpolice.test", "acente@guvenpolice.test", "underwriter@guvenpolice.test", "hasar@guvenpolice.test", "muhasebe@guvenpolice.test"} {
		t.Run(email, func(t *testing.T) { requirePolicyPDF(t, server.URL, path, tokenFor(t, email)) })
	}
	for _, tc := range []struct {
		token string
		code  int
	}{
		{"", http.StatusUnauthorized},
		{tokenFor(t, "eksper@guvenpolice.test"), http.StatusForbidden},
	} {
		s, raw, _ := readPolicyPDF(t, server.URL, path, tc.token)
		require.Equal(t, tc.code, s, string(raw))
	}
	ctx := context.Background()
	email := fmt.Sprintf("b2-%d@test.invalid", time.Now().UnixNano())
	var userID, agentID, tariffID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO users(email,password_hash,full_name,role)
		SELECT $1,password_hash,'B2 Test Acentesi','AGENT'
		  FROM users WHERE email='acente@guvenpolice.test' RETURNING id`, email).Scan(&userID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO agents(user_id,agency_name) VALUES($1,$2) RETURNING id`, userID, email).Scan(&agentID))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM tariff_versions WHERE product='KONUT' AND version_no=1`).Scan(&tariffID))
	var foreign policy.Issued
	start := caldate.Today(time.Now())
	canID := customerID(t, can)
	require.NoError(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var err error
		foreign, err = policy.Issue(ctx, tx, policy.IssueInput{ActorID: userID, PolicyNoOverride: fmt.Sprintf("B2-%d", time.Now().UnixNano()),
			Product: pricing.Konut, CustomerID: canID, AgentID: &agentID, TariffVersionID: tariffID,
			StartDate: start, EndDate: caldate.AddYear(start), AnnualPremium: 100000, AnnualLimitKurus: 10000000, InstallmentCount: 1,
			RiskData: []byte(`{"region_code":"34"}`), Coverages: []policy.CoverageTerm{{Code: "YANGIN", LimitKurus: 10000000, PremiumKurus: 100000, DeductibleType: pricing.DeductibleNone}}})
		return err
	}))
	foreignPath := fmt.Sprintf("/api/v1/policies/%d", foreign.ID)
	for _, denied := range []string{agent, tokenFor(t, "musteri@guvenpolice.test")} {
		s, raw, _ := readPolicyPDF(t, server.URL, foreignPath, denied)
		require.Equal(t, http.StatusNotFound, s, string(raw))
	}
	requirePolicyPDF(t, server.URL, foreignPath, tokenFor(t, email))
}

func TestPolicyPDFEndorsementCancellationAndNoQuote(t *testing.T) {
	defer assertBillingInvariant(t)
	agent := tokenFor(t, "acente@guvenpolice.test")
	pol := issuePolicy(t, agent)
	path := policyPath(pol)
	start := dateOf(t, pol["start_date"])
	s, body, _ := postKey(t, agent, path+"/endorsements", map[string]any{"effective_date": day(start, 30), "vehicle_group": "D", "base_version_no": 1}, fmt.Sprintf("b2-z-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, body)
	requirePolicyPDF(t, server.URL, path, agent)
	s, body, _ = postKey(t, agent, path+"/cancel", map[string]any{"effective_date": day(start, 60), "reason": "CUSTOMER_REQUEST", "base_version_no": 2}, fmt.Sprintf("b2-c-%v", pol["id"]))
	require.Equal(t, http.StatusCreated, s, body)
	requirePolicyPDF(t, server.URL, path, agent)
	noQuote := billingPolicy(t, 400, 1, 100000)
	requirePolicyPDF(t, server.URL, policyPath(noQuote), agent)
}

func TestPolicyPDFUsesOnePoolConnection(t *testing.T) {
	pol := issuePolicy(t, tokenFor(t, "acente@guvenpolice.test"))
	token := tokenFor(t, "musteri@guvenpolice.test")
	cfg := pool.Config().Copy()
	cfg.MaxConns = 1
	small, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer small.Close()
	srv := httptest.NewServer(app.NewRouter(app.Deps{Pool: small, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		JWTSecret: []byte("integration-secret-at-least-32-bytes"), AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour,
		UWPremiumThresholdKurus: 3000000, IdempotencyStaleAfter: time.Minute}))
	defer srv.Close()
	requirePolicyPDF(t, srv.URL, policyPath(pol), token)
}
