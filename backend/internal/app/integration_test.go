//go:build integration

// Gerçek PostgreSQL'e karşı uçtan uca testler. DATABASE_URL, CREATE DATABASE yetkisi olan
// bir kullanıcıyı göstermeli; her çalıştırma kendi geçici veritabanını açar ve siler.
//
//	DATABASE_URL=postgres://... go test -tags integration ./...
package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/app"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
)

const (
	migrationsDir = "../../../db/migrations"
	demoPassword  = "Integration-Test-Pass-1"
)

var (
	server *httptest.Server
	pool   *pgxpool.Pool
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	adminURL := os.Getenv("DATABASE_URL")
	if adminURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL gerekli (integration testleri)")
		return 1
	}
	testURL, drop, err := createTempDB(ctx, adminURL, "gp_it")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer drop()

	if err := db.Migrate(ctx, testURL, migrationsDir); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}
	pool, err = db.Connect(ctx, testURL, 10*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	if err := seed.Demo(ctx, pool, demoPassword); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		return 1
	}
	server = httptest.NewServer(app.NewRouter(app.Deps{
		Pool:            pool,
		Logger:          slog.New(slog.NewJSONHandler(io.Discard, nil)),
		JWTSecret:       []byte("integration-secret-at-least-32-bytes"),
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,

		UWPremiumThresholdKurus: 3_000_000,
		IdempotencyStaleAfter:   time.Minute,
	}))
	defer server.Close()
	return m.Run()
}

// createTempDB benzersiz adlı bir veritabanı açar; drop onu siler.
func createTempDB(ctx context.Context, adminURL, prefix string) (string, func(), error) {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return "", nil, err
	}
	name := fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), time.Now().UnixNano()%1_000_000)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		conn.Close(ctx)
		return "", nil, err
	}
	u, _ := url.Parse(adminURL)
	u.Path = "/" + name
	drop := func() {
		_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		conn.Close(context.Background())
	}
	return u.String(), drop, nil
}

type client struct {
	t    *testing.T
	http *http.Client
}

func newClient(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

func (c *client) do(method, path, token string, body any) (int, map[string]any) {
	c.t.Helper()
	status, out, _ := c.doH(method, path, token, body, nil)
	return status, out
}

// doH ek başlıklarla istek atar; yanıt başlıklarını da döner.
func (c *client) doH(method, path, token string, body any, headers map[string]string) (int, map[string]any, http.Header) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, server.URL+path, r)
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	out := map[string]any{}
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out, resp.Header
}

func (c *client) login(email, password string) (int, map[string]any) {
	return c.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": password})
}

func (c *client) refreshCookie() string {
	u, _ := url.Parse(server.URL + "/api/v1/auth/refresh")
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == auth.RefreshCookie {
			return ck.Value
		}
	}
	return ""
}

