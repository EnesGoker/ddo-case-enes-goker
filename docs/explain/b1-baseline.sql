-- B1 performance gate: existing ADR-18 production query grouped by agent.
-- from/to omitted: NULL means unbounded; default and widest period are identical.
BEGIN READ ONLY;
SET LOCAL statement_timeout = '10s';
EXPLAIN (ANALYZE, BUFFERS)
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
   SELECT CASE 'agent'::text WHEN 'month' THEN to_char(m.entry_date,'YYYY-MM')
                  WHEN 'product' THEN m.product::text
                  ELSE coalesce(m.agent_id::text,'unassigned') END AS key,
          CASE 'agent'::text WHEN 'month' THEN to_char(m.entry_date,'YYYY-MM')
                  WHEN 'product' THEN m.product::text
                  ELSE coalesce(a.agency_name,'Acentesiz') END AS label,
          m.premium,m.policies,m.claims
     FROM movements m LEFT JOIN agents a ON a.id=m.agent_id
    WHERE (NULL::date IS NULL OR m.entry_date >= NULL)
      AND (NULL::date IS NULL OR m.entry_date < NULL)
  )
  SELECT key,label,sum(premium),sum(policies),sum(claims)
    FROM grouped GROUP BY key,label ORDER BY key;
ROLLBACK;
