-- +goose Up
ALTER TABLE appraisals ADD COLUMN request_key TEXT;
CREATE UNIQUE INDEX appraisals_request_key_uq
    ON appraisals (appraiser_id, request_key) WHERE request_key IS NOT NULL;

-- +goose Down
DROP INDEX appraisals_request_key_uq;
ALTER TABLE appraisals DROP COLUMN request_key;
