-- The network each account signed up from, for the daily per-network cap
-- on new accounts and for staff.
ALTER TABLE users ADD COLUMN signup_ip TEXT;
CREATE INDEX users_signup_ip_idx ON users (signup_ip, created_at DESC) WHERE signup_ip IS NOT NULL;
