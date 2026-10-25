package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

type ctxKey struct{}

// FromContext Authenticate middleware'inin koyduğu kimliği döner.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// Authenticate "Authorization: Bearer <jwt>" başlığını doğrular; yoksa/geçersizse 401.
func Authenticate(tokens *TokenIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			raw, ok := strings.CutPrefix(h, "Bearer ")
			if !ok || raw == "" {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "kimlik doğrulaması gerekli")
				return
			}
			p, err := tokens.Parse(raw)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "geçersiz veya süresi dolmuş token")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// RequireRoles kimliği doğrulanmış kullanıcının rolü listede değilse 403 döner.
// Authenticate'ten sonra kullanılmalı; kimlik yoksa 401.
func RequireRoles(roles ...Role) func(http.Handler) http.Handler {
	allowed := make(map[Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := FromContext(r.Context())
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "kimlik doğrulaması gerekli")
				return
			}
			if !allowed[p.Role] {
				httpx.Error(w, http.StatusForbidden, httpx.CodeForbidden, "bu işlem için yetkiniz yok")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