func (c *client) setRefreshCookie(v string) {
	u, _ := url.Parse(server.URL + "/api/v1/auth/")
	c.http.Jar.SetCookies(u, []*http.Cookie{{Name: auth.RefreshCookie, Value: v, Path: "/api/v1/auth"}})
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func TestHealth(t *testing.T) {
	status, body := newClient(t).do(http.MethodGet, "/health", "", nil)
	assert.Equal(t, 200, status)
	assert.Equal(t, "ok", body["db"])
}

func TestHealthReportsDatabaseDown(t *testing.T) {
	cfg := pool.Config().Copy()
	closed, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	closed.Close()

	srv := httptest.NewServer(app.NewRouter(app.Deps{
		Pool: closed, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		JWTSecret: []byte("integration-secret-at-least-32-bytes"), AccessTokenTTL: time.Minute,
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestAllDemoRolesCanLogin(t *testing.T) {
	for _, u := range seed.DemoUsers {
		t.Run(string(u.Role), func(t *testing.T) {
			c := newClient(t)
			status, body := c.login(u.Email, demoPassword)
			require.Equal(t, 200, status, body)
			token, _ := body["access_token"].(string)
			require.NotEmpty(t, token)
			assert.NotEmpty(t, c.refreshCookie(), "refresh httpOnly cookie ile gelmeli")
			assert.NotContains(t, body, "refresh_token", "refresh token gövdede olmamalı")

			status, me := c.do(http.MethodGet, "/api/v1/auth/me", token, nil)
			require.Equal(t, 200, status)
			assert.Equal(t, string(u.Role), me["role"])
			assert.NotContains(t, me, "password_hash")
		})
	}
}

func TestPasswordsAreBcryptHashed(t *testing.T) {
	var hash string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT password_hash FROM users WHERE email = 'acente@guvenpolice.test'`).Scan(&hash))
	assert.True(t, strings.HasPrefix(hash, "$2a$12$"), "bcrypt cost 12")
	assert.NotContains(t, hash, demoPassword)
}

func TestLoginFailures(t *testing.T) {
	c := newClient(t)

	status, body := c.login("acente@guvenpolice.test", "wrong-password")
	assert.Equal(t, 401, status)
	assert.Equal(t, "INVALID_CREDENTIALS", errCode(body))

	// Bilinmeyen kullanıcı aynı yanıtı alır (kullanıcı varlığı sızmaz)
	status, body2 := c.login("yok@guvenpolice.test", "whatever")
	assert.Equal(t, 401, status)
	assert.Equal(t, body, body2)

	status, body = c.login("not-an-email", "")
	assert.Equal(t, 400, status)
	assert.Equal(t, "VALIDATION_ERROR", errCode(body))
}

func TestCustomersRBAC(t *testing.T) {
	tokenFor := func(email string) string {
		_, body := newClient(t).login(email, demoPassword)
		return body["access_token"].(string)
	}
	c := newClient(t)

	status, body := c.do(http.MethodGet, "/api/v1/customers", "", nil)
	assert.Equal(t, 401, status)
	assert.Equal(t, "UNAUTHENTICATED", errCode(body))

	for _, email := range []string{"musteri@guvenpolice.test", "eksper@guvenpolice.test",
		"hasar@guvenpolice.test", "muhasebe@guvenpolice.test"} {
		status, body = c.do(http.MethodGet, "/api/v1/customers", tokenFor(email), nil)
		assert.Equal(t, 403, status, email)
		assert.Equal(t, "FORBIDDEN", errCode(body), email)
	}

	// Acente yalnızca kendi 4 müşterisini görür; underwriter 5'ini
	status, body = c.do(http.MethodGet, "/api/v1/customers", tokenFor("acente@guvenpolice.test"), nil)
	require.Equal(t, 200, status)
	assert.Len(t, body["items"], 4)
	first := body["items"].([]any)[0].(map[string]any)
	assert.Equal(t, "*******0001", first["national_id_masked"])

	uw := tokenFor("underwriter@guvenpolice.test")
	status, body = c.do(http.MethodGet, "/api/v1/customers", uw, nil)
	require.Equal(t, 200, status)
	assert.Len(t, body["items"], 5)

	// Keyset: limit=2 → imleç ile sonraki sayfa
	status, body = c.do(http.MethodGet, "/api/v1/customers?limit=2", uw, nil)
	require.Equal(t, 200, status)
	assert.Len(t, body["items"], 2)
	cursor, _ := body["next_cursor"].(string)
	require.NotEmpty(t, cursor)
	status, body = c.do(http.MethodGet, "/api/v1/customers?limit=2&cursor="+cursor, uw, nil)
	require.Equal(t, 200, status)
	assert.Len(t, body["items"], 2)

	status, _ = c.do(http.MethodGet, "/api/v1/customers?limit=0", uw, nil)
	assert.Equal(t, 400, status)
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	c := newClient(t)
	status, _ := c.login("underwriter@guvenpolice.test", demoPassword)
	require.Equal(t, 200, status)
	first := c.refreshCookie()

	status, body := c.do(http.MethodPost, "/api/v1/auth/refresh", "", nil)
	require.Equal(t, 200, status, body)
	assert.NotEmpty(t, body["access_token"])
	second := c.refreshCookie()
	require.NotEmpty(t, second)
	assert.NotEqual(t, first, second, "her yenilemede yeni refresh token")

	// Kullanılmış token tekrar gelirse reddedilir ...
	c.setRefreshCookie(first)
	status, body = c.do(http.MethodPost, "/api/v1/auth/refresh", "", nil)
	assert.Equal(t, 401, status)
	assert.Equal(t, "INVALID_REFRESH_TOKEN", errCode(body))

	// ... ve çalınma şüphesiyle o kullanıcının güncel token'ı da iptal edilir
	c.setRefreshCookie(second)
	status, _ = c.do(http.MethodPost, "/api/v1/auth/refresh", "", nil)
	assert.Equal(t, 401, status)

	status, _ = newClient(t).do(http.MethodPost, "/api/v1/auth/refresh", "", nil)
	assert.Equal(t, 401, status, "cookie yoksa 401")
}

func TestConcurrentRefreshOnlyOneSucceeds(t *testing.T) {
	c := newClient(t)
	status, _ := c.login("muhasebe@guvenpolice.test", demoPassword)
	require.Equal(t, 200, status)
	token := c.refreshCookie()

	const n = 5
	var wg sync.WaitGroup
	codes := make(chan int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cc := newClient(t)
			cc.setRefreshCookie(token)
			s, _ := cc.do(http.MethodPost, "/api/v1/auth/refresh", "", nil)
			codes <- s
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for s := range codes {
		if s == 200 {
			ok++
		}
	}
	assert.Equal(t, 1, ok, "aynı refresh token ile yalnızca bir yenileme başarılı olmalı")
}

func TestLogoutRevokesRefresh(t *testing.T) {
	c := newClient(t)
	_, body := c.login("eksper@guvenpolice.test", demoPassword)
	token := body["access_token"].(string)
	saved := c.refreshCookie()

	status, _ := c.do(http.MethodPost, "/api/v1/auth/logout", token, nil)
	assert.Equal(t, 204, status)

	c.setRefreshCookie(saved)
	status, _ = c.do(http.MethodPost, "/api/v1/auth/refresh", "", nil)
	assert.Equal(t, 401, status)
}

func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, seed.Demo(ctx, pool, demoPassword))
	var users, customers int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM customers`).Scan(&customers))
	assert.GreaterOrEqual(t, users, 6, "diğer testler ek kullanıcı açabilir")
	assert.Equal(t, 5, customers)
	var tariffs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM tariff_versions WHERE status = 'ACTIVE'`).Scan(&tariffs))
	assert.Equal(t, 4, tariffs)
}

func TestMetricsExposed(t *testing.T) {
	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(b), "http_requests_total")
}

func TestMigrationsUpDownUp(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_mig")
	require.NoError(t, err)
	defer drop()

	sqlDB, err := sql.Open("pgx", u)
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, goose.SetDialect("postgres"))
	goose.SetLogger(goose.NopLogger())

	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir), "up")
	v, err := goose.GetDBVersionContext(ctx, sqlDB)
	require.NoError(t, err)
	assert.Equal(t, int64(5), v)

	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 0), "down")
	var tables int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'goose_db_version'`).Scan(&tables))
	assert.Equal(t, 0, tables, "down sonrası tablo kalmamalı")

	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir), "tekrar up")
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'goose_db_version'`).Scan(&tables))
	assert.Equal(t, 34, tables)

	// 00002 kısıtları gerçekten devrede
	var n int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM pg_indexes WHERE indexname IN ('journal_entries_reverses_uq','quotes_agent_idx')`).Scan(&n))
	assert.Equal(t, 2, n)
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM pg_constraint WHERE conname IN ('policy_versions_policy_id_id_uq','claims_policy_version_match_fk')`).Scan(&n))
	assert.Equal(t, 2, n)

	// 00003 sürüm koruma tetikleyicileri devrede
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM pg_trigger WHERE tgname IN ('policy_versions_immutable','policy_version_coverages_immutable')`).Scan(&n))
	assert.Equal(t, 2, n)

	// 00005 tek başına geri alınıp tekrar uygulanabilir
	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 4), "00005 down")
	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir), "00005 tekrar up")

	// 00004 tek başına geri alınıp tekrar uygulanabilir
	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 3), "00004 down")
	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir), "00004 tekrar up")

	// 00003 tek başına geri alınıp tekrar uygulanabilir
	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 2), "00003 down")
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM pg_trigger WHERE tgname IN ('policy_versions_immutable','policy_version_coverages_immutable')`).Scan(&n))
	assert.Equal(t, 0, n)
	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir), "00003 tekrar up")
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM pg_trigger WHERE tgname IN ('policy_versions_immutable','policy_version_coverages_immutable')`).Scan(&n))
	assert.Equal(t, 2, n)
}
