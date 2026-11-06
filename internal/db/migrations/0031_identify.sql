-- Photo identification on the listing form: one row per identification, so
-- each seller can be held to a daily limit.
CREATE TABLE identify_usage (
    id         BIGSERIAL PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX identify_usage_user_time ON identify_usage (user_id, created_at);
