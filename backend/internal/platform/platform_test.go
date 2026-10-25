package platform_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/logging"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/metrics"
)

func TestErrorFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.ValidationError(rec, map[string]string{"email": "zorunlu"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.JSONEq(t,
		`{"error":{"code":"VALIDATION_ERROR","message":"istek doğrulanamadı","details":{"email":"zorunlu"}}}`,
		rec.Body.String())
}

func TestDecodeRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	var dst struct {
		Email string `json:"email"`
	}
	for _, body := range []string{`{"email":"a","extra":1}`, `{"email":"a"}{"email":"b"}`, `not json`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		assert.False(t, httpx.Decode(rec, req, &dst), body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"x@y.z"}`))
	require.True(t, httpx.Decode(rec, req, &dst))
	assert.Equal(t, "x@y.z", dst.Email)
}

func TestRequestID(t *testing.T) {
	var seen string
	h := logging.RequestIDMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = logging.RequestID(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(logging.RequestIDHeader, "abc-123")
	h.ServeHTTP(rec, req)
	assert.Equal(t, "abc-123", seen)
	assert.Equal(t, "abc-123", rec.Header().Get(logging.RequestIDHeader))

	// Geçersiz (log enjeksiyonu riski) değer yerine yenisi üretilir
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(logging.RequestIDHeader, "bad id\nwith newline")
	h.ServeHTTP(rec, req)
	assert.Len(t, seen, 24)
	assert.Equal(t, seen, rec.Header().Get(logging.RequestIDHeader))
}

func TestMetricsUseRoutePatternAndPrometheusFormat(t *testing.T) {
	reg := metrics.NewRegistry()
	r := chi.NewRouter()
	r.Use(reg.HTTPMiddleware)
	r.Get("/items/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	r.Get("/metrics", reg.Handler())

	for _, id := range []string{"1", "2", "3"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/"+id, nil))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	out := rec.Body.String()
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, out, "# TYPE http_requests_total counter")
	assert.Contains(t, out, `http_requests_total{method="GET",route="/items/{id}",status="418"} 3`)
	assert.NotContains(t, out, "/items/1", "ham yol etiket olmamalı")
}

func TestErrorBodyIsValidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.Error(rec, http.StatusForbidden, httpx.CodeForbidden, `tırnak " ve \ içerir`)
	var body httpx.ErrorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, httpx.CodeForbidden, body.Error.Code)
}
