package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("e-posta veya parola hatalı")
	ErrInvalidRefresh     = errors.New("geçersiz refresh token")
)

// BcryptCost demo seed ve parola belirleme için kullanılan maliyet.
const BcryptCost = 12

// dummyHash bilinmeyen e-postada da bcrypt karşılaştırması yapıp zamanlama farkıyla
// kullanıcı varlığının sızmasını önler.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("guvenpolice-dummy-password"), BcryptCost)

type Session struct {
	AccessToken  string
	ExpiresIn    time.Duration
	RefreshToken string
	RefreshExp   time.Time
	User         User
}

type Service struct {
	pool       *pgxpool.Pool
	repo       *Repo
	tokens     *TokenIssuer
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(pool *pgxpool.Pool, tokens *TokenIssuer, refreshTTL time.Duration) *Service {
	return &Service{pool: pool, repo: NewRepo(pool), tokens: tokens, refreshTTL: refreshTTL, now: time.Now}
}

func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	u, err := s.repo.UserByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil || !u.IsActive {
		return Session{}, ErrInvalidCredentials
	}
	raw, hash, err := newRefreshToken()
	if err != nil {
		return Session{}, err
	}
	exp := s.now().Add(s.refreshTTL)
	if err := s.repo.CreateRefresh(ctx, u.ID, hash, exp); err != nil {
		return Session{}, err
	}
	return s.session(u, raw, exp)
}

// Refresh refresh token'ı tek kullanımlık olarak döndürür (rotation). İptal edilmiş
// bir token tekrar gelirse çalınmış sayılır ve kullanıcının tüm refresh token'ları iptal edilir.
func (s *Service) Refresh(ctx context.Context, raw string) (Session, error) {
	if raw == "" {
		return Session{}, ErrInvalidRefresh
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	t, err := lockRefresh(ctx, tx, hashToken(raw))
	if errors.Is(err, ErrNotFound) {
		return Session{}, ErrInvalidRefresh
	}
	if err != nil {
		return Session{}, err
	}
	if t.RevokedAt != nil {
		if err := revokeAllForUser(ctx, tx, t.UserID); err != nil {
			return Session{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Session{}, err
		}
		return Session{}, ErrInvalidRefresh
	}
	if !s.now().Before(t.ExpiresAt) {
		return Session{}, ErrInvalidRefresh
	}
	u, err := scanUser(tx.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, t.UserID))
	if err != nil {
		return Session{}, err
	}
	if !u.IsActive {
		return Session{}, ErrInvalidRefresh
	}
	if err := revokeRefresh(ctx, tx, t.ID); err != nil {
		return Session{}, err
	}
	newRaw, newHash, err := newRefreshToken()
	if err != nil {
		return Session{}, err
	}
	exp := s.now().Add(s.refreshTTL)
	if err := insertRefresh(ctx, tx, u.ID, newHash, exp); err != nil {
		return Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	return s.session(u, newRaw, exp)
}

// Logout yalnızca isteği yapan kullanıcıya ait refresh token'ı iptal eder.
func (s *Service) Logout(ctx context.Context, userID int64, raw string) error {
	if raw == "" {
		return nil
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockRefresh(ctx, tx, hashToken(raw))
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if t.UserID != userID {
			return nil
		}
		return revokeRefresh(ctx, tx, t.ID)
	})
}

func (s *Service) Me(ctx context.Context, userID int64) (User, error) {
	return s.repo.UserByID(ctx, userID)
}

func (s *Service) session(u User, raw string, exp time.Time) (Session, error) {
	at, err := s.tokens.Issue(Principal{UserID: u.ID, Role: u.Role})
	if err != nil {
		return Session{}, err
	}
	return Session{AccessToken: at, ExpiresIn: s.tokens.TTL(), RefreshToken: raw, RefreshExp: exp, User: u}, nil
}

// newRefreshToken 256 bit rastgele token üretir; veritabanında yalnızca SHA-256 özeti tutulur.
func newRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
