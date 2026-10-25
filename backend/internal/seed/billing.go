package seed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
)

// Tarihler yalnızca ilk oluşturulurken bugüne göre seçilir; tekrar seed geçmişi değiştirmez.
func billingPolicies(ctx context.Context, tx pgx.Tx, agentID, actorID int64) error {
	var customerID int64
	if err := tx.QueryRow(ctx, `
  SELECT id FROM customers WHERE national_id = '10000000001'`).Scan(&customerID); err != nil {
		return err
	}
	for _, sample := range []struct {
		no   string
		days int
	}{{"GP-DEMO-OVERDUE-A", 75}, {"GP-DEMO-OVERDUE-B", 20}} {
		var exists bool
		if err := tx.QueryRow(ctx, `
   SELECT EXISTS (SELECT 1 FROM policies WHERE policy_no = $1)`, sample.no).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		start := caldate.Today(time.Now()).AddDate(0, 0, -sample.days)
		plate := "34DMA055"
		if sample.days == 20 {
			plate = "34DMB055"
		}
		_, err := issuePriced(ctx, tx, customerID, agentID, actorID, sample.no, plate, start, 3)
		if err != nil {
			return err
		}
	}
	return nil
}
