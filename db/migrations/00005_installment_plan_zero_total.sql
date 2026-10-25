-- +goose Up
-- Tam prim azaltımında iptal olmayan taksitlerin toplamı sıfır olabilir.
ALTER TABLE installment_plans
    DROP CONSTRAINT installment_plans_total_kurus_check;
ALTER TABLE installment_plans
    ADD CONSTRAINT installment_plans_total_kurus_check CHECK (total_kurus >= 0);

-- +goose Down
-- Sıfır toplamlı plan varsa Down başarısız olabilir; veri silinmez/değiştirilmez.
ALTER TABLE installment_plans
    DROP CONSTRAINT installment_plans_total_kurus_check;
ALTER TABLE installment_plans
    ADD CONSTRAINT installment_plans_total_kurus_check CHECK (total_kurus > 0);
