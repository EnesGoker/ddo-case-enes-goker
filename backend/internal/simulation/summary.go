package simulation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/ledger"
)

func (r *runner) summarize(ctx context.Context) (json.RawMessage, string, error) {
	if err := r.progress(ctx, "SUMMARY", 0, 1); err != nil {
		return nil, "", err
	}
	trial, err := ledger.ReadTrialBalance(ctx, r.pool)
	if err != nil {
		return nil, "", err
	}
	if !trial.Balanced || !trial.ReceivableReconciliation.Matches || trial.EmptyEntries != 0 || trial.UnbalancedEntries != 0 {
		return nil, "", fmt.Errorf("simülasyon mali mutabakatı başarısız")
	}
	values := map[string]json.RawMessage{}
	queries := []struct{ key, sql string }{
		{"policies", `
          SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM (
            SELECT product,status,count(*) AS count FROM policies WHERE policy_no LIKE 'SIM-%'
             GROUP BY product,status ORDER BY product,status) x`},
		{"installments", `
          SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM (
            SELECT i.status,count(*) AS count,sum(i.amount_kurus) AS amount_kurus,sum(i.paid_kurus) AS paid_kurus
              FROM installments i JOIN installment_plans pl ON pl.id=i.plan_id
              JOIN policies p ON p.id=pl.policy_id WHERE p.policy_no LIKE 'SIM-%'
             GROUP BY i.status ORDER BY i.status) x`},
		{"payments", `
          SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM (
            SELECT pay.kind,count(*) AS count,sum(pay.amount_kurus) AS amount_kurus
              FROM payments pay JOIN policies p ON p.id=pay.policy_id WHERE p.policy_no LIKE 'SIM-%'
             GROUP BY pay.kind ORDER BY pay.kind) x`},
		{"claims", `
          SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM (
            SELECT c.status,count(*) AS count,count(*) FILTER(WHERE fraud_flagged) AS flagged
              FROM claims c JOIN policies p ON p.id=c.policy_id WHERE p.policy_no LIKE 'SIM-%'
             GROUP BY c.status ORDER BY c.status) x`},
		{"tasks", `
          SELECT jsonb_build_object('open',count(*) FILTER(WHERE t.status IN ('OPEN','IN_PROGRESS')),
                 'breached',count(*) FILTER(WHERE t.status IN ('OPEN','IN_PROGRESS') AND t.sla_due_at < $1))
            FROM claim_tasks t JOIN claims c ON c.id=t.claim_id JOIN policies p ON p.id=c.policy_id
           WHERE p.policy_no LIKE 'SIM-%'`},
		{"ledger", `
          SELECT jsonb_build_object('written_premium_kurus',coalesce(sum(l.credit_kurus-l.debit_kurus) FILTER(WHERE a.code='4000'),0),
                 'cash_kurus',coalesce(sum(l.debit_kurus-l.credit_kurus) FILTER(WHERE a.code='1000'),0),
                 'claim_expense_kurus',coalesce(sum(l.debit_kurus-l.credit_kurus) FILTER(WHERE a.code='5000'),0))
            FROM journal_lines l JOIN accounts a ON a.id=l.account_id
            JOIN journal_entries e ON e.id=l.entry_id JOIN users u ON u.id=e.created_by
           WHERE u.email LIKE 'sim-%@guvenpolice.test'`},
	}
	for _, q := range queries {
		var raw []byte
		var err error
		if q.key == "tasks" {
			err = r.pool.QueryRow(ctx, q.sql, r.cfg.Anchor.Add(24*time.Hour)).Scan(&raw)
		} else {
			err = r.pool.QueryRow(ctx, q.sql).Scan(&raw)
		}
		if err != nil {
			return nil, "", err
		}
		values[q.key] = raw
	}
	values["balanced"] = json.RawMessage("true")
	values["receivable_matches"] = json.RawMessage("true")
	summary, err := json.Marshal(values)
	if err != nil {
		return nil, "", err
	}
	fingerprint, err := r.fingerprint(ctx)
	return summary, fingerprint, err
}

