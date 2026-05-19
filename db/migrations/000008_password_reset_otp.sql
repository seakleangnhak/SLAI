CREATE TABLE IF NOT EXISTS password_reset_otps (
    email TEXT PRIMARY KEY,
    otp_hash TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    request_count INTEGER NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    first_requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_sent_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'password_reset_otps_set_updated_at'
    ) THEN
        CREATE TRIGGER password_reset_otps_set_updated_at
        BEFORE UPDATE ON password_reset_otps
        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS password_reset_otps_expires_at_idx
ON password_reset_otps(expires_at);

CREATE TABLE IF NOT EXISTS password_reset_otp_rate_limits (
    scope TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    window_start TIMESTAMPTZ NOT NULL,
    last_request_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, key_hash)
);

CREATE INDEX IF NOT EXISTS password_reset_otp_rate_limits_last_request_at_idx
ON password_reset_otp_rate_limits(last_request_at);
