package policy

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

type Handler struct {
	pool    *pgxpool.Pool
	changes *ChangeService
}

func NewHandler(pool *pgxpool.Pool, now func() time.Time) *Handler {
	return &Handler{pool: pool, changes: NewChangeService(pool, now)}
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ValidationError(w, map[string]string{"id": "geçersiz kimlik"})
		return 0, false
	}
	return id, true
}

// visibleHeader poliçeyi okur; kapsam dışıysa 404 (A2 kuralı).
func (h *Handler) visibleHeader(r *http.Request, id int64) (Header, error) {
	p, _ := auth.FromContext(r.Context())
	sc, err := access.Resolve(r.Context(), h.pool, p)
	if err != nil {
		return Header{}, access.AsHTTP(err)
	}
	hd, err := LoadHeader(r.Context(), h.pool, id, false)
	if errors.Is(err, ErrNotFound) || (err == nil && !sc.Allows(hd.AgentID, hd.CustomerID)) {
		return hd, httpx.NotFound("poliçe")
	}
	return hd, err
}

// Detail GET /api/v1/policies/{id} — poliçe, tüm sürümler, teminatlar, zeyiller ve limit bakiyeleri.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	hd, err := h.visibleHeader(r, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, err := LoadDetail(r.Context(), h.pool, hd)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

type coverageResponse struct {
	PolicyID     int64       `json:"policy_id"`
	PolicyNo     string      `json:"policy_no"`
	PolicyStatus string      `json:"policy_status"`
	Date         string      `json:"date"`
	Version      VersionView `json:"version"`
}

// Coverage GET /api/v1/policies/{id}/coverage?date= — o gün geçerli sürüm ve teminatlar.
func (h *Handler) Coverage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	on, err := caldate.Parse(r.URL.Query().Get("date"))
	if err != nil {
		httpx.ValidationError(w, map[string]string{"date": "zorunlu, YYYY-MM-DD"})
		return
	}
	hd, err := h.visibleHeader(r, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := VersionOn(r.Context(), h.pool, id, hd.Product, on)
	if errors.Is(err, ErrNoCoverage) {
		httpx.Error(w, http.StatusNotFound, "NO_COVERAGE_ON_DATE",
			"bu tarihte geçerli teminat yok (poliçe dönemi dışında veya iptal sonrası)")
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, coverageResponse{PolicyID: hd.ID, PolicyNo: hd.PolicyNo, PolicyStatus: hd.Status,
		Date: caldate.Format(on), Version: v})
}

func (h *Handler) PreviewVehicleChange(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req VehicleChangeRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.changes.PreviewVehicleChange(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) VehicleChange(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req VehicleChangeRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.changes.VehicleChange(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) PreviewCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req CancelRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.changes.PreviewCancel(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req CancelRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	out, err := h.changes.Cancel(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}
