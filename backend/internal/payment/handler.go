package payment

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ValidationError(w, map[string]string{"id": "geçersiz kimlik"})
		return 0, false
	}
	return id, true
}

func (h *Handler) Billing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Billing(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req PayRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.svc.Pay(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req RefundRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.svc.Refund(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AsOf string `json:"as_of"`
		RunOptions
	}
	// Gövdesiz çağrı da bugünün gün sonudur; JSON verilirse ortak katı decoder kullanılır.
	if r.Body != nil && r.ContentLength != 0 && !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.svc.Run(r.Context(), req.AsOf, req.RunOptions)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
