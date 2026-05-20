CREATE TABLE IF NOT EXISTS user_email_notifications (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    alert_key TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, alert_key)
);

CREATE INDEX IF NOT EXISTS user_email_notifications_kind_sent_at_idx
ON user_email_notifications(kind, sent_at);
