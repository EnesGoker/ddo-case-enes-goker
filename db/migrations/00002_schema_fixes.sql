-- +goose Up
-- A0 şema değerlendirmesinden onaylanan düzeltmeler (00001 değiştirilmez)

-- 1. Bir yevmiye kaydı en fazla bir kez ters çevrilebilir
CREATE UNIQUE INDEX journal_entries_reverses_uq
    ON journal_entries (reverses_entry_id) WHERE reverses_entry_id IS NOT NULL;

-- 2. Hasarın bağlandığı poliçe sürümü, hasarın poliçesine ait olmalı
ALTER TABLE policy_versions
    ADD CONSTRAINT policy_versions_policy_id_id_uq UNIQUE (policy_id, id);
ALTER TABLE claims
    ADD CONSTRAINT claims_policy_version_match_fk
    FOREIGN KEY (policy_id, policy_version_id) REFERENCES policy_versions (policy_id, id);

-- 3. Acente teklif listesi için keyset indeksi
CREATE INDEX quotes_agent_idx ON quotes (agent_id, id);

-- +goose Down
DROP INDEX IF EXISTS quotes_agent_idx;
ALTER TABLE claims DROP CONSTRAINT IF EXISTS claims_policy_version_match_fk;
ALTER TABLE policy_versions DROP CONSTRAINT IF EXISTS policy_versions_policy_id_id_uq;
DROP INDEX IF EXISTS journal_entries_reverses_uq;
