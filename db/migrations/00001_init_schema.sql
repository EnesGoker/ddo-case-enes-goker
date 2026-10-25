-- +goose Up
-- GüvenPoliçe başlangıç şeması (PostgreSQL 16)
-- Kurallar: para = BIGINT kuruş (float yok) · oranlar = tam sayı (bp = 1/10000, ppm = 1/1.000.000)
--           tarih aralıkları [başlangıç, bitiş) yarı açık; bitiş NULL = süresiz

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- =====================================================================
-- 1. KİMLİK VE KİŞİLER
-- =====================================================================
CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    full_name     TEXT        NOT NULL,
    role          TEXT        NOT NULL CHECK (role IN
                  ('CUSTOMER','AGENT','UNDERWRITER','APPRAISER','CLAIMS_ADJUSTER','ACCOUNTING')),
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_uq ON users (lower(email));

CREATE TABLE refresh_tokens (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);

CREATE TABLE agents (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT  NOT NULL UNIQUE REFERENCES users(id),
    agency_name   TEXT    NOT NULL,
    commission_bp INTEGER NOT NULL DEFAULT 0 CHECK (commission_bp BETWEEN 0 AND 10000)
);

CREATE TABLE customers (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT UNIQUE REFERENCES users(id),
    agent_id    BIGINT REFERENCES agents(id),
    national_id TEXT        NOT NULL UNIQUE CHECK (national_id ~ '^[0-9]{11}$'),
    full_name   TEXT        NOT NULL,
    birth_date  DATE        NOT NULL,
    region_code TEXT        NOT NULL,
    phone       TEXT,
    email       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX customers_agent_idx ON customers (agent_id);

-- =====================================================================
-- 2. TARİFE (sürümlü, zaman geçerlilikli)  -> T-1
-- =====================================================================
CREATE TABLE tariff_versions (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product    TEXT    NOT NULL CHECK (product IN ('KASKO','TRAFIK','KONUT')),
    version_no INTEGER NOT NULL,
    valid_from DATE    NOT NULL,
    valid_to   DATE,
    status     TEXT    NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','RETIRED')),
    created_by BIGINT  REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product, version_no),
    CHECK (valid_to IS NULL OR valid_to > valid_from),
    -- aynı üründe aynı tarihte iki aktif tarife olamaz
    EXCLUDE USING gist (product WITH =, daterange(valid_from, valid_to, '[)') WITH &&)
        WHERE (status = 'ACTIVE')
);

CREATE TABLE tariff_base_rates (
    tariff_version_id  BIGINT NOT NULL REFERENCES tariff_versions(id) ON DELETE CASCADE,
    coverage_code      TEXT   NOT NULL,
    base_premium_kurus BIGINT NOT NULL CHECK (base_premium_kurus >= 0),
    PRIMARY KEY (tariff_version_id, coverage_code)
);

-- Çarpan tablosu: kategorik (bölge, araç grubu, hasarsızlık kademesi) ya da aralıklı (yaş)
CREATE TABLE tariff_factors (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tariff_version_id BIGINT  NOT NULL REFERENCES tariff_versions(id) ON DELETE CASCADE,
    factor_type       TEXT    NOT NULL CHECK (factor_type IN ('AGE','NCD','REGION','VEHICLE_GROUP','OTHER')),
    factor_key        TEXT,
    num_range         INT4RANGE,
    multiplier_bp     INTEGER NOT NULL CHECK (multiplier_bp > 0),   -- 10000 = x1.0000
    CHECK ((factor_key IS NOT NULL) <> (num_range IS NOT NULL))
);
CREATE UNIQUE INDEX tariff_factors_key_uq
    ON tariff_factors (tariff_version_id, factor_type, factor_key) WHERE factor_key IS NOT NULL;
ALTER TABLE tariff_factors ADD CONSTRAINT tariff_factors_range_excl
    EXCLUDE USING gist (tariff_version_id WITH =, factor_type WITH =, num_range WITH &&)
    WHERE (num_range IS NOT NULL);

-- Kısa dönem tarifesi: poliçe kaç gün yaşadıysa yıllık primin yüzde kaçı kalır -> T-2
CREATE TABLE tariff_short_period_rates (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tariff_version_id BIGINT    NOT NULL REFERENCES tariff_versions(id) ON DELETE CASCADE,
    days_range        INT4RANGE NOT NULL,                            -- [alt, üst) gün
    rate_bp           INTEGER   NOT NULL CHECK (rate_bp BETWEEN 0 AND 10000),
    EXCLUDE USING gist (tariff_version_id WITH =, days_range WITH &&)
);

