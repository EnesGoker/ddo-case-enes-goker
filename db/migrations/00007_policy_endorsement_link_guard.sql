-- +goose Up
-- Bağlantı yalnızca aynı poliçenin bu sürümü üreten zeyline yazılabilir.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION policy_versions_guard() RETURNS trigger AS $$
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
    IF OLD.endorsement_id IS NULL AND NEW.endorsement_id IS NOT NULL
       AND EXISTS (SELECT 1 FROM endorsements e WHERE e.id=NEW.endorsement_id
                   AND e.policy_id=NEW.policy_id AND e.to_version_id=NEW.id)
       AND (to_jsonb(NEW) - 'endorsement_id') = (to_jsonb(OLD) - 'endorsement_id') THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'policy_versions % değiştirilemez: yalnızca açık sürümün effective_to alanı bir kez kapatılabilir', OLD.id
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION policy_versions_guard() RETURNS trigger AS $$
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
    IF OLD.endorsement_id IS NULL AND NEW.endorsement_id IS NOT NULL
       AND (to_jsonb(NEW) - 'endorsement_id') = (to_jsonb(OLD) - 'endorsement_id') THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'policy_versions % değiştirilemez: yalnızca açık sürümün effective_to alanı bir kez kapatılabilir', OLD.id
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

