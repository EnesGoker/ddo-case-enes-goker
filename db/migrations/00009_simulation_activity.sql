-- +goose Up
-- Kurgusal tarihsel tarifeler: v1 faktörleri, taban fiyatların %85'i.
-- Başlangıç seed'den önce de uygulanabilir; mevcut v1/v2 yerinde değiştirilmez.
CREATE TABLE simulation_runs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    seed BIGINT NOT NULL,
    parameters JSONB NOT NULL,
    params_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('RUNNING','COMPLETED','FAILED','LIVE')),
    phase TEXT NOT NULL,
    processed BIGINT NOT NULL DEFAULT 0,
    target BIGINT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    fingerprint TEXT,
    summary JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_log_occurred_at_idx ON audit_log (occurred_at);
CREATE INDEX late_interest_open_idx ON late_interest_charges (installment_id)
    WHERE paid_kurus < amount_kurus;
-- installments_open_due_idx başlangıç şemasında zaten bulunur.

INSERT INTO tariff_versions (product, version_no, valid_from, valid_to, status)
VALUES ('KASKO',0,'0001-01-01','2025-01-01','ACTIVE'),
       ('TRAFIK',0,'0001-01-01','2025-01-01','ACTIVE'),
       ('KONUT',0,'0001-01-01','2025-01-01','ACTIVE');
INSERT INTO tariff_base_rates (tariff_version_id, coverage_code, base_premium_kurus)
SELECT t.id, v.coverage, v.amount FROM tariff_versions t
JOIN (VALUES ('KASKO','KASKO_ANA',850000),('KASKO','CAM',59500),('KASKO','IMM',34000),
             ('TRAFIK','TRAFIK_MADDI',212500),('TRAFIK','TRAFIK_BEDENI',297500),
             ('KONUT','YANGIN',127500),('KONUT','HIRSIZLIK',51000)) v(product,coverage,amount)
  ON v.product=t.product WHERE t.version_no=0;
INSERT INTO tariff_factors (tariff_version_id, factor_type, factor_key, num_range, multiplier_bp)
SELECT t.id, v.typ, v.key, v.rng::int4range, v.bp FROM tariff_versions t
JOIN (VALUES
('KASKO','AGE',NULL,'[18,21)',14000),
('KASKO','AGE',NULL,'[21,26)',12500),
('KASKO','AGE',NULL,'[26,46)',10000),
('KASKO','AGE',NULL,'[46,66)',9500),
('KASKO','AGE',NULL,'[66,)',11500),
('TRAFIK','AGE',NULL,'[18,21)',17500),
('TRAFIK','AGE',NULL,'[21,26)',13000),
('TRAFIK','AGE',NULL,'[26,66)',10000),
('TRAFIK','AGE',NULL,'[66,)',12000),
('KASKO','NCD','0',NULL,10000),
('KASKO','NCD','1',NULL,9200),
('KASKO','NCD','2',NULL,8400),
('KASKO','NCD','3',NULL,7600),
('KASKO','NCD','4',NULL,6800),
('KASKO','REGION','34',NULL,11500),
('KASKO','REGION','06',NULL,10000),
('KASKO','REGION','35',NULL,10500),
('KASKO','REGION','DIGER',NULL,9500),
('KASKO','VEHICLE_GROUP','A',NULL,9000),
('KASKO','VEHICLE_GROUP','B',NULL,10000),
('KASKO','VEHICLE_GROUP','C',NULL,11500),
('KASKO','VEHICLE_GROUP','D',NULL,14000),
('TRAFIK','NCD','0',NULL,13500),
('TRAFIK','NCD','1',NULL,11500),
('TRAFIK','NCD','2',NULL,10000),
('TRAFIK','NCD','3',NULL,9000),
('TRAFIK','NCD','4',NULL,8000),
('TRAFIK','REGION','34',NULL,11500),
('TRAFIK','REGION','06',NULL,10000),
('TRAFIK','REGION','35',NULL,10500),
('TRAFIK','REGION','DIGER',NULL,9500),
('TRAFIK','VEHICLE_GROUP','A',NULL,9000),
('TRAFIK','VEHICLE_GROUP','B',NULL,10000),
('TRAFIK','VEHICLE_GROUP','C',NULL,11000),
('TRAFIK','VEHICLE_GROUP','D',NULL,13000),
('KONUT','NCD','0',NULL,10000),
('KONUT','NCD','1',NULL,9500),
('KONUT','NCD','2',NULL,9000),
('KONUT','NCD','3',NULL,8500),
('KONUT','REGION','34',NULL,13000),
('KONUT','REGION','35',NULL,12500),
('KONUT','REGION','06',NULL,10000),
('KONUT','REGION','DIGER',NULL,11000)) v(product,typ,key,rng,bp) ON v.product=t.product WHERE t.version_no=0;
INSERT INTO tariff_short_period_rates (tariff_version_id, days_range, rate_bp)
SELECT t.id, v.rng::int4range, v.bp FROM tariff_versions t
CROSS JOIN (VALUES ('[0,31)',2500),('[31,91)',4000),('[91,181)',6000),
                   ('[181,271)',8000),('[271,367)',10000)) v(rng,bp)
WHERE t.version_no=0;

-- +goose Down
-- Tarihsel tarifeye bağlı poliçe varsa FK koruması Down'ı reddeder; veri silinmez.
DELETE FROM tariff_versions WHERE version_no=0 AND product IN ('KASKO','TRAFIK','KONUT');
DROP INDEX late_interest_open_idx;
DROP INDEX audit_log_occurred_at_idx;
DROP TABLE simulation_runs;
