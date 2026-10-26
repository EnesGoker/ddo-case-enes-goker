SELECT p.id, p.policy_id, p.kind, p.amount_kurus, p.method, p.refund_of_payment_id,
          p.paid_at, p.journal_entry_id, coalesce(p.reference,''),
          CASE WHEN p.kind = 'PAYMENT' THEN p.amount_kurus - coalesce((
           SELECT sum(r.amount_kurus) FROM payments r WHERE r.refund_of_payment_id = p.id
          ),0) ELSE 0 END
     FROM payments p WHERE p.policy_id = 5 ORDER BY p.paid_at DESC, p.id DESC;
