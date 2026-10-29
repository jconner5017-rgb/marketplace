-- Searches a member saved, optionally with a notification when new cards
-- match. query is the /search query string, normalized (sorted, no paging).
CREATE TABLE saved_searches (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name             TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    query            TEXT NOT NULL CHECK (length(query) <= 1000),
    notify           BOOLEAN NOT NULL DEFAULT true,
    checked_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_notified_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, query)
);
CREATE INDEX saved_searches_due_idx ON saved_searches (checked_at) WHERE notify;