-- =====================================================================
-- 3. TEKLİF  -> F-1, T-1
-- =====================================================================
CREATE TABLE quotes (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer_id         BIGINT NOT NULL REFERENCES customers(id),
    agent_id            BIGINT REFERENCES agents(id),
    created_by          BIGINT NOT NULL REFERENCES users(id),
    product             TEXT   NOT NULL CHECK (product IN ('KASKO','TRAFIK','KONUT')),
    tariff_version_id   BIGINT NOT NULL REFERENCES tariff_versions(id),
    term_start          DATE   NOT NULL,
    term_end            DATE   NOT NULL,
    risk_data           JSONB  NOT NULL,                  -- yaş, bölge, araç grubu, hasarsızlık vb. girdiler
    coverage_selection  JSONB  NOT NULL,                  -- seçilen teminat/limit/muafiyet
    input_hash          TEXT   NOT NULL,                  -- aynı girdi + aynı tarife => aynı hash/sonuç
    total_premium_kurus BIGINT NOT NULL CHECK (total_premium_kurus >= 0),
    status              TEXT   NOT NULL DEFAULT 'CALCULATED' CHECK (status IN
                        ('CALCULATED','UW_REVIEW','APPROVED','REJECTED','CONVERTED','EXPIRED')),
    underwriter_id      BIGINT REFERENCES users(id),
    underwriter_note    TEXT,
    valid_until         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (term_end > term_start)
);
CREATE INDEX quotes_customer_idx ON quotes (customer_id, created_at DESC);
CREATE INDEX quotes_status_idx   ON quotes (status, created_at) WHERE status IN ('CALCULATED','UW_REVIEW');

-- Faktör faktör döküm: hangi etken kaç TL (amount işaretli)
CREATE TABLE quote_premium_lines (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    quote_id      BIGINT  NOT NULL REFERENCES quotes(id) ON DELETE CASCADE,
    seq           INTEGER NOT NULL,
    coverage_code TEXT,
    line_type     TEXT    NOT NULL CHECK (line_type IN ('BASE','FACTOR','DISCOUNT','SURCHARGE','TAX','ROUNDING')),
    factor_type   TEXT,
    factor_label  TEXT,
    multiplier_bp INTEGER,
    amount_kurus  BIGINT  NOT NULL,
    UNIQUE (quote_id, seq)
);

-- =====================================================================
-- 4. POLİÇE, SÜRÜMLER, TEMİNATLAR, ZEYİL  -> F-2, T-2
-- =====================================================================
CREATE TABLE policies (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    policy_no             TEXT   NOT NULL UNIQUE,
    product               TEXT   NOT NULL CHECK (product IN ('KASKO','TRAFIK','KONUT')),
    customer_id           BIGINT NOT NULL REFERENCES customers(id),
    agent_id              BIGINT REFERENCES agents(id),
    quote_id              BIGINT UNIQUE REFERENCES quotes(id),
    start_date            DATE   NOT NULL,
    end_date              DATE   NOT NULL,
    status                TEXT   NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','CANCELLED','EXPIRED')),
    cancelled_at          TIMESTAMPTZ,
    cancel_effective_date DATE,
    cancel_reason         TEXT   CHECK (cancel_reason IN ('CUSTOMER_REQUEST','NON_PAYMENT','UNDERWRITER','OTHER')),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_date > start_date),
    CHECK ((status = 'CANCELLED') = (cancelled_at IS NOT NULL))
);
CREATE INDEX policies_customer_idx ON policies (customer_id, id);
CREATE INDEX policies_agent_idx    ON policies (agent_id, id);
CREATE INDEX policies_start_idx    ON policies (start_date);
CREATE INDEX policies_active_end_idx ON policies (end_date) WHERE status = 'ACTIVE';

