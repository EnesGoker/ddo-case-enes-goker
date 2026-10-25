-- +goose Up
-- Poliçe sürümleri değişmez (A3 kararı). Tek izin verilen değişiklik: açık sürümün
-- effective_to alanının bir kez NULL → tarih olarak kapatılması (zeyil/iptal). Diğer tüm
-- alanlar, kapanmış sürümler ve sürüm teminatları değiştirilemez/silinemez.
-- Not: policy_versions.endorsement_id bu kural nedeniyle doldurulmaz (sürüm eklendikten sonra
-- güncellenemez); sürüm ↔ zeyil bağı endorsements.from_version_id / to_version_id ile tutulur.

-- +goose StatementBegin
CREATE FUNCTION policy_versions_guard() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'policy_versions % silinemez (sürümler değişmez)', OLD.id
            USING ERRCODE = '55000';
    END IF;
    IF OLD.effective_to IS NULL
       AND NEW.effective_to IS NOT NULL
       AND (to_jsonb(NEW) - 'effective_to') = (to_jsonb(OLD) - 'effective_to') THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'policy_versions % değiştirilemez: yalnızca açık sürümün effective_to alanı bir kez kapatılabilir', OLD.id
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER policy_versions_immutable
    BEFORE UPDATE OR DELETE ON policy_versions
    FOR EACH ROW EXECUTE FUNCTION policy_versions_guard();

-- Sürüm teminatları tamamen değişmez (00001'deki forbid_modify yeniden kullanılır)
CREATE TRIGGER policy_version_coverages_immutable
    BEFORE UPDATE OR DELETE ON policy_version_coverages
    FOR EACH ROW EXECUTE FUNCTION forbid_modify();

-- +goose Down
DROP TRIGGER IF EXISTS policy_version_coverages_immutable ON policy_version_coverages;
DROP TRIGGER IF EXISTS policy_versions_immutable ON policy_versions;
DROP FUNCTION IF EXISTS policy_versions_guard();