func (r *runner) fingerprint(ctx context.Context) (string, error) {
	sum := sha256.New()
	// Her bölüm canonical JSONB satırı olarak sıralanır; kimlikler doğal iş anahtarlarıyla çözülür.
	sections := []struct{ name, sql string }{
		{"policies", `
          SELECT jsonb_build_array(p.policy_no,p.product,c.national_id,u.email,p.start_date,p.end_date,p.status,p.cancel_effective_date,p.cancel_reason,
                 v.version_no,v.effective_from,v.effective_to,t.version_no,v.vehicle_plate,v.risk_data,v.annual_premium_kurus,v.annual_limit_kurus)::text
            FROM policies p JOIN customers c ON c.id=p.customer_id JOIN agents a ON a.id=p.agent_id
            JOIN users u ON u.id=a.user_id JOIN policy_versions v ON v.policy_id=p.id
            JOIN tariff_versions t ON t.id=v.tariff_version_id WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"coverages", `
          SELECT jsonb_build_array(p.policy_no,v.version_no,c.coverage_code,c.limit_kurus,c.deductible_type,c.deductible_value,c.annual_premium_kurus,c.event_limit_kurus)::text
            FROM policy_version_coverages c JOIN policy_versions v ON v.id=c.policy_version_id
            JOIN policies p ON p.id=v.policy_id WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"limits", `
          SELECT jsonb_build_array(p.policy_no,b.coverage_code,b.period_start,b.period_end,b.limit_kurus,b.used_kurus)::text
            FROM policy_limit_balances b JOIN policies p ON p.id=b.policy_id WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"installments", `
          SELECT jsonb_build_array(p.policy_no,pl.total_kurus,pl.installment_count,pl.status,i.seq,i.due_date,i.amount_kurus,i.paid_kurus,i.status)::text
            FROM installments i JOIN installment_plans pl ON pl.id=i.plan_id JOIN policies p ON p.id=pl.policy_id
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"interest", `
          SELECT jsonb_build_array(p.policy_no,i.seq,c.accrued_from,c.accrued_to,c.overdue_principal_kurus,c.daily_rate_ppm,c.amount_kurus,c.paid_kurus)::text
            FROM late_interest_charges c JOIN installments i ON i.id=c.installment_id JOIN installment_plans pl ON pl.id=i.plan_id
            JOIN policies p ON p.id=pl.policy_id WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"payments", `
          SELECT jsonb_build_array(p.policy_no,pay.kind,pay.amount_kurus,pay.method,pay.paid_at,pay.reference,u.email,
                 original.paid_at,original.amount_kurus)::text FROM payments pay JOIN policies p ON p.id=pay.policy_id
            JOIN users u ON u.id=pay.created_by LEFT JOIN payments original ON original.id=pay.refund_of_payment_id
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"allocations", `
          SELECT jsonb_build_array(p.policy_no,pay.kind,pay.paid_at,pay.amount_kurus,a.amount_kurus,i.seq,ci.seq,ic.accrued_from,ic.accrued_to)::text
            FROM payment_allocations a JOIN payments pay ON pay.id=a.payment_id JOIN policies p ON p.id=pay.policy_id
            LEFT JOIN installments i ON i.id=a.installment_id LEFT JOIN late_interest_charges ic ON ic.id=a.late_interest_charge_id
            LEFT JOIN installments ci ON ci.id=ic.installment_id WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"endorsements", `
          SELECT jsonb_build_array(p.policy_no,e.endorsement_no,e.type,e.effective_date,e.premium_delta_kurus,e.calc_detail-'tariff_version_id',u.email)::text
            FROM endorsements e JOIN policies p ON p.id=e.policy_id JOIN users u ON u.id=e.created_by
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"claims", `
          SELECT jsonb_build_array(p.policy_no,c.description,c.coverage_code,c.loss_date,c.plate,w.name,c.claimed_amount_kurus,c.status,c.fraud_score,c.fraud_flagged,u.email)::text
            FROM claims c JOIN policies p ON p.id=c.policy_id JOIN users u ON u.id=c.created_by
            LEFT JOIN workshops w ON w.id=c.workshop_id WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"history", `
          SELECT jsonb_build_array(c.description,h.from_status,h.to_status,u.email,h.note)::text FROM claim_status_history h
            JOIN claims c ON c.id=h.claim_id JOIN policies p ON p.id=c.policy_id LEFT JOIN users u ON u.id=h.actor_user_id
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"fraud", `
          SELECT jsonb_build_array(c.description,f.rule_code,f.points,(f.detail->'data')-'previous_claim_nos',f.detail->'score_threshold')::text
            FROM fraud_signals f JOIN claims c ON c.id=f.claim_id JOIN policies p ON p.id=c.policy_id
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"tasks", `
          SELECT jsonb_build_array(c.description,t.task_type,t.status,u.email,t.sla_due_at)::text FROM claim_tasks t
            JOIN claims c ON c.id=t.claim_id JOIN policies p ON p.id=c.policy_id LEFT JOIN users u ON u.id=t.assigned_to
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"appraisals", `
          SELECT jsonb_build_array(c.description,a.damage_amount_kurus,a.is_total_loss,u.email,a.notes)::text FROM appraisals a
            JOIN claims c ON c.id=a.claim_id JOIN policies p ON p.id=c.policy_id JOIN users u ON u.id=a.appraiser_id
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"settlements", `
          SELECT jsonb_build_array(c.description,s.gross_damage_kurus,s.deductible_kurus,s.payable_kurus,s.status,s.calc_steps,u.email)::text
            FROM claim_settlements s JOIN claims c ON c.id=s.claim_id JOIN policies p ON p.id=c.policy_id
            JOIN users u ON u.id=s.approved_by WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"consumptions", `
          SELECT jsonb_build_array(c.description,b.coverage_code,b.period_start,l.amount_kurus)::text FROM limit_consumptions l
            JOIN claim_settlements s ON s.id=l.settlement_id JOIN claims c ON c.id=s.claim_id
            JOIN policy_limit_balances b ON b.id=l.balance_id JOIN policies p ON p.id=c.policy_id
           WHERE p.policy_no LIKE 'SIM-%' ORDER BY 1`},
		{"ledger", `
          SELECT jsonb_build_array(e.ref_type,e.entry_date,u.email,a.code,l.debit_kurus,l.credit_kurus,
            CASE
              WHEN e.ref_type='POLICY_ISSUE' THEN (SELECT jsonb_build_array(p.policy_no) FROM policies p WHERE p.id=e.ref_id)
              WHEN e.ref_type IN ('PAYMENT','REFUND') THEN (SELECT jsonb_build_array(p.policy_no,pay.kind,pay.paid_at,pay.amount_kurus) FROM payments pay JOIN policies p ON p.id=pay.policy_id WHERE pay.id=e.ref_id)
              WHEN e.ref_type='ENDORSEMENT' THEN (SELECT jsonb_build_array(p.policy_no,en.endorsement_no) FROM endorsements en JOIN policies p ON p.id=en.policy_id WHERE en.id=e.ref_id)
              WHEN e.ref_type='LATE_INTEREST' THEN (SELECT jsonb_build_array(p.policy_no,i.seq,ic.accrued_from,ic.accrued_to) FROM late_interest_charges ic JOIN installments i ON i.id=ic.installment_id JOIN installment_plans pl ON pl.id=i.plan_id JOIN policies p ON p.id=pl.policy_id WHERE ic.id=e.ref_id)
              WHEN e.ref_type IN ('CLAIM_APPROVAL','CLAIM_PAYMENT') THEN (SELECT jsonb_build_array(c.description) FROM claims c WHERE c.id=e.ref_id)
            END)::text
            FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id JOIN accounts a ON a.id=l.account_id
            JOIN users u ON u.id=e.created_by WHERE u.email LIKE 'sim-%@guvenpolice.test' ORDER BY 1`},
	}
	for _, section := range sections {
		_, _ = fmt.Fprintln(sum, section.name)
		rows, err := r.pool.Query(ctx, section.sql)
		if err != nil {
			return "", fmt.Errorf("fingerprint %s: %w", section.name, err)
		}
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				rows.Close()
				return "", err
			}
			_, _ = fmt.Fprintln(sum, row)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
