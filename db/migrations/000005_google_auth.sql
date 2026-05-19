ALTER TABLE users
    ALTER COLUMN password_hash DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'password',
    ADD COLUMN IF NOT EXISTS google_subject TEXT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'users_auth_provider_check'
    ) THEN
        ALTER TABLE users
        ADD CONSTRAINT users_auth_provider_check
        CHECK (auth_provider IN ('password', 'google'));
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS users_google_subject_unique_idx
ON users(google_subject)
WHERE google_subject IS NOT NULL;
