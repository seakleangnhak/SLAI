ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS reviewed_by_admin_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS rejection_reason TEXT NULL,
    ADD COLUMN IF NOT EXISTS admin_payment_reference TEXT NULL,
    ADD COLUMN IF NOT EXISTS admin_payment_reference_normalized TEXT NULL,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'payments_set_updated_at'
    ) THEN
        CREATE TRIGGER payments_set_updated_at
        BEFORE UPDATE ON payments
        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS payments_admin_reference_paid_unique_idx
ON payments(provider, admin_payment_reference_normalized)
WHERE admin_payment_reference_normalized IS NOT NULL
  AND status = 'paid';

CREATE INDEX IF NOT EXISTS payments_status_idx ON payments(status);
CREATE INDEX IF NOT EXISTS payments_provider_idx ON payments(provider);
CREATE INDEX IF NOT EXISTS payments_reviewed_by_admin_id_idx ON payments(reviewed_by_admin_id);

CREATE TABLE IF NOT EXISTS payment_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    display_name TEXT NOT NULL,
    account_name TEXT NULL,
    account_id TEXT NULL,
    khqr_image_path TEXT NULL,
    khqr_image_mime TEXT NULL,
    instructions TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'payment_settings_set_updated_at'
    ) THEN
        CREATE TRIGGER payment_settings_set_updated_at
        BEFORE UPDATE ON payment_settings
        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END $$;

INSERT INTO payment_settings (provider, display_name, enabled)
VALUES ('bakong_khqr', 'Bakong KHQR', false)
ON CONFLICT (provider) DO NOTHING;

CREATE TABLE IF NOT EXISTS payment_proofs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id UUID NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_mime TEXT NOT NULL,
    file_size BIGINT NOT NULL CHECK (file_size >= 0),
    file_sha256 TEXT NOT NULL,
    user_transaction_ref TEXT NULL,
    user_note TEXT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS payment_proofs_payment_id_idx ON payment_proofs(payment_id);
CREATE INDEX IF NOT EXISTS payment_proofs_user_id_idx ON payment_proofs(user_id);
CREATE INDEX IF NOT EXISTS payment_proofs_file_sha256_idx ON payment_proofs(file_sha256);
