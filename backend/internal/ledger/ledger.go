// Package ledger çağıranın transaction'ında dengeli, satırlı yevmiye kaydı yazar.
package ledger

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

type Entry struct {
	Date        time.Time
	Description string
	RefType     string
	RefID       int64
	CreatedBy   int64
}

type Line struct {
	AccountCode string
	DebitKurus  int64
	CreditKurus int64
}

func Post(ctx context.Context, tx pgx.Tx, entry Entry, lines []Line) (int64, error) {
	if len(lines) == 0 {
		return 0, fmt.Errorf("yevmiye satırsız olamaz")
	}
	var debit, credit int64
	for _, line := range lines {
		if line.AccountCode == "" || line.DebitKurus < 0 || line.CreditKurus < 0 || (line.DebitKurus > 0) == (line.CreditKurus > 0) {
			return 0, fmt.Errorf("geçersiz yevmiye satırı")
		}
		if line.DebitKurus > math.MaxInt64-debit || line.CreditKurus > math.MaxInt64-credit {
			return 0, fmt.Errorf("yevmiye toplamı taşması")
		}
		debit += line.DebitKurus
		credit += line.CreditKurus
	}
	if debit != credit {
		return 0, fmt.Errorf("yevmiye dengede değil: %d != %d", debit, credit)
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO journal_entries (entry_date, description, ref_type, ref_id, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`, entry.Date, entry.Description, entry.RefType, entry.RefID, entry.CreatedBy).Scan(&id); err != nil {
		return 0, err
	}
	for _, line := range lines {
		tag, err := tx.Exec(ctx, `INSERT INTO journal_lines (entry_id, account_id, debit_kurus, credit_kurus) SELECT $1,id,$3,$4 FROM accounts WHERE code=$2`, id, line.AccountCode, line.DebitKurus, line.CreditKurus)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() != 1 {
			return 0, fmt.Errorf("muhasebe hesabı bulunamadı: %s", line.AccountCode)
		}
	}
	return id, nil
}
