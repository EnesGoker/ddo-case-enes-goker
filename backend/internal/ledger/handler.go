package ledger

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
)

func TrialBalanceHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var out TrialBalance
		err := pgx.BeginTxFunc(r.Context(), pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			var err error
			out, err = ReadTrialBalance(r.Context(), tx)
			return err
		})
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, out)
	}
}

func EntriesHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, bad := page.Parse(r)
		if bad != nil {
			httpx.ValidationError(w, bad)
			return
		}
		entries, err := ListEntries(r.Context(), pool, params)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, page.Build(entries, params.Limit, func(e JournalView) int64 { return e.ID }))
	}
}
