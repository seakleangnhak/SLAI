ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS external_payment_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS checkout_reference TEXT NULL,
    ADD COLUMN IF NOT EXISTS qr_payload TEXT NULL,
    ADD COLUMN IF NOT EXISTS qr_image_data_uri TEXT NULL,
    ADD COLUMN IF NOT EXISTS qr_md5 TEXT NULL,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS callback_received_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS provider_status TEXT NULL,
    ADD COLUMN IF NOT EXISTS provider_metadata JSONB NULL,
    ADD COLUMN IF NOT EXISTS provider_transaction_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS provider_apv TEXT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS payments_provider_external_payment_unique_idx
ON payments(provider, external_payment_id)
WHERE external_payment_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS payments_provider_checkout_reference_unique_idx
ON payments(provider, checkout_reference)
WHERE checkout_reference IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS payments_provider_transaction_paid_unique_idx
ON payments(provider, provider_transaction_id)
WHERE provider_transaction_id IS NOT NULL
  AND status = 'paid';

CREATE INDEX IF NOT EXISTS payments_provider_status_idx ON payments(provider_status);
CREATE INDEX IF NOT EXISTS payments_checkout_reference_idx ON payments(checkout_reference);
