CREATE TABLE IF NOT EXISTS signup_email_verifications (
    email TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    otp_hash TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'signup_email_verifications_set_updated_at'
    ) THEN
        CREATE TRIGGER signup_email_verifications_set_updated_at
        BEFORE UPDATE ON signup_email_verifications
        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS signup_email_verifications_expires_at_idx
ON signup_email_verifications(expires_at);
