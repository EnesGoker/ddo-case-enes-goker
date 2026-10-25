package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

func TestAuthenticateAndRequireRoles(t *testing.T) {
	now := time.Now()
	iss := fixedIssuer(now)
	token := func(role Role) string {
		s, err := iss.Issue(Principal{UserID: 7, Role: role})
		require.NoError(t, err)
		return "Bearer " + s
	}
	expired, err := fixedIssuer(now.Add(-time.Hour)).Issue(Principal{UserID: 7, Role: RoleAgent})
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(Authenticate(iss))
	r.With(RequireRoles(RoleAgent, RoleUnderwriter)).Get("/customers", func(w http.ResponseWriter, r *http.Request) {
		p, ok := FromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, int64(7), p.UserID)
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name   string
		header string
		status int
		code   string
	}{
		{"başlık yok", "", 401, httpx.CodeUnauthenticated},
		{"Bearer değil", "Basic abc", 401, httpx.CodeUnauthenticated},
		{"boş token", "Bearer ", 401, httpx.CodeUnauthenticated},
		{"bozuk token", "Bearer not.a.jwt", 401, httpx.CodeUnauthenticated},
		{"süresi dolmuş", "Bearer " + expired, 401, httpx.CodeUnauthenticated},
		{"müşteri yetkisiz", token(RoleCustomer), 403, httpx.CodeForbidden},
		{"eksper yetkisiz", token(RoleAppraiser), 403, httpx.CodeForbidden},
		{"hasar uzmanı yetkisiz", token(RoleClaimsAdjuster), 403, httpx.CodeForbidden},
		{"muhasebe yetkisiz", token(RoleAccounting), 403, httpx.CodeForbidden},
		{"acente yetkili", token(RoleAgent), 200, ""},
		{"underwriter yetkili", token(RoleUnderwriter), 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/customers", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			assert.Equal(t, tc.status, rec.Code)
			if tc.code != "" {
				var body httpx.ErrorBody
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, tc.code, body.Error.Code)
				assert.NotEmpty(t, body.Error.Message)
			}
		})
	}
}

func TestRequireRolesWithoutAuthenticateIs401(t *testing.T) {
	h := RequireRoles(RoleAgent)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler çalışmamalı")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