-- Her sürüm, poliçenin bir "geçerlilik dilimi"dir; dilimler çakışamaz.
-- "Şu tarihte hangi teminatlar geçerliydi?" => effective_from <= t AND (effective_to IS NULL OR t < effective_to)
CREATE TABLE policy_versions (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    policy_id            BIGINT  NOT NULL REFERENCES policies(id),
    version_no           INTEGER NOT NULL,
    effective_from       DATE    NOT NULL,
    effective_to         DATE,
    endorsement_id       BIGINT,                                   -- FK aşağıda (döngüsel bağımlılık)
    tariff_version_id    BIGINT  NOT NULL REFERENCES tariff_versions(id),
    vehicle_plate        TEXT,
    risk_data            JSONB   NOT NULL,
    annual_premium_kurus BIGINT  NOT NULL CHECK (annual_premium_kurus >= 0),  -- 1 yıllık eşdeğer prim
    annual_limit_kurus   BIGINT  CHECK (annual_limit_kurus IS NULL OR annual_limit_kurus > 0),
    recorded_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (policy_id, version_no),
    CHECK (effective_to IS NULL OR effective_to > effective_from),
    EXCLUDE USING gist (policy_id WITH =, daterange(effective_from, effective_to, '[)') WITH &&)
);
CREATE INDEX policy_versions_plate_idx ON policy_versions (vehicle_plate) WHERE vehicle_plate IS NOT NULL;

CREATE TABLE policy_version_coverages (
    policy_version_id    BIGINT NOT NULL REFERENCES policy_versions(id) ON DELETE CASCADE,
    coverage_code        TEXT   NOT NULL,
    limit_kurus          BIGINT NOT NULL CHECK (limit_kurus >= 0),
    deductible_type      TEXT   NOT NULL DEFAULT 'NONE' CHECK (deductible_type IN ('NONE','FIXED','PERCENT_BP')),
    deductible_value     BIGINT NOT NULL DEFAULT 0 CHECK (deductible_value >= 0),   -- FIXED: kuruş, PERCENT_BP: bp
    annual_premium_kurus BIGINT NOT NULL CHECK (annual_premium_kurus >= 0),
    PRIMARY KEY (policy_version_id, coverage_code),
    CHECK (deductible_type <> 'PERCENT_BP' OR deductible_value <= 10000),
    CHECK (deductible_type <> 'NONE' OR deductible_value = 0)
);

CREATE TABLE endorsements (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    policy_id           BIGINT  NOT NULL REFERENCES policies(id),
    endorsement_no      INTEGER NOT NULL,
    type                TEXT    NOT NULL CHECK (type IN ('VEHICLE_CHANGE','COVERAGE_CHANGE','CANCELLATION','OTHER')),
    effective_date      DATE    NOT NULL,
    from_version_id     BIGINT  NOT NULL REFERENCES policy_versions(id),
    to_version_id       BIGINT  REFERENCES policy_versions(id),     -- iptalde NULL
    premium_delta_kurus BIGINT  NOT NULL,                           -- + ek prim, - iade
    calc_detail         JSONB   NOT NULL,                           -- gün sayıları, artık yıl, kısa dönem oranı, adımlar
    created_by          BIGINT  NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (policy_id, endorsement_no),
    CHECK (type = 'CANCELLATION' OR to_version_id IS NOT NULL)
);
ALTER TABLE policy_versions
    ADD CONSTRAINT policy_versions_endorsement_fk FOREIGN KEY (endorsement_id) REFERENCES endorsements(id);
CREATE INDEX endorsements_created_idx ON endorsements (created_at);

-- =====================================================================
-- 5. MUHASEBE (çift taraflı defter, ekleme-yalnız)  -> T-4
-- =====================================================================
CREATE TABLE accounts (
    id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('ASSET','LIABILITY','EQUITY','REVENUE','EXPENSE'))
);

