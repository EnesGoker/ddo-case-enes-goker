package claim

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
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
func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
	var req ReportRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.svc.Report(r.Context(), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/claims/"+strconv.FormatInt(out.ID, 10))
	httpx.JSON(w, 201, out)
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	params, bad := page.Parse(r)
	status := strings.ToUpper(r.URL.Query().Get("status"))
	actor, _ := auth.FromContext(r.Context())
	valid := validStatuses[status]
	if externalRole(actor.Role) {
		valid = (valid && status != "FRAUD_REVIEW") || status == "UNDER_REVIEW"
	}
	if status != "" && !valid {
		if bad == nil {
			bad = map[string]string{}
		}
		bad["status"] = "geçersiz durum"
	}
	if bad != nil {
		httpx.ValidationError(w, bad)
		return
	}
	out, err := h.svc.List(r.Context(), params, status)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, page.Build(out, params.Limit, func(c Claim) int64 { return c.ID }))
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req TransitionRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.svc.ChangeStatus(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
func (h *Handler) Appraise(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req AppraisalRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.svc.Appraise(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 201, out)
}
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Preview(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Approve(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 201, out)
}
func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Pay(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
