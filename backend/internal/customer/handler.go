package customer

import (
	"errors"
	"net/http"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
)

// ListHandler GET /api/v1/customers. Rol denetimi router'da (AGENT, UNDERWRITER);
// burada acente yalnızca kendi müşterilerine daraltılır.
func ListHandler(repo *Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, bad := page.Parse(r)
		if bad != nil {
			httpx.ValidationError(w, bad)
			return
		}
		p, _ := auth.FromContext(r.Context())
		f := ListFilter{Limit: params.Limit, AfterID: params.AfterID}
		if p.Role == auth.RoleAgent {
			id, err := repo.AgentIDForUser(r.Context(), p.UserID)
			if errors.Is(err, ErrNoAgentProfile) {
				httpx.Error(w, http.StatusForbidden, httpx.CodeForbidden, "acente profili bulunamadı")
				return
			}
			if err != nil {
				httpx.Internal(w, r, err)
				return
			}
			f.AgentID = &id
		}
		items, err := repo.List(r.Context(), f)
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, page.Build(items, params.Limit, func(c Customer) int64 { return c.ID }))
	}
}
