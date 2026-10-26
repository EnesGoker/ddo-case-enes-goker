-- +goose Up
-- Onaylı tarih düzeltmesi: v0 kimlikleri, fiyatlar ve bağlı poliçeler korunur.
-- Tarife üzerinde değişmezlik tetikleyicisi yoktur; poliçe korumaları kapatılmaz.
-- +goose StatementBegin
DO $$
BEGIN
    IF (SELECT count(*) FROM tariff_versions
         WHERE version_no=0 AND product IN ('KASKO','TRAFIK','KONUT')) <> 3
       OR EXISTS (SELECT 1 FROM tariff_versions
                   WHERE version_no=0 AND product IN ('KASKO','TRAFIK','KONUT')
                     AND (valid_from NOT IN ('0001-01-01','2020-01-01')
                          OR valid_to IS DISTINCT FROM DATE '2025-01-01')) THEN
        RAISE EXCEPTION 'v0 tarih düzeltmesi: beklenmeyen tarife';
    END IF;
    IF EXISTS (SELECT 1 FROM policy_versions v JOIN tariff_versions t ON t.id=v.tariff_version_id
                WHERE t.version_no=0 AND v.effective_from < DATE '2020-01-01')
       OR EXISTS (SELECT 1 FROM quotes q JOIN tariff_versions t ON t.id=q.tariff_version_id
                   WHERE t.version_no=0 AND q.term_start < DATE '2020-01-01') THEN
        RAISE EXCEPTION 'v0 tarih düzeltmesi: 2020 öncesi bağlı kayıt var';
    END IF;
    UPDATE tariff_versions SET valid_from=DATE '2020-01-01'
     WHERE version_no=0 AND product IN ('KASKO','TRAFIK','KONUT');
END;
$$;
-- +goose StatementEnd

-- +goose Down
UPDATE tariff_versions SET valid_from=DATE '0001-01-01'
 WHERE version_no=0 AND product IN ('KASKO','TRAFIK','KONUT')
   AND valid_from=DATE '2020-01-01';
