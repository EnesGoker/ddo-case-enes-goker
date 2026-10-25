package claim

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/jackc/pgx/v5"
)

func (s *Service) evaluate(ctx context.Context, tx pgx.Tx, c Claim) (fraud.Result, error) {
	var startDate string
	if err := tx.QueryRow(ctx, `SELECT to_char(start_date,'YYYY-MM-DD') FROM policies WHERE id=$1`, c.PolicyID).Scan(&startDate); err != nil {
		return fraud.Result{}, err
	}
	start, _ := caldate.Parse(startDate)
	loss, _ := caldate.Parse(c.LossDate)
	in := fraud.Input{PolicyStart: start, Loss: loss, HasPlate: c.Plate != nil, HasWorkshop: c.WorkshopID != nil}
	if c.Plate != nil {
		plate, _ := policy.NormalizePlate(*c.Plate)
		from, to := fraud.Window(loss, s.fraud.RepeatDays)
		rows, err := tx.Query(ctx, `
   SELECT claim_no FROM claims WHERE plate=$1 AND id<>$2 AND loss_date BETWEEN $3 AND $4
   ORDER BY loss_date,id`, plate, c.ID, from, to)
		if err != nil {
			return fraud.Result{}, err
		}
		in.PlateClaims, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fraud.Result{}, err
		}
	}
	if c.WorkshopID != nil {
		from, to := fraud.Window(loss, s.fraud.WorkshopDays)
		rows, err := tx.Query(ctx, `
   SELECT claim_no FROM claims WHERE workshop_id=$1 AND id<>$2 AND loss_date BETWEEN $3 AND $4
   ORDER BY loss_date,id`, c.WorkshopID, c.ID, from, to)
		if err != nil {
			return fraud.Result{}, err
		}
		in.WorkshopClaims, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fraud.Result{}, err
		}
	}
	result, err := fraud.Evaluate(in, s.fraud)
	if err != nil {
		return result, err
	}
	for _, signal := range result.Signals {
		detail, err := json.Marshal(signal.Detail)
		if err != nil {
			return result, err
		}
		// Açıklama da kayıtta kalır; sonradan kural değişse bile eski neden korunur.
		payload, err := json.Marshal(map[string]any{"data": json.RawMessage(detail), "explanation": signal.Explanation, "score_threshold": result.Threshold})
		if err != nil {
			return result, err
		}
		if _, err := tx.Exec(ctx, `
   INSERT INTO fraud_signals(claim_id,rule_code,points,detail,created_at) VALUES($1,$2,$3,$4,$5)`, c.ID, signal.RuleCode, signal.Points, payload, s.now()); err != nil {
			return result, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE claims SET fraud_score=$2,fraud_flagged=$3 WHERE id=$1`, c.ID, result.Score, result.Flagged)
	return result, err
}

func (s *Service) fraudDetail(ctx context.Context, q db.DBTX, id int64) (*fraud.Result, error) {
	result := &fraud.Result{Threshold: s.fraud.Threshold, Signals: []fraud.Signal{}}
	if err := q.QueryRow(ctx, `SELECT fraud_score,fraud_flagged FROM claims WHERE id=$1`, id).Scan(&result.Score, &result.Flagged); err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT rule_code,points,detail FROM fraud_signals WHERE claim_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	result.Signals, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (fraud.Signal, error) {
		var signal fraud.Signal
		var raw []byte
		if err := r.Scan(&signal.RuleCode, &signal.Points, &raw); err != nil {
			return signal, err
		}
		var saved struct {
			Data        fraud.Detail `json:"data"`
			Explanation string       `json:"explanation"`
			Threshold   int          `json:"score_threshold"`
		}
		if err := json.Unmarshal(raw, &saved); err != nil {
			return signal, err
		}
		signal.Detail = saved.Data
		signal.Explanation = strings.TrimSpace(saved.Explanation)
		if saved.Threshold > 0 {
			result.Threshold = saved.Threshold
		}
		return signal, nil
	})
	return result, err
}
