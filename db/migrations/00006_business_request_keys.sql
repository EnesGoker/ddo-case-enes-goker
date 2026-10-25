-- +goose Up
ALTER TABLE payments ADD COLUMN request_key TEXT;
ALTER TABLE endorsements ADD COLUMN request_key TEXT;
ALTER TABLE claims ADD COLUMN request_key TEXT;
CREATE UNIQUE INDEX payments_request_key_uq ON payments (created_by, request_key) WHERE request_key IS NOT NULL;
CREATE UNIQUE INDEX endorsements_request_key_uq ON endorsements (created_by, request_key) WHERE request_key IS NOT NULL;
CREATE UNIQUE INDEX claims_request_key_uq ON claims (created_by, request_key) WHERE request_key IS NOT NULL;
ALTER TABLE idempotency_keys ALTER COLUMN response_body TYPE bytea USING convert_to(response_body::text, 'UTF8');

-- +goose Down
-- JSONB'ye dönüş bayt sırasını korumaz; iş kayıtları silinmez.
ALTER TABLE idempotency_keys ALTER COLUMN response_body TYPE jsonb USING convert_from(response_body, 'UTF8')::jsonb;
DROP INDEX claims_request_key_uq;
DROP INDEX endorsements_request_key_uq;
DROP INDEX payments_request_key_uq;
ALTER TABLE claims DROP COLUMN request_key;
ALTER TABLE endorsements DROP COLUMN request_key;
ALTER TABLE payments DROP COLUMN request_key;
