-- +goose Up
-- Eski sürümler NULL kalır; geçmiş poliçe/tazminat/defter yeniden yazılmaz.
ALTER TABLE policy_version_coverages ADD COLUMN event_limit_kurus BIGINT
    CONSTRAINT policy_version_coverages_event_limit_positive
    CHECK (event_limit_kurus IS NULL OR event_limit_kurus > 0);

-- +goose Down
-- Olay limitiyle düzenlenmiş sürüm varsa sözleşme bilgisini silmeden reddet.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM policy_version_coverages WHERE event_limit_kurus IS NOT NULL) THEN
        RAISE EXCEPTION 'olay limiti bulunan poliçe sürümleri var; Down veri kaybı yaratır';
    END IF;
END;
$$;
-- +goose StatementEnd
ALTER TABLE policy_version_coverages DROP COLUMN event_limit_kurus;
