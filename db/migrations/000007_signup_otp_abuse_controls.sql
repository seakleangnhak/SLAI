ALTER TABLE signup_email_verifications
    ADD COLUMN IF NOT EXISTS request_count INTEGER NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    ADD COLUMN IF NOT EXISTS first_requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS last_sent_at TIMESTAMPTZ NULL;

UPDATE signup_email_verifications
SET first_requested_at = COALESCE(first_requested_at, created_at),
    last_sent_at = COALESCE(last_sent_at, created_at)
WHERE last_sent_at IS NULL;

CREATE TABLE IF NOT EXISTS signup_otp_rate_limits (
    scope TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    window_start TIMESTAMPTZ NOT NULL,
    last_request_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, key_hash)
);

CREATE INDEX IF NOT EXISTS signup_otp_rate_limits_last_request_at_idx
ON signup_otp_rate_limits(last_request_at);
