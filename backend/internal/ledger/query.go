package ledger

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/page"
)

type AccountBalance struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	DebitKurus   int64  `json:"debit_kurus"`
	CreditKurus  int64  `json:"credit_kurus"`
	BalanceKurus int64  `json:"balance_kurus"`
}

type Reconciliation struct {
	Ledger1200       int64 `json:"ledger_1200"`
	OpenInstallments int64 `json:"open_installments"`
	OpenInterest     int64 `json:"open_interest"`
	Matches          bool  `json:"matches"`
}

type TrialBalance struct {
	Accounts                 []AccountBalance `json:"accounts"`
	TotalDebitKurus          int64            `json:"total_debit_kurus"`
	TotalCreditKurus         int64            `json:"total_credit_kurus"`
	Balanced                 bool             `json:"balanced"`
	EmptyEntries             int64            `json:"empty_entries"`
	UnbalancedEntries        int64            `json:"unbalanced_entries"`
	ReceivableReconciliation Reconciliation   `json:"receivable_reconciliation"`
}

func ReadTrialBalance(ctx context.Context, q db.DBTX) (TrialBalance, error) {
	out := TrialBalance{Accounts: []AccountBalance{}}
	rows, err := q.Query(ctx, `
  SELECT a.code, a.name, coalesce(sum(l.debit_kurus),0), coalesce(sum(l.credit_kurus),0)
    FROM accounts a LEFT JOIN journal_lines l ON l.account_id = a.id
   GROUP BY a.id ORDER BY a.code`)
	if err != nil {
		return out, err
	}
	out.Accounts, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AccountBalance, error) {
		var a AccountBalance
		err := r.Scan(&a.Code, &a.Name, &a.DebitKurus, &a.CreditKurus)
		a.BalanceKurus = a.DebitKurus - a.CreditKurus
		return a, err
	})
	if err != nil {
		return out, err
	}
	for _, a := range out.Accounts {
		out.TotalDebitKurus += a.DebitKurus
		out.TotalCreditKurus += a.CreditKurus
		if a.Code == "1200" {
			out.ReceivableReconciliation.Ledger1200 = a.BalanceKurus
		}
	}
	err = q.QueryRow(ctx, `
  SELECT count(*) FILTER (WHERE n = 0), count(*) FILTER (WHERE debit <> credit)
    FROM (
     SELECT e.id, count(l.id) AS n, coalesce(sum(l.debit_kurus),0) AS debit,
            coalesce(sum(l.credit_kurus),0) AS credit
       FROM journal_entries e LEFT JOIN journal_lines l ON l.entry_id = e.id
      GROUP BY e.id
    ) entries`).Scan(&out.EmptyEntries, &out.UnbalancedEntries)
	if err != nil {
		return out, err
	}
	err = q.QueryRow(ctx, `
  SELECT
   (SELECT coalesce(sum(amount_kurus - paid_kurus),0)
      FROM installments WHERE status IN ('PENDING','PARTIAL')),
   (SELECT coalesce(sum(amount_kurus - paid_kurus),0) FROM late_interest_charges)
 `).Scan(&out.ReceivableReconciliation.OpenInstallments, &out.ReceivableReconciliation.OpenInterest)
	out.Balanced = out.TotalDebitKurus == out.TotalCreditKurus && out.EmptyEntries == 0 && out.UnbalancedEntries == 0
	r := &out.ReceivableReconciliation
	r.Matches = r.Ledger1200 == r.OpenInstallments+r.OpenInterest
	return out, err
}

type JournalLineView struct {
	ID          int64  `json:"id"`
	AccountCode string `json:"account_code"`
	DebitKurus  int64  `json:"debit_kurus"`
	CreditKurus int64  `json:"credit_kurus"`
}

type JournalView struct {
	ID          int64             `json:"id"`
	EntryDate   string            `json:"entry_date"`
	Description string            `json:"description"`
	RefType     string            `json:"ref_type"`
	RefID       int64             `json:"ref_id"`
	CreatedAt   time.Time         `json:"created_at"`
	Lines       []JournalLineView `json:"lines"`
}

func ListEntries(ctx context.Context, q db.DBTX, params page.Params) ([]JournalView, error) {
	rows, err := q.Query(ctx, `
  SELECT id, to_char(entry_date,'YYYY-MM-DD'), description, ref_type, ref_id, created_at
    FROM journal_entries WHERE ($1::bigint = 0 OR id < $1)
   ORDER BY id DESC LIMIT $2`, params.AfterID, params.Limit)
	if err != nil {
		return nil, err
	}
	entries, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (JournalView, error) {
		e := JournalView{Lines: []JournalLineView{}}
		err := r.Scan(&e.ID, &e.EntryDate, &e.Description, &e.RefType, &e.RefID, &e.CreatedAt)
		return e, err
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return entries, nil
	}
	ids := make([]int64, len(entries))
	indices := map[int64]int{}
	for i, e := range entries {
		ids[i] = e.ID
		indices[e.ID] = i
	}
	rows, err = q.Query(ctx, `
  SELECT l.entry_id, l.id, a.code, l.debit_kurus, l.credit_kurus
    FROM journal_lines l JOIN accounts a ON a.id = l.account_id
   WHERE l.entry_id = ANY($1) ORDER BY l.id`, ids)
	if err != nil {
		return nil, err
	}
	var entryID int64
	var line JournalLineView
	_, err = pgx.ForEachRow(rows, []any{&entryID, &line.ID, &line.AccountCode, &line.DebitKurus, &line.CreditKurus}, func() error {
		i := indices[entryID]
		entries[i].Lines = append(entries[i].Lines, line)
		return nil
	})
	return entries, err
}
