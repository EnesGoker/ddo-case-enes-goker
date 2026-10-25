package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSecret = []byte("test-secret-at-least-32-bytes-long!!")

func fixedIssuer(now time.Time) *TokenIssuer {
	t := NewTokenIssuer(testSecret, 15*time.Minute)
	t.now = func() time.Time { return now }
	return t
}

func TestTokenRoundTrip(t *testing.T) {
	iss := fixedIssuer(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	tok, err := iss.Issue(Principal{UserID: 42, Role: RoleAgent})
	require.NoError(t, err)

	p, err := iss.Parse(tok)
	require.NoError(t, err)
	assert.Equal(t, Principal{UserID: 42, Role: RoleAgent}, p)
}

func TestTokenExpired(t *testing.T) {
	start := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	tok, err := fixedIssuer(start).Issue(Principal{UserID: 1, Role: RoleCustomer})
	require.NoError(t, err)

	// Sınır: tam 15 dk sonra süresi dolmuş sayılır
	_, err = fixedIssuer(start.Add(15 * time.Minute)).Parse(tok)
	assert.ErrorIs(t, err, ErrInvalidToken)

	_, err = fixedIssuer(start.Add(15*time.Minute - time.Second)).Parse(tok)
	assert.NoError(t, err)
}

func TestTokenTamperedSignature(t *testing.T) {
	iss := fixedIssuer(time.Now())
	tok, err := iss.Issue(Principal{UserID: 1, Role: RoleCustomer})
	require.NoError(t, err)

	// İlk imza karakterini değiştir: son Base64 karakterinin kullanılmayan
	// bitlerini değiştirmek aynı imza baytlarına çözülebilir.
	b := []byte(tok)
	signatureStart := strings.LastIndexByte(tok, '.') + 1
	if b[signatureStart] == 'A' {
		b[signatureStart] = 'B'
	} else {
		b[signatureStart] = 'A'
	}
	_, err = iss.Parse(string(b))
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestTokenWrongSecret(t *testing.T) {
	tok, err := fixedIssuer(time.Now()).Issue(Principal{UserID: 1, Role: RoleAccounting})
	require.NoError(t, err)

	other := NewTokenIssuer([]byte("another-secret-at-least-32-bytes!!!"), time.Minute)
	_, err = other.Parse(tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestTokenRejectsNoneAlgorithm(t *testing.T) {
	c := claims{Role: RoleAccounting, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: issuer, Subject: "1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, c).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = fixedIssuer(time.Now()).Parse(tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestTokenRejectsUnknownRoleAndIssuer(t *testing.T) {
	sign := func(c claims) string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(testSecret)
		require.NoError(t, err)
		return s
	}
	exp := jwt.NewNumericDate(time.Now().Add(time.Hour))
	iss := fixedIssuer(time.Now())

	_, err := iss.Parse(sign(claims{Role: "ADMIN", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: issuer, Subject: "1", ExpiresAt: exp}}))
	assert.ErrorIs(t, err, ErrInvalidToken, "bilinmeyen rol")

	_, err = iss.Parse(sign(claims{Role: RoleAgent, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "someone-else", Subject: "1", ExpiresAt: exp}}))
	assert.ErrorIs(t, err, ErrInvalidToken, "yanlış issuer")

	_, err = iss.Parse(sign(claims{Role: RoleAgent, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: issuer, Subject: "1"}}))
	assert.ErrorIs(t, err, ErrInvalidToken, "exp zorunlu")
}

func TestRefreshTokenHashing(t *testing.T) {
	raw1, hash1, err := newRefreshToken()
	require.NoError(t, err)
	raw2, _, err := newRefreshToken()
	require.NoError(t, err)

	assert.NotEqual(t, raw1, raw2)
	assert.Len(t, raw1, 43, "32 bayt base64url")
	assert.Equal(t, hash1, hashToken(raw1))
	assert.NotContains(t, hash1, raw1, "veritabanına ham token yazılmaz")
}
