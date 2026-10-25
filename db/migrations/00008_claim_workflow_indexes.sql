-- +goose Up
INSERT INTO claim_transitions (from_status,to_status) VALUES ('FRAUD_REVIEW','APPRAISAL');
-- claims_plate_loss_idx ve claim_tasks_load_idx başlangıç şemasında zaten var.
CREATE INDEX claims_workshop_loss_idx ON claims (workshop_id,loss_date) WHERE workshop_id IS NOT NULL;
CREATE INDEX journal_entries_date_idx ON journal_entries (entry_date);

-- +goose Down
DROP INDEX journal_entries_date_idx;
DROP INDEX claims_workshop_loss_idx;
-- Bu geçişi kullanan geçmiş varsa FK nedeniyle Down başarısız olabilir; geçmiş silinmez.
DELETE FROM claim_transitions WHERE from_status='FRAUD_REVIEW' AND to_status='APPRAISAL';
