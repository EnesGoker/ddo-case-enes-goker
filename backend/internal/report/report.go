// Package report dönem raporlarının DB ve HTTP uygulama katmanıdır.
package report

import (
	"context"
	"net/http"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/lossratio"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Filter struct {
	From    *time.Time
	To      *time.Time
	GroupBy string
}

type Row struct {
	Key            string `json:"key"`
	Label          string `json:"label"`
	WrittenPremium int64  `json:"written_premium_kurus"`
	PolicyCount    int64  `json:"policy_count"`
	IncurredClaims int64  `json:"incurred_claims_kurus"`
	LossRatioBP    *int64 `json:"loss_ratio_bp"`
}

type Production struct {
	GroupBy string `json:"group_by"`
	Items   []Row  `json:"items"`
	Total   Row    `json:"total"`
}

// ReadProduction tüm boyutları aynı defter hareketlerinden ve tek sorgu snapshot'ından üretir.
func ReadProduction(ctx context.Context, q db.DBTX, filter Filter) (Production, error) {
	out := Production{GroupBy: filter.GroupBy, Items: []Row{}, Total: Row{Key: "total", Label: "Toplam"}}
	rows, err := q.Query(ctx, `
  WITH movements AS (
   SELECT e.entry_date,p.product,p.agent_id,
          l.credit_kurus-l.debit_kurus AS premium,
          CASE WHEN e.ref_type='POLICY_ISSUE' THEN 1 ELSE 0 END AS policies,
          0::bigint AS claims
     FROM journal_entries e JOIN journal_lines l ON l.entry_id=e.id
     JOIN accounts a ON a.id=l.account_id AND a.code='4000'
     LEFT JOIN endorsements z ON e.ref_type='ENDORSEMENT' AND z.id=e.ref_id
     JOIN policies p ON p.id=CASE WHEN e.ref_type='POLICY_ISSUE' THEN e.ref_id ELSE z.policy_id END
    WHERE e.ref_type IN ('POLICY_ISSUE','ENDORSEMENT')
   UNION ALL
   SELECT e.entry_date,p.product,p.agent_id,0::bigint,0,l.debit_kurus-l.credit_kurus
     FROM journal_entries e JOIN journal_lines l ON l.entry_id=e.id
     JOIN accounts a ON a.id=l.account_id AND a.code='5000'
     JOIN claims c ON c.id=e.ref_id JOIN policies p ON p.id=c.policy_id
    WHERE e.ref_type='CLAIM_APPROVAL'
  ), grouped AS (
   SELECT CASE $3 WHEN 'month' THEN to_char(m.entry_date,'YYYY-MM')
                  WHEN 'product' THEN m.product::text
                  ELSE coalesce(m.agent_id::text,'unassigned') END AS key,
          CASE $3 WHEN 'month' THEN to_char(m.entry_date,'YYYY-MM')
                  WHEN 'product' THEN m.product::text
                  ELSE coalesce(a.agency_name,'Acentesiz') END AS label,
          m.premium,m.policies,m.claims
     FROM movements m LEFT JOIN agents a ON a.id=m.agent_id
    WHERE ($1::date IS NULL OR m.entry_date >= $1)
      AND ($2::date IS NULL OR m.entry_date < $2)
  )
  SELECT key,label,sum(premium),sum(policies),sum(claims)
    FROM grouped GROUP BY key,label ORDER BY key`, filter.From, filter.To, filter.GroupBy)
	if err != nil {
		return out, err
	}
	out.Items, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Row, error) {
		var row Row
		err := r.Scan(&row.Key, &row.Label, &row.WrittenPremium, &row.PolicyCount, &row.IncurredClaims)
		row.LossRatioBP = lossratio.LossRatioBP(row.IncurredClaims, row.WrittenPremium)
		return row, err
	})
	if err != nil {
		return out, err
	}
	for _, row := range out.Items {
		out.Total.WrittenPremium += row.WrittenPremium
		out.Total.PolicyCount += row.PolicyCount
		out.Total.IncurredClaims += row.IncurredClaims
	}
	out.Total.LossRatioBP = lossratio.LossRatioBP(out.Total.IncurredClaims, out.Total.WrittenPremium)
	return out, nil
}

func ProductionHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := Filter{GroupBy: r.URL.Query().Get("group_by")}
		if filter.GroupBy == "" {
			filter.GroupBy = "month"
		}
		if filter.GroupBy != "month" && filter.GroupBy != "product" && filter.GroupBy != "agent" {
			httpx.ValidationError(w, map[string]string{"group_by": "month, product veya agent olmalı"})
			return
		}
		for _, bound := range []struct {
			key    string
			target **time.Time
		}{{"from", &filter.From}, {"to", &filter.To}} {
			if raw := r.URL.Query().Get(bound.key); raw != "" {
				date, err := caldate.Parse(raw)
				if err != nil {
					httpx.Error(w, http.StatusUnprocessableEntity, "INVALID_PERIOD", "tarihler YYYY-MM-DD biçiminde olmalı")
					return
				}
				*bound.target = &date
			}
		}
		if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
			httpx.Error(w, http.StatusUnprocessableEntity, "INVALID_PERIOD", "from, to tarihinden önce olmalı")
			return
		}
		out, err := ReadProduction(r.Context(), pool, filter)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, out)
	}
}
