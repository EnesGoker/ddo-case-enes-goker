package customer

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

type listResponse struct {
	Items      []Customer `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

// ListHandler GET /api/v1/customers. Rol denetimi router'da (AGENT, UNDERWRITER);
// burada acente yalnızca kendi müşterilerine daraltılır.
func ListHandler(repo *Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.FromContext(r.Context())
		f := ListFilter{Limit: defaultLimit}
		details := map[string]string{}
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > maxLimit {
				details["limit"] = "1-200 arası tam sayı olmalı"
			}
			f.Limit = n
		}
		if v := r.URL.Query().Get("cursor"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				details["cursor"] = "geçersiz imleç"
			}
			f.AfterID = n
		}
		if len(details) > 0 {
			httpx.ValidationError(w, details)
			return
		}
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
		if items == nil {
			items = []Customer{}
		}
		resp := listResponse{Items: items}
		if len(items) == f.Limit {
			c := strconv.FormatInt(items[len(items)-1].ID, 10)
			resp.NextCursor = &c
		}
		httpx.JSON(w, http.StatusOK, resp)
	}
}
