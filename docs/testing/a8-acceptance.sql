-- Salt okunur; son100k batch bitince, verify/load kabul verisinden önce alınır.
WITH run AS (
    SELECT *, (parameters->>'anchor')::timestamptz + interval '24 hours' AS batch_now
      FROM simulation_runs ORDER BY id DESC LIMIT 1
), sim_claims AS (
    SELECT c.* FROM claims c JOIN policies p ON p.id=c.policy_id
     WHERE p.policy_no LIKE 'SIM-%'
), sim_tasks AS (
    SELECT t.* FROM claim_tasks t JOIN sim_claims c ON c.id=t.claim_id
), rules AS (
    SELECT f.rule_code, count(*) AS signals,
           count(DISTINCT f.claim_id) AS claims,
           count(*) FILTER (WHERE c.fraud_flagged) AS flagged_signals
      FROM fraud_signals f JOIN sim_claims c ON c.id=f.claim_id
     GROUP BY f.rule_code ORDER BY f.rule_code
)
SELECT json_build_object(
    'measured_at', now(),
    'status',r.status,'phase',r.phase,'processed',r.processed,'target',r.target,
    'elapsed_seconds',extract(epoch FROM r.finished_at-r.started_at),
    'parameters',r.parameters,'fingerprint',r.fingerprint,'summary',r.summary,
    'database_bytes',pg_database_size(current_database()),
    'sim_policies',(SELECT count(*) FROM policies WHERE policy_no LIKE 'SIM-%'),
    'claims',(SELECT count(*) FROM sim_claims),
    'flagged',(SELECT count(*) FROM sim_claims WHERE fraud_flagged),
    'open_fraud_review',(SELECT count(*) FROM sim_claims WHERE status='FRAUD_REVIEW'),
    'open_tasks',(SELECT count(*) FROM sim_tasks WHERE status IN ('OPEN','IN_PROGRESS')),
    'breached_at_batch_end',(SELECT count(*) FROM sim_tasks WHERE status IN ('OPEN','IN_PROGRESS') AND sla_due_at<r.batch_now),
    'breached_at_measurement',(SELECT count(*) FROM sim_tasks WHERE status IN ('OPEN','IN_PROGRESS') AND sla_due_at<now()),
    'open_before_window',(SELECT count(*) FROM sim_tasks WHERE status IN ('OPEN','IN_PROGRESS') AND created_at<r.batch_now-interval '48 hours'),
    'appraisal_owner_mismatch',(SELECT count(*) FROM sim_tasks t JOIN appraisals a ON a.claim_id=t.claim_id
        WHERE t.status='DONE' AND t.task_type='APPRAISAL' AND t.assigned_to IS DISTINCT FROM a.appraiser_id),
    'done_appraisal_without_record',(SELECT count(*) FROM sim_tasks t WHERE t.status='DONE' AND t.task_type='APPRAISAL'
        AND NOT EXISTS(SELECT 1 FROM appraisals a WHERE a.claim_id=t.claim_id)),
    'cancelled_decision_tasks',(SELECT count(*) FROM sim_tasks WHERE status='CANCELLED' AND task_type IN ('ADJUSTMENT','FRAUD_REVIEW')),
    'rule_breakdown',(SELECT json_agg(rules) FROM rules)
) FROM run r;
