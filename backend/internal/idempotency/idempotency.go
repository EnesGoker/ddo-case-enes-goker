// Package idempotency kritik POST uçları için Idempotency-Key middleware'idir (idempotency_keys tablosu).
//
// Akış: anahtar IN_PROGRESS olarak eklenir → handler çalışır → 2xx/4xx yanıt COMPLETED olarak
// saklanır (5xx'te kayıt silinir, tekrar denenebilir). Aynı anahtar + aynı istek → saklı yanıt
// aynen döner; aynı anahtar + farklı istek → 422; işlem sürerken → 409.
//
// Bilinen sınır: handler'ın iş transaction'ı ile yanıtın COMPLETED yazılması ayrı adımlardır.
// Arada süreç çökerse kayıt IN_PROGRESS kalır; staleAfter süresinden sonra istek yeniden işlenir
// ve iş kuralları (ör. teklif zaten CONVERTED) ikinci etkiyi engeller.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

const (
	Header         = "Idempotency-Key"
	ReplayedHeader = "Idempotent-Replayed"
	maxKeyLen      = 255
	maxBody        = 1 << 20
)

type Store struct {
	pool       *pgxpool.Pool
	staleAfter time.Duration
}

func NewStore(pool *pgxpool.Pool, staleAfter time.Duration) *Store {
	if staleAfter <= 0 {
		staleAfter = 60 * time.Second
	}
	return &Store{pool: pool, staleAfter: staleAfter}
}

type stored struct {
	requestHash string
	status      string
	respStatus  *int
	respBody    []byte
	stale       bool
}

// Middleware Authenticate'ten sonra kullanılmalı (anahtar kullanıcı başınadır).
func (s *Store) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.FromContext(r.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "kimlik doğrulaması gerekli")
			return
		}
		key := r.Header.Get(Header)
		if key == "" || len(key) > maxKeyLen {
			httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED",
				"bu işlem için 1-255 karakterlik Idempotency-Key başlığı zorunlu")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, httpx.CodeValidation, "istek gövdesi okunamadı")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		hash := RequestHash(r.Method, r.URL.Path, body)

		acquired, prev, err := s.acquire(r.Context(), p.UserID, key, r.Method, r.URL.Path, hash)
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		if !acquired {
			switch {
			case prev.requestHash != hash:
				httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED",
					"bu Idempotency-Key farklı bir istek için kullanılmış")
			case prev.status == "COMPLETED" && prev.respStatus != nil:
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set(ReplayedHeader, "true")
				w.WriteHeader(*prev.respStatus)
				_, _ = w.Write(prev.respBody)
			default:
				httpx.Error(w, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS",
					"aynı Idempotency-Key ile bir istek hâlâ işleniyor")
			}
			return
		}

		rec := &recorder{header: w.Header()}
		finalize := context.WithoutCancel(r.Context())
		defer func() {
			if v := recover(); v != nil {
				s.release(finalize, p.UserID, key)
				panic(v)
			}
		}()
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		if rec.status >= 500 {
			s.release(finalize, p.UserID, key)
		} else if err := s.complete(finalize, p.UserID, key, rec.status, rec.buf.Bytes()); err != nil {
			slog.ErrorContext(r.Context(), "idempotency yanıtı saklanamadı", "err", err)
		}
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.buf.Bytes())
	})
}

// RequestHash method + path + gövdenin SHA-256 özetidir.
func RequestHash(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{'\n'})
	h.Write([]byte(path))
	h.Write([]byte{'\n'})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) acquire(ctx context.Context, userID int64, key, method, path, hash string) (bool, stored, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, idem_key, method, path, request_hash, status)
		VALUES ($1, $2, $3, $4, $5, 'IN_PROGRESS')
		ON CONFLICT (user_id, idem_key) DO NOTHING`, userID, key, method, path, hash)
	if err != nil {
		return false, stored{}, err
	}
	if tag.RowsAffected() == 1 {
		return true, stored{}, nil
	}
	var st stored
	err = s.pool.QueryRow(ctx, `
		SELECT request_hash, status, response_status, response_body,
		       created_at < now() - make_interval(secs => $3)
		  FROM idempotency_keys WHERE user_id = $1 AND idem_key = $2`,
		userID, key, s.staleAfter.Seconds()).
		Scan(&st.requestHash, &st.status, &st.respStatus, &st.respBody, &st.stale)
	if errors.Is(err, pgx.ErrNoRows) {
		// Arada silindi (5xx sonrası); bir kez daha dene
		return s.acquire(ctx, userID, key, method, path, hash)
	}
	if err != nil {
		return false, stored{}, err
	}
	if st.requestHash == hash && st.status == "IN_PROGRESS" && st.stale {
		// Takılı kalmış kaydı devral; aynı anda yalnızca bir istek devralabilir
		tag, err := s.pool.Exec(ctx, `
			UPDATE idempotency_keys SET created_at = now()
			 WHERE user_id = $1 AND idem_key = $2 AND status = 'IN_PROGRESS'
			   AND created_at < now() - make_interval(secs => $3)`,
			userID, key, s.staleAfter.Seconds())
		if err != nil {
			return false, stored{}, err
		}
		if tag.RowsAffected() == 1 {
			return true, stored{}, nil
		}
	}
	return false, st, nil
}

func (s *Store) complete(ctx context.Context, userID int64, key string, status int, body []byte) error {
	var stored any
	if len(body) > 0 {
		if json.Valid(body) {
			stored = json.RawMessage(body)
		} else {
			stored = string(body)
		}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE idempotency_keys
		   SET status = 'COMPLETED', response_status = $3, response_body = $4, completed_at = now()
		 WHERE user_id = $1 AND idem_key = $2`, userID, key, status, stored)
	return err
}

func (s *Store) release(ctx context.Context, userID int64, key string) {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM idempotency_keys WHERE user_id = $1 AND idem_key = $2 AND status = 'IN_PROGRESS'`,
		userID, key); err != nil {
		slog.ErrorContext(ctx, "idempotency kaydı bırakılamadı", "err", err)
	}
}

// recorder handler yanıtını tamponlar; başlıklar gerçek yazıcıya doğrudan yazılır.
type recorder struct {
	header http.Header
	status int
	buf    bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.buf.Write(b)
}
