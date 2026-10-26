SELECT claim_no FROM claims WHERE workshop_id=4 AND id<>7783 AND loss_date BETWEEN '2026-09-25'::date AND '2026-10-25'::date ORDER BY loss_date,id;
