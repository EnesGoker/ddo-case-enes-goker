// Acente panosu, ADR-18 hareketlerini kaynak başına toplayarak okur.
package report

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/access"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/lossratio"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentFilter struct {
	From    *time.Time
	To      *time.Time
	AgentID *int64
}

type AgentPerformance struct {
	From  *string `json:"from"`
	To    *string `json:"to"`
	Items []Row   `json:"items"`
	Total *Row    `json:"total,omitempty"`
}

const agentPerformanceSQL = `WITH issued AS (
    SELECT p.agent_id, count(*)::bigint AS policies,
           sum(l.credit_kurus-l.debit_kurus) AS premium
      FROM journal_entries e
      JOIN journal_lines l ON l.entry_id=e.id
      JOIN accounts a ON a.id=l.account_id AND a.code='4000'
      JOIN policies p ON p.id=e.ref_id
     WHERE e.ref_type='POLICY_ISSUE'
       AND ($1::date IS NULL OR e.entry_date >= $1)
       AND ($2::date IS NULL OR e.entry_date < $2)
       AND ($3::bigint IS NULL OR p.agent_id=$3)
     GROUP BY p.agent_id
), changed AS (
    SELECT p.agent_id, sum(l.credit_kurus-l.debit_kurus) AS premium
      FROM journal_entries e
      JOIN journal_lines l ON l.entry_id=e.id
      JOIN accounts a ON a.id=l.account_id AND a.code='4000'
      JOIN endorsements z ON z.id=e.ref_id
      JOIN policies p ON p.id=z.policy_id
     WHERE e.ref_type='ENDORSEMENT'
       AND ($1::date IS NULL OR e.entry_date >= $1)
       AND ($2::date IS NULL OR e.entry_date < $2)
       AND ($3::bigint IS NULL OR p.agent_id=$3)
     GROUP BY p.agent_id
), incurred AS (
    SELECT p.agent_id, sum(l.debit_kurus-l.credit_kurus) AS claims
      FROM journal_entries e
      JOIN journal_lines l ON l.entry_id=e.id
      JOIN accounts a ON a.id=l.account_id AND a.code='5000'
      JOIN claims c ON c.id=e.ref_id
      JOIN policies p ON p.id=c.policy_id
     WHERE e.ref_type='CLAIM_APPROVAL'
       AND ($1::date IS NULL OR e.entry_date >= $1)
       AND ($2::date IS NULL OR e.entry_date < $2)
       AND ($3::bigint IS NULL OR p.agent_id=$3)
     GROUP BY p.agent_id
), agencies AS (
    SELECT id, agency_name AS label FROM agents
     WHERE $3::bigint IS NULL OR id=$3
    UNION ALL
    SELECT NULL::bigint, 'Acentesiz'
     WHERE $3::bigint IS NULL
       AND EXISTS (SELECT 1 FROM policies WHERE agent_id IS NULL)
)
SELECT coalesce(a.id::text,'unassigned') AS key, a.label,
       (coalesce(i.premium,0)+coalesce(z.premium,0))::bigint AS written_premium_kurus,
       coalesce(i.policies,0)::bigint AS policy_count,
       coalesce(c.claims,0)::bigint AS incurred_claims_kurus
  FROM agencies a
  LEFT JOIN issued i ON i.agent_id IS NOT DISTINCT FROM a.id
  LEFT JOIN changed z ON z.agent_id IS NOT DISTINCT FROM a.id
  LEFT JOIN incurred c ON c.agent_id IS NOT DISTINCT FROM a.id
 ORDER BY a.id NULLS LAST`

