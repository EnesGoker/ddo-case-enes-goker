-- tested_commit=c374d9b6f96dabd180a0d655f85dfb7452064e87
-- Parameters: $1 from inclusive, $2 to exclusive, $3 authenticated agent_id.
WITH issued AS (
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
 ORDER BY a.id NULLS LAST;
