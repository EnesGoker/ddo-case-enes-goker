-- +goose NO TRANSACTION

-- +goose Up
-- Billing her tahsilatın iade toplamını bu FK üzerinden okur. CONCURRENTLY,
-- devam eden simülatör/API yazmalarını engellemeden indeksi oluşturur.
CREATE INDEX CONCURRENTLY IF NOT EXISTS payments_refund_of_payment_idx
    ON payments (refund_of_payment_id)
    WHERE refund_of_payment_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS payments_refund_of_payment_idx;