// AgentPeriod uses the existing entry_date [from,to) rules. Explicit empty bounds
// are open; only an omitted pair selects the last twelve calendar months.
func AgentPeriod(query url.Values, now time.Time) (AgentFilter, error) {
	var filter AgentFilter
	if !query.Has("from") && !query.Has("to") {
		to := caldate.Today(now).AddDate(0, 0, 1)
		from := caldate.AddMonths(to, -12)
		filter.From, filter.To = &from, &to
		return filter, nil
	}
	for _, bound := range []struct {
		key    string
		target **time.Time
	}{{"from", &filter.From}, {"to", &filter.To}} {
		if raw := query.Get(bound.key); raw != "" {
			date, err := caldate.Parse(raw)
			if err != nil {
				return filter, httpx.NewError(http.StatusUnprocessableEntity, "INVALID_PERIOD", "tarihler YYYY-MM-DD biçiminde olmalı")
			}
			*bound.target = &date
		}
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return filter, httpx.NewError(http.StatusUnprocessableEntity, "INVALID_PERIOD", "from, to tarihinden önce olmalı")
	}
	return filter, nil
}

func dateLabel(date *time.Time) *string {
	if date == nil {
		return nil
	}
	value := caldate.Format(*date)
	return &value
}

// agentResult also enforces the row projection, so an agent never receives a
// global total or an unrelated row even if the database adapter returns one.
func agentResult(filter AgentFilter, rows []Row) (AgentPerformance, error) {
	out := AgentPerformance{From: dateLabel(filter.From), To: dateLabel(filter.To), Items: []Row{}}
	var premium, policies, claims big.Int
	for _, row := range rows {
		if filter.AgentID != nil && row.Key != strconv.FormatInt(*filter.AgentID, 10) {
			continue
		}
		row.LossRatioBP = lossratio.LossRatioBP(row.IncurredClaims, row.WrittenPremium)
		out.Items = append(out.Items, row)
		premium.Add(&premium, big.NewInt(row.WrittenPremium))
		policies.Add(&policies, big.NewInt(row.PolicyCount))
		claims.Add(&claims, big.NewInt(row.IncurredClaims))
	}
	if filter.AgentID == nil {
		if !premium.IsInt64() || !policies.IsInt64() || !claims.IsInt64() {
			return out, fmt.Errorf("acente panosu toplamı int64 sınırını aşıyor")
		}
		out.Total = &Row{Key: "total", Label: "Toplam", WrittenPremium: premium.Int64(), PolicyCount: policies.Int64(), IncurredClaims: claims.Int64()}
		out.Total.LossRatioBP = lossratio.LossRatioBP(out.Total.IncurredClaims, out.Total.WrittenPremium)
	}
	return out, nil
}

func ReadAgentPerformance(ctx context.Context, q db.DBTX, filter AgentFilter) (AgentPerformance, error) {
	rows, err := q.Query(ctx, agentPerformanceSQL, filter.From, filter.To, filter.AgentID)
	if err != nil {
		return AgentPerformance{}, err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Row, error) {
		var row Row
		err := r.Scan(&row.Key, &row.Label, &row.WrittenPremium, &row.PolicyCount, &row.IncurredClaims)
		return row, err
	})
	if err != nil {
		return AgentPerformance{}, err
	}
	return agentResult(filter, items)
}

func AgentPerformanceHandler(pool *pgxpool.Pool, now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.FromContext(r.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "kimlik doğrulaması gerekli")
			return
		}
		switch principal.Role {
		case auth.RoleAgent, auth.RoleUnderwriter, auth.RoleAccounting:
		default:
			httpx.Error(w, http.StatusForbidden, httpx.CodeForbidden, "bu işlem için yetkiniz yok")
			return
		}
		filter, err := AgentPeriod(r.URL.Query(), now())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		scope, err := access.Resolve(r.Context(), pool, principal)
		if err != nil {
			httpx.Fail(w, r, access.AsHTTP(err))
			return
		}
		if principal.Role == auth.RoleAgent {
			if scope.AgentID == nil {
				httpx.Error(w, http.StatusForbidden, httpx.CodeForbidden, "acente profili gerekli")
				return
			}
			filter.AgentID = scope.AgentID
		}
		out, err := ReadAgentPerformance(r.Context(), pool, filter)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, out)
	}
}
