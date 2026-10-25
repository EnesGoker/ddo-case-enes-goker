package auth

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/metrics"
)

const (
	RefreshCookie     = "gp_refresh"
	refreshCookiePath = "/api/v1/auth"
)

type Handler struct {
	svc          *Service
	tokens       *TokenIssuer
	cookieSecure bool
	metrics      *metrics.Registry
}

func NewHandler(svc *Service, tokens *TokenIssuer, cookieSecure bool, m *metrics.Registry) *Handler {
	m.Describe("auth_login_total", "counter", "Giriş denemeleri (sonuca göre)")
	return &Handler{svc: svc, tokens: tokens, cookieSecure: cookieSecure, metrics: m}
}

// Routes /api/v1/auth altına bağlanır.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Group(func(r chi.Router) {
		r.Use(Authenticate(h.tokens))
		r.Post("/logout", h.logout)
		r.Get("/me", h.me)
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	User        User   `json:"user"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	details := map[string]string{}
	if _, err := mail.ParseAddress(req.Email); err != nil || len(req.Email) > 254 {
		details["email"] = "geçerli bir e-posta adresi olmalı"
	}
	if req.Password == "" || len(req.Password) > 72 {
		details["password"] = "boş olmamalı, en fazla 72 bayt"
	}
	if len(details) > 0 {
		httpx.ValidationError(w, details)
		return
	}
	s, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		h.metrics.Add("auth_login_total", 1, "result", "invalid_credentials")
		httpx.Error(w, http.StatusUnauthorized, httpx.CodeInvalidCredentials, "e-posta veya parola hatalı")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	h.metrics.Add("auth_login_total", 1, "result", "success")
	h.writeSession(w, s)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	raw := ""
	if c, err := r.Cookie(RefreshCookie); err == nil {
		raw = c.Value
	}
	s, err := h.svc.Refresh(r.Context(), raw)
	if errors.Is(err, ErrInvalidRefresh) {
		h.clearCookie(w)
		httpx.Error(w, http.StatusUnauthorized, httpx.CodeInvalidRefresh, "oturum yenilenemedi, tekrar giriş yapın")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	h.writeSession(w, s)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := FromContext(r.Context())
	raw := ""
	if c, err := r.Cookie(RefreshCookie); err == nil {
		raw = c.Value
	}
	if err := h.svc.Logout(r.Context(), p.UserID, raw); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, _ := FromContext(r.Context())
	u, err := h.svc.Me(r.Context(), p.UserID)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "kullanıcı bulunamadı")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) writeSession(w http.ResponseWriter, s Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookie,
		Value:    s.RefreshToken,
		Path:     refreshCookiePath,
		Expires:  s.RefreshExp,
		MaxAge:   int(time.Until(s.RefreshExp).Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, sessionResponse{
		AccessToken: s.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.ExpiresIn.Seconds()),
		User:        s.User,
	})
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookie,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}
