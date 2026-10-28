-- Automatic fraud and risk alerts for staff review.
CREATE TABLE fraud_alerts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind             TEXT NOT NULL,      -- high_value_spike, takeover_ips, failed_logins, off_platform, repeat_reports,
                                         -- staff_mfa_locked, shill_shared_ip, shill_pattern, bid_burst, unpaid_items
    severity         TEXT NOT NULL CHECK (severity IN ('low', 'medium', 'high')),
    subject_user_id  UUID REFERENCES users (id) ON DELETE CASCADE,
    listing_id       UUID REFERENCES listings (id) ON DELETE SET NULL,
    summary          TEXT NOT NULL,
    details          JSONB NOT NULL DEFAULT '{}',
    dedupe_key       TEXT NOT NULL UNIQUE,  -- one alert per subject per kind per window
    status           TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewed', 'dismissed')),
    reviewed_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at      TIMESTAMPTZ,
    review_note      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX fraud_alerts_status_idx ON fraud_alerts (status, created_at DESC);
CREATE INDEX fraud_alerts_subject_idx ON fraud_alerts (subject_user_id, created_at DESC);