CREATE TABLE journal_entries (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entry_date        DATE   NOT NULL DEFAULT CURRENT_DATE,
    description       TEXT   NOT NULL,
    ref_type          TEXT   NOT NULL,                              -- PAYMENT, REFUND, POLICY_ISSUE, ENDORSEMENT, LATE_INTEREST, CLAIM_PAYMENT ...
    ref_id            BIGINT NOT NULL,
    reverses_entry_id BIGINT REFERENCES journal_entries(id),
    created_by        BIGINT REFERENCES users(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX journal_entries_ref_idx ON journal_entries (ref_type, ref_id);
-- Aynı iş olayı için ikinci "asıl" kayıt atılamaz (çift kayıt koruması); ters kayıtlar serbest
CREATE UNIQUE INDEX journal_entries_one_per_event_uq
    ON journal_entries (ref_type, ref_id) WHERE reverses_entry_id IS NULL;

CREATE TABLE journal_lines (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entry_id     BIGINT NOT NULL REFERENCES journal_entries(id),
    account_id   BIGINT NOT NULL REFERENCES accounts(id),
    debit_kurus  BIGINT NOT NULL DEFAULT 0 CHECK (debit_kurus  >= 0),
    credit_kurus BIGINT NOT NULL DEFAULT 0 CHECK (credit_kurus >= 0),
    CHECK ((debit_kurus > 0) <> (credit_kurus > 0))
);
CREATE INDEX journal_lines_entry_idx   ON journal_lines (entry_id);
CREATE INDEX journal_lines_account_idx ON journal_lines (account_id);

INSERT INTO accounts (code, name, type) VALUES
    ('1000', 'Banka / Kasa',             'ASSET'),
    ('1200', 'Prim Alacakları',          'ASSET'),
    ('2000', 'Ödenecek Tazminatlar',     'LIABILITY'),
    ('2100', 'Ödenecek Prim İadeleri',   'LIABILITY'),
    ('4000', 'Prim Gelirleri',           'REVENUE'),
    ('4100', 'Gecikme Faizi Gelirleri',  'REVENUE'),
    ('5000', 'Tazminat Giderleri',       'EXPENSE');

-- =====================================================================
-- 6. TAKSİT, TAHSİLAT, İDEMPOTANS  -> F-3, T-4
-- =====================================================================
CREATE TABLE installment_plans (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    policy_id         BIGINT  NOT NULL REFERENCES policies(id),
    endorsement_id    BIGINT  REFERENCES endorsements(id),
    total_kurus       BIGINT  NOT NULL CHECK (total_kurus > 0),
    installment_count INTEGER NOT NULL CHECK (installment_count BETWEEN 1 AND 12),
    status            TEXT    NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUPERSEDED','CANCELLED')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX installment_plans_one_active_uq ON installment_plans (policy_id) WHERE status = 'ACTIVE';

CREATE TABLE installments (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    plan_id      BIGINT  NOT NULL REFERENCES installment_plans(id),
    seq          INTEGER NOT NULL CHECK (seq > 0),
    due_date     DATE    NOT NULL,
    amount_kurus BIGINT  NOT NULL CHECK (amount_kurus > 0),
    paid_kurus   BIGINT  NOT NULL DEFAULT 0,
    status       TEXT    NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PARTIAL','PAID','CANCELLED')),
    UNIQUE (plan_id, seq),
    CHECK (paid_kurus BETWEEN 0 AND amount_kurus),
    CHECK (status <> 'PAID' OR paid_kurus = amount_kurus)
);
-- "30 gün ödenmeyen taksit" taraması ve en eski taksite tahsis için
CREATE INDEX installments_open_due_idx ON installments (due_date) WHERE status IN ('PENDING','PARTIAL');
CREATE INDEX installments_plan_due_idx ON installments (plan_id, due_date);

CREATE TABLE late_interest_charges (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    installment_id          BIGINT  NOT NULL REFERENCES installments(id),
    accrued_from            DATE    NOT NULL,
    accrued_to              DATE    NOT NULL,
    overdue_principal_kurus BIGINT  NOT NULL CHECK (overdue_principal_kurus > 0),
    daily_rate_ppm          INTEGER NOT NULL CHECK (daily_rate_ppm > 0),    -- 500 ppm = %0,05 / gün
    amount_kurus            BIGINT  NOT NULL CHECK (amount_kurus > 0),
    paid_kurus              BIGINT  NOT NULL DEFAULT 0,
    CHECK (accrued_to > accrued_from),
    CHECK (paid_kurus BETWEEN 0 AND amount_kurus),
    EXCLUDE USING gist (installment_id WITH =, daterange(accrued_from, accrued_to, '[)') WITH &&)
);

CREATE TABLE payments (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    policy_id            BIGINT NOT NULL REFERENCES policies(id),
    kind                 TEXT   NOT NULL CHECK (kind IN ('PAYMENT','REFUND')),
    amount_kurus         BIGINT NOT NULL CHECK (amount_kurus > 0),
    method               TEXT   NOT NULL CHECK (method IN ('CASH','CARD','TRANSFER')),
    refund_of_payment_id BIGINT REFERENCES payments(id),
    reference            TEXT,
    paid_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    journal_entry_id     BIGINT REFERENCES journal_entries(id),
    created_by           BIGINT NOT NULL REFERENCES users(id),
    CHECK ((kind = 'REFUND') = (refund_of_payment_id IS NOT NULL))
);
CREATE INDEX payments_policy_idx ON payments (policy_id, paid_at);

-- Kısmi ödeme tahsisi: en eski taksitten başlayarak (miktar hep pozitif; yön payments.kind'dan gelir)
CREATE TABLE payment_allocations (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payment_id              BIGINT NOT NULL REFERENCES payments(id),
    installment_id          BIGINT REFERENCES installments(id),
    late_interest_charge_id BIGINT REFERENCES late_interest_charges(id),
    amount_kurus            BIGINT NOT NULL CHECK (amount_kurus > 0),
    CHECK ((installment_id IS NOT NULL) <> (late_interest_charge_id IS NOT NULL))
);
CREATE INDEX payment_allocations_payment_idx     ON payment_allocations (payment_id);
CREATE INDEX payment_allocations_installment_idx ON payment_allocations (installment_id);

CREATE TABLE idempotency_keys (
    user_id         BIGINT  NOT NULL REFERENCES users(id),
    idem_key        TEXT    NOT NULL CHECK (length(idem_key) BETWEEN 1 AND 255),
    method          TEXT    NOT NULL,
    path            TEXT    NOT NULL,
    request_hash    TEXT    NOT NULL,                     -- aynı anahtar + farklı gövde => 422
    status          TEXT    NOT NULL CHECK (status IN ('IN_PROGRESS','COMPLETED')),
    response_status INTEGER,
    response_body   JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    PRIMARY KEY (user_id, idem_key)
);
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- =====================================================================
-- 7. HASAR, EKSPERTİZ, İŞ AKIŞI, SAHTECİLİK  -> F-4, F-5, T-5
-- =====================================================================
CREATE TABLE workshops (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT NOT NULL,
    region_code TEXT NOT NULL
);

-- Durum makinesi: izinli geçişler veritabanında da tutulur
CREATE TABLE claim_transitions (
    from_status TEXT NOT NULL,
    to_status   TEXT NOT NULL,
    PRIMARY KEY (from_status, to_status)
);
INSERT INTO claim_transitions (from_status, to_status) VALUES
    ('REPORTED','TRIAGED'),    ('REPORTED','REJECTED'),
    ('TRIAGED','APPRAISAL'),   ('TRIAGED','FRAUD_REVIEW'),  ('TRIAGED','REJECTED'),
    ('APPRAISAL','ADJUSTMENT'),('APPRAISAL','FRAUD_REVIEW'),('APPRAISAL','REJECTED'),
    ('FRAUD_REVIEW','ADJUSTMENT'), ('FRAUD_REVIEW','REJECTED'),
    ('ADJUSTMENT','APPROVED'), ('ADJUSTMENT','FRAUD_REVIEW'), ('ADJUSTMENT','REJECTED'),
    ('APPROVED','PAID'),
    ('PAID','CLOSED'),         ('REJECTED','CLOSED');

CREATE TABLE claims (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_no             TEXT    NOT NULL UNIQUE,
    policy_id            BIGINT  NOT NULL REFERENCES policies(id),
    policy_version_id    BIGINT  NOT NULL REFERENCES policy_versions(id),   -- hasar tarihine göre çözülen sürüm
    coverage_code        TEXT    NOT NULL,
    loss_date            DATE    NOT NULL,
    reported_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    plate                TEXT,
    workshop_id          BIGINT  REFERENCES workshops(id),
    description          TEXT    NOT NULL,
    claimed_amount_kurus BIGINT  NOT NULL CHECK (claimed_amount_kurus >= 0),
    status               TEXT    NOT NULL DEFAULT 'REPORTED' CHECK (status IN
                         ('REPORTED','TRIAGED','APPRAISAL','ADJUSTMENT','FRAUD_REVIEW','APPROVED','REJECTED','PAID','CLOSED')),
    fraud_score          INTEGER NOT NULL DEFAULT 0 CHECK (fraud_score >= 0),
    fraud_flagged        BOOLEAN NOT NULL DEFAULT FALSE,
    row_version          INTEGER NOT NULL DEFAULT 1,                         -- iyimser kilit
    created_by           BIGINT  NOT NULL REFERENCES users(id),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- seçilen teminat, çözülen sürümde gerçekten var mı
    FOREIGN KEY (policy_version_id, coverage_code)
        REFERENCES policy_version_coverages (policy_version_id, coverage_code)
);
CREATE INDEX claims_policy_loss_idx    ON claims (policy_id, loss_date);
CREATE INDEX claims_plate_loss_idx     ON claims (plate, loss_date) WHERE plate IS NOT NULL;
CREATE INDEX claims_workshop_idx       ON claims (workshop_id, reported_at) WHERE workshop_id IS NOT NULL;
CREATE INDEX claims_status_idx         ON claims (status, id);
CREATE INDEX claims_fraud_queue_idx    ON claims (id) WHERE fraud_flagged AND status = 'FRAUD_REVIEW';

CREATE TABLE claim_status_history (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_id      BIGINT NOT NULL REFERENCES claims(id),
    from_status   TEXT,
    to_status     TEXT   NOT NULL,
    actor_user_id BIGINT REFERENCES users(id),
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- ilk kayıtta from_status NULL: bileşik FK devreye girmez (MATCH SIMPLE)
    FOREIGN KEY (from_status, to_status) REFERENCES claim_transitions (from_status, to_status),
    CHECK (from_status IS NOT NULL OR to_status = 'REPORTED')
);
CREATE INDEX claim_status_history_claim_idx ON claim_status_history (claim_id, id);

CREATE TABLE claim_tasks (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_id     BIGINT NOT NULL REFERENCES claims(id),
    task_type    TEXT   NOT NULL CHECK (task_type IN ('APPRAISAL','ADJUSTMENT','FRAUD_REVIEW')),
    assigned_to  BIGINT REFERENCES users(id),
    status       TEXT   NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','IN_PROGRESS','DONE','CANCELLED')),
    sla_due_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CHECK (status <> 'DONE' OR completed_at IS NOT NULL)
);
CREATE UNIQUE INDEX claim_tasks_one_open_uq ON claim_tasks (claim_id, task_type) WHERE status IN ('OPEN','IN_PROGRESS');
CREATE INDEX claim_tasks_load_idx ON claim_tasks (assigned_to)  WHERE status IN ('OPEN','IN_PROGRESS');   -- en az yüklü uzman
CREATE INDEX claim_tasks_sla_idx  ON claim_tasks (sla_due_at)   WHERE status IN ('OPEN','IN_PROGRESS');   -- SLA ihlal taraması

CREATE TABLE appraisals (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_id            BIGINT NOT NULL REFERENCES claims(id),
    appraiser_id        BIGINT NOT NULL REFERENCES users(id),
    inspected_at        TIMESTAMPTZ NOT NULL,
    damage_amount_kurus BIGINT NOT NULL CHECK (damage_amount_kurus >= 0),
    is_total_loss       BOOLEAN NOT NULL DEFAULT FALSE,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX appraisals_claim_idx ON appraisals (claim_id);

CREATE TABLE fraud_signals (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_id   BIGINT  NOT NULL REFERENCES claims(id),
    rule_code  TEXT    NOT NULL,                          -- EARLY_CLAIM, REPEAT_PLATE, WORKSHOP_CONCENTRATION ...
    points     INTEGER NOT NULL CHECK (points > 0),
    detail     JSONB   NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (claim_id, rule_code)
);

-- =====================================================================
-- 8. TAZMİNAT, LİMİT TÜKETİMİ  -> F-6, T-3
-- =====================================================================
-- Limit sayacı: koşullu UPDATE ... WHERE used + x <= limit ile eşzamanlı onayda taşma engellenir.
-- coverage_code NULL = yıllık toplam limit
CREATE TABLE policy_limit_balances (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    policy_id     BIGINT NOT NULL REFERENCES policies(id),
    coverage_code TEXT,
    period_start  DATE   NOT NULL,
    period_end    DATE   NOT NULL,
    limit_kurus   BIGINT NOT NULL CHECK (limit_kurus >= 0),
    used_kurus    BIGINT NOT NULL DEFAULT 0 CHECK (used_kurus >= 0),
    CHECK (period_end > period_start),
    CHECK (used_kurus <= limit_kurus),
    UNIQUE NULLS NOT DISTINCT (policy_id, coverage_code, period_start)
);

CREATE TABLE claim_settlements (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_id                BIGINT NOT NULL REFERENCES claims(id),
    gross_damage_kurus      BIGINT NOT NULL CHECK (gross_damage_kurus >= 0),
    deductible_kurus        BIGINT NOT NULL CHECK (deductible_kurus >= 0),
    payable_kurus           BIGINT NOT NULL CHECK (payable_kurus >= 0),
    calc_steps              JSONB  NOT NULL,              -- adım adım: hasar -> muafiyet -> teminat limiti -> yıllık limit
    status                  TEXT   NOT NULL DEFAULT 'APPROVED' CHECK (status IN ('APPROVED','PAID','CANCELLED')),
    approved_by             BIGINT NOT NULL REFERENCES users(id),
    approved_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at                 TIMESTAMPTZ,
    payment_journal_entry_id BIGINT REFERENCES journal_entries(id),
    CHECK (status <> 'PAID' OR paid_at IS NOT NULL)
);
CREATE UNIQUE INDEX claim_settlements_one_active_uq ON claim_settlements (claim_id) WHERE status <> 'CANCELLED';

CREATE TABLE limit_consumptions (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    settlement_id BIGINT NOT NULL REFERENCES claim_settlements(id),
    balance_id    BIGINT NOT NULL REFERENCES policy_limit_balances(id),
    amount_kurus  BIGINT NOT NULL CHECK (amount_kurus > 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (settlement_id, balance_id)
);

-- =====================================================================
-- 9. DENETİM KAYDI
-- =====================================================================
CREATE TABLE audit_log (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_user_id BIGINT REFERENCES users(id),
    actor_role    TEXT,
    action        TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    entity_id     TEXT NOT NULL,
    old_value     JSONB,
    new_value     JSONB,
    request_id    TEXT
);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, occurred_at);
CREATE INDEX audit_log_actor_idx  ON audit_log (actor_user_id, occurred_at);

-- =====================================================================
-- 10. TETİKLEYİCİLER: ekleme-yalnız + defter dengesi
-- =====================================================================
-- +goose StatementBegin
CREATE FUNCTION forbid_modify() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% on % is not allowed (append-only table)', TG_OP, TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION forbid_modify();
CREATE TRIGGER journal_entries_append_only BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION forbid_modify();
CREATE TRIGGER journal_lines_append_only BEFORE UPDATE OR DELETE ON journal_lines
    FOR EACH ROW EXECUTE FUNCTION forbid_modify();
CREATE TRIGGER claim_status_history_append_only BEFORE UPDATE OR DELETE ON claim_status_history
    FOR EACH ROW EXECUTE FUNCTION forbid_modify();

-- Her yevmiye kaydı transaction sonunda borç = alacak olmak zorunda
-- +goose StatementBegin
CREATE FUNCTION check_journal_balanced() RETURNS trigger AS $$
DECLARE
    d BIGINT;
    c BIGINT;
BEGIN
    SELECT COALESCE(SUM(debit_kurus), 0), COALESCE(SUM(credit_kurus), 0)
      INTO d, c
      FROM journal_lines
     WHERE entry_id = NEW.entry_id;
    IF d <> c THEN
        RAISE EXCEPTION 'journal entry % is not balanced (debit=%, credit=%)', NEW.entry_id, d, c
            USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER journal_lines_balanced
    AFTER INSERT ON journal_lines
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_journal_balanced();

-- +goose Down
DROP TABLE IF EXISTS
    audit_log, limit_consumptions, claim_settlements, policy_limit_balances,
    fraud_signals, appraisals, claim_tasks, claim_status_history, claims,
    claim_transitions, workshops,
    idempotency_keys, payment_allocations, payments, late_interest_charges,
    installments, installment_plans,
    journal_lines, journal_entries, accounts,
    policy_version_coverages, endorsements, policy_versions, policies,
    quote_premium_lines, quotes,
    tariff_short_period_rates, tariff_factors, tariff_base_rates, tariff_versions,
    customers, agents, refresh_tokens, users
    CASCADE;
DROP FUNCTION IF EXISTS check_journal_balanced();
DROP FUNCTION IF EXISTS forbid_modify();
