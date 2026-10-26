package policy

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/clock"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/policypdf"
)

// PDF GET /api/v1/policies/{id}/pdf. The route uses the existing policy readers.
func (h *Handler) PDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := h.loadPDFSnapshot(r.Context(), id, clock.Now(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// The DB transaction has ended before the CPU-only render begins.
	raw, err := policypdf.Render(d)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="police-%d.pdf"`, id))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
