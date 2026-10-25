package task

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	params, bad := page.Parse(r)
	if bad != nil {
		httpx.ValidationError(w, bad)
		return
	}
	q := r.URL.Query()
	f := Filter{Status: q.Get("status"), Type: strings.ToUpper(q.Get("task_type"))}
	if f.Status == "" {
		f.Status = "open"
	}
	if f.Status != "open" && f.Status != "done" && f.Status != "all" {
		httpx.ValidationError(w, map[string]string{"status": "open, done veya all olmalı"})
		return
	}
	if f.Type != "" && f.Type != "APPRAISAL" && f.Type != "ADJUSTMENT" && f.Type != "FRAUD_REVIEW" {
		httpx.ValidationError(w, map[string]string{"task_type": "geçersiz görev tipi"})
		return
	}
	if v := q.Get("claim_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			httpx.ValidationError(w, map[string]string{"claim_id": "pozitif id gerekli"})
			return
		}
		f.ClaimID = id
	}
	if v := q.Get("breached"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			httpx.ValidationError(w, map[string]string{"breached": "boolean olmalı"})
			return
		}
		f.Breached = b
	}
	assigned := q.Get("assigned_to")
	if assigned != "" && assigned != "me" {
		httpx.ValidationError(w, map[string]string{"assigned_to": "yalnızca me desteklenir"})
		return
	}
	actor, _ := auth.FromContext(r.Context())
	if actor.Role == auth.RoleAppraiser || assigned == "me" {
		f.AssignedTo = actor.UserID
	}
	rows, err := List(r.Context(), h.svc.pool, params, f, h.svc.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page.Build(rows, params.Limit, func(v View) int64 { return v.ID }))
}

func (h *Handler) Workload(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Workload(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Summary(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": rows})
}
