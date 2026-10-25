package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role, users.role CHECK kısıtındaki değerlerle birebir aynıdır.
type Role string

const (
	RoleCustomer       Role = "CUSTOMER"
	RoleAgent          Role = "AGENT"
	RoleUnderwriter    Role = "UNDERWRITER"
	RoleAppraiser      Role = "APPRAISER"
	RoleClaimsAdjuster Role = "CLAIMS_ADJUSTER"
	RoleAccounting     Role = "ACCOUNTING"
)

func (r Role) Valid() bool {
	switch r {
	case RoleCustomer, RoleAgent, RoleUnderwriter, RoleAppraiser, RoleClaimsAdjuster, RoleAccounting:
		return true
	}
	return false
}

const issuer = "guvenpolice"

var ErrInvalidToken = errors.New("geçersiz veya süresi dolmuş token")

// Principal kimliği doğrulanmış istek sahibidir.
type Principal struct {
	UserID int64
	Role   Role
}

type claims struct {
	Role Role `json:"role"`
	jwt.RegisteredClaims
}

// TokenIssuer HS256 access token üretir ve doğrular.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenIssuer(secret []byte, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: secret, ttl: ttl, now: time.Now}
}

func (t *TokenIssuer) TTL() time.Duration { return t.ttl }

func (t *TokenIssuer) Issue(p Principal) (string, error) {
	now := t.now()
	c := claims{
		Role: p.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   strconv.FormatInt(p.UserID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
}

func (t *TokenIssuer) Parse(token string) (Principal, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c,
		func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(t.now),
	)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil || id <= 0 || !c.Role.Valid() {
		return Principal{}, ErrInvalidToken
	}
	return Principal{UserID: id, Role: c.Role}, nil
}
