package quote

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var req Request
	if !httpx.Decode(w, r, &req) {
		return
	}
	c, err := h.svc.Calculate(r.Context(), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req Request
	if !httpx.Decode(w, r, &req) {
		return
	}
	q, err := h.svc.Create(r.Context(), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/quotes/"+strconv.FormatInt(q.ID, 10))
	httpx.JSON(w, http.StatusCreated, q)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	params, bad := page.Parse(r)
	status := strings.ToUpper(r.URL.Query().Get("status"))
	if status != "" && !validStatuses[status] {
		if bad == nil {
			bad = map[string]string{}
		}
		bad["status"] = "geçersiz durum"
	}
	if bad != nil {
		httpx.ValidationError(w, bad)
		return
	}
	items, err := h.svc.List(r.Context(), params, status)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page.Build(items, params.Limit, func(q Quote) int64 { return q.ID }))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	q, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}

func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req ReviewRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	q, err := h.svc.Review(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}

func (h *Handler) Convert(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	count := 1
	if r.Body != nil {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			httpx.ValidationError(w, map[string]string{"body": "gövde okunamadı"})
			return
		}
		if len(data) > 0 {
			var req struct {
				InstallmentCount *int `json:"installment_count"`
			}
			r.Body = io.NopCloser(strings.NewReader(string(data)))
			if !httpx.Decode(w, r, &req) {
				return
			}
			if req.InstallmentCount != nil {
				count = *req.InstallmentCount
			}
		}
	}
	if count < 1 || count > 12 {
		httpx.ValidationError(w, map[string]string{"installment_count": "1–12 arasında olmalı"})
		return
	}
	p, err := h.svc.Convert(r.Context(), id, count)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, p)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ValidationError(w, map[string]string{"id": "geçersiz kimlik"})
		return 0, false
	}
	return id, true
}
