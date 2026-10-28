package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Fraud runs the automatic risk checks and stores alerts for staff.
type Fraud struct {
	DB    *sql.DB
	Audit *Audit
}

// Each check is one INSERT ... SELECT. dedupe_key keeps one alert per
// subject per kind per window, so re-running a check is harmless.
var fraudChecks = []struct {
	name string
	sql  string
}{
	{"high_value_spike", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'high_value_spike', 'medium', l.seller_id,
		       count(*) || ' cards near their price limit listed in 24 hours',
		       jsonb_build_object('listings', count(*), 'value_cents', sum(l.price_cents), 'tier', sp.tier),
		       'high_value_spike:' || l.seller_id || ':' || to_char(now(), 'YYYY-MM-DD')
		FROM listings l JOIN seller_profiles sp ON sp.user_id = l.seller_id
		WHERE l.created_at > now() - interval '24 hours'
		  AND l.price_cents >= 0.8 * (CASE sp.tier WHEN 1 THEN 2000 WHEN 2 THEN 25000 ELSE 100000 END)
		GROUP BY l.seller_id, sp.tier HAVING count(*) > 20
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"value_jump", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'high_value_spike', 'medium', x.seller_id,
		       'Listed value jumped more than tenfold in a day',
		       jsonb_build_object('today_cents', x.today, 'daily_average_cents', x.avg30),
		       'value_jump:' || x.seller_id || ':' || to_char(now(), 'YYYY-MM-DD')
		FROM (
			SELECT seller_id,
			       sum(price_cents * quantity) FILTER (WHERE created_at > now() - interval '24 hours') AS today,
			       coalesce(sum(price_cents * quantity) FILTER (WHERE created_at <= now() - interval '24 hours'
			                AND created_at > now() - interval '31 days'), 0) / 30 AS avg30,
			       min(created_at) AS first
			FROM listings GROUP BY seller_id
		) x
		WHERE x.today > 50000 AND x.first < now() - interval '7 days' AND x.today > 10 * greatest(x.avg30, 1)
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"takeover_ips", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'takeover_ips', 'high', e.user_id,
		       'Signed in from ' || count(DISTINCT e.ip) || ' different networks in 24 hours',
		       jsonb_build_object('ips', array_agg(DISTINCT e.ip)),
		       'takeover_ips:' || e.user_id || ':' || to_char(now(), 'YYYY-MM-DD')
		FROM login_events e
		WHERE e.success AND e.user_id IS NOT NULL AND e.created_at > now() - interval '24 hours'
		GROUP BY e.user_id HAVING count(DISTINCT e.ip) >= 3
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"failed_logins", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'failed_logins', 'medium', e.user_id,
		       count(*) || ' wrong passwords in an hour',
		       jsonb_build_object('attempts', count(*), 'ips', array_agg(DISTINCT e.ip)),
		       'failed_logins:' || e.user_id || ':' || to_char(now(), 'YYYY-MM-DD-HH24')
		FROM login_events e
		WHERE NOT e.success AND e.reason = 'bad_password' AND e.user_id IS NOT NULL AND e.created_at > now() - interval '1 hour'
		GROUP BY e.user_id HAVING count(*) >= 10
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"off_platform", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'off_platform', 'high', m.sender_id,
		       count(*) || ' messages blocked for off-site payment or contact in 24 hours',
		       jsonb_build_object('blocked', count(*), 'threads', count(DISTINCT m.thread_id)),
		       'off_platform:' || m.sender_id || ':' || to_char(now(), 'YYYY-MM-DD')
		FROM messages m WHERE m.blocked IS NOT NULL AND m.created_at > now() - interval '24 hours'
		GROUP BY m.sender_id HAVING count(*) >= 3
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"repeat_reports", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'repeat_reports', 'medium', r.target_user_id,
		       count(*) || ' open reports from ' || count(DISTINCT r.reporter_id) || ' members',
		       jsonb_build_object('reports', count(*), 'reasons', array_agg(DISTINCT r.reason)),
		       'repeat_reports:' || r.target_user_id || ':' || to_char(now(), 'IYYY-IW')
		FROM reports r WHERE r.status = 'open' AND r.target_user_id IS NOT NULL
		GROUP BY r.target_user_id HAVING count(*) >= 3
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"staff_mfa_locked", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'staff_mfa_locked', 'high', a.actor_id,
		       'Staff access locked after 5 wrong authenticator codes',
		       jsonb_build_object('ip', a.ip, 'at', a.at),
		       'staff_mfa_locked:' || a.id
		FROM audit_log a JOIN users u ON u.id = a.actor_id
		WHERE a.action = 'staff.mfa_locked' AND a.at > now() - interval '2 days'
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"shill_shared_ip", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, listing_id, summary, details, dedupe_key)
		SELECT DISTINCT ON (b.bidder_id, l.id) 'shill_shared_ip', 'high', b.bidder_id, l.id,
		       'Bidder shares a network with the seller',
		       jsonb_build_object('seller_id', l.seller_id, 'ip', b.ip),
		       'shill_shared_ip:' || b.bidder_id || ':' || l.id
		FROM bids b JOIN listings l ON l.id = b.listing_id
		JOIN login_events e ON e.user_id = l.seller_id AND e.success AND e.ip = b.ip AND e.created_at > now() - interval '30 days'
		WHERE b.created_at > now() - interval '2 days' AND b.ip IS NOT NULL AND b.ip <> ''
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"shill_pattern", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'shill_pattern', 'medium', x.bidder_id,
		       'Bid on ' || x.auctions || ' of one seller''s auctions without winning any',
		       jsonb_build_object('seller_id', x.seller_id, 'auctions', x.auctions, 'bids', x.bids),
		       'shill_pattern:' || x.bidder_id || ':' || x.seller_id || ':' || to_char(now(), 'IYYY-IW')
		FROM (
			SELECT b.bidder_id, l.seller_id, count(DISTINCT l.id) AS auctions, count(*) AS bids,
			       count(DISTINCT l.id) FILTER (WHERE l.winner_id = b.bidder_id) AS won,
			       (SELECT count(DISTINCT l2.seller_id) FROM bids b2 JOIN listings l2 ON l2.id = b2.listing_id WHERE b2.bidder_id = b.bidder_id) AS sellers
			FROM bids b JOIN listings l ON l.id = b.listing_id
			WHERE b.created_at > now() - interval '30 days' AND b.removed_at IS NULL
			GROUP BY b.bidder_id, l.seller_id
		) x
		WHERE x.auctions >= 3 AND x.won = 0 AND x.sellers <= 2
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"bid_burst", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, listing_id, summary, details, dedupe_key)
		SELECT 'bid_burst', 'low', l.seller_id, l.id,
		       count(*) || ' bids from ' || count(DISTINCT b.bidder_id) || ' bidders in 10 minutes',
		       jsonb_build_object('bids', count(*), 'bidders', count(DISTINCT b.bidder_id), 'price_cents', max(l.price_cents)),
		       'bid_burst:' || l.id || ':' || l.auction_round
		FROM bids b JOIN listings l ON l.id = b.listing_id
		WHERE b.created_at > now() - interval '10 minutes'
		GROUP BY l.id, l.seller_id, l.auction_round HAVING count(*) >= 6 AND count(DISTINCT b.bidder_id) <= 2
		ON CONFLICT (dedupe_key) DO NOTHING`},
	{"unpaid_items", `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		SELECT 'unpaid_items', 'medium', h.buyer_id,
		       count(*) || ' won or accepted cards not paid for in 30 days',
		       jsonb_build_object('unpaid', count(*)),
		       'unpaid_items:' || h.buyer_id || ':' || to_char(now(), 'IYYY-IW')
		FROM holds h WHERE h.status IN ('expired', 'cancelled') AND h.updated_at > now() - interval '30 days'
		GROUP BY h.buyer_id HAVING count(*) >= 2
		ON CONFLICT (dedupe_key) DO NOTHING`},
}

// Run executes every check and returns how many new alerts were raised.
func (f *Fraud) Run(ctx context.Context) (int, error) {
	total := 0
	for _, c := range fraudChecks {
		res, err := f.DB.ExecContext(ctx, c.sql)
		if err != nil {
			return total, errors.New(c.name + ": " + err.Error())
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

// Alert is one fraud alert.
type Alert struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Severity   string          `json:"severity"`
	SubjectID  *string         `json:"subject_user_id"`
	Subject    string          `json:"subject"`
	Status     string          `json:"subject_status"`
	ListingID  *string         `json:"listing_id"`
	Listing    string          `json:"listing_title"`
	Summary    string          `json:"summary"`
	Details    json.RawMessage `json:"details"`
	State      string          `json:"status"`
	ReviewedBy string          `json:"reviewed_by"`
	ReviewedAt *time.Time      `json:"reviewed_at"`
	Note       *string         `json:"review_note"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Alerts lists alerts; open ones by severity then newest.
func (f *Fraud) Alerts(ctx context.Context, status, kind string, limit int) ([]Alert, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := f.DB.QueryContext(ctx, `
		SELECT a.id, a.kind, a.severity, a.subject_user_id::text, coalesce(u.username, ''), coalesce(u.status, ''),
		       a.listing_id::text, coalesce(l.title, ''), a.summary, a.details, a.status, coalesce(r.email, ''),
		       a.reviewed_at, a.review_note, a.created_at
		FROM fraud_alerts a LEFT JOIN users u ON u.id = a.subject_user_id
		LEFT JOIN listings l ON l.id = a.listing_id LEFT JOIN users r ON r.id = a.reviewed_by
		WHERE ($1 = '' OR a.status = $1) AND ($2 = '' OR a.kind = $2)
		ORDER BY CASE a.severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, a.created_at DESC LIMIT $3`, status, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		var sid, lid, note sql.NullString
		var at sql.NullTime
		if err := rows.Scan(&a.ID, &a.Kind, &a.Severity, &sid, &a.Subject, &a.Status, &lid, &a.Listing, &a.Summary, &a.Details,
			&a.State, &a.ReviewedBy, &at, &note, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.SubjectID, a.ListingID, a.Note, a.ReviewedAt = nstr(sid), nstr(lid), nstr(note), ntime(at)
		out = append(out, a)
	}
	return out, rows.Err()
}

// Review marks an alert reviewed (something was done) or dismissed.
func (f *Fraud) Review(ctx context.Context, a Actor, id, status, note string) error {
	if status != "reviewed" && status != "dismissed" {
		return &FieldError{"status", "reviewed or dismissed"}
	}
	note, err := needReason(note)
	if err != nil {
		return err
	}
	tx, err := f.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kind string
	var subject sql.NullString
	err = tx.QueryRowContext(ctx, `UPDATE fraud_alerts SET status = $2, reviewed_by = $3, reviewed_at = now(), review_note = $4
		WHERE id::text = $1 AND status = 'open' RETURNING kind, subject_user_id::text`, id, status, a.ID, note).Scan(&kind, &subject)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAlreadyHandled
	}
	if err != nil {
		return err
	}
	if err := f.Audit.Log(ctx, tx, a.entry("fraud."+status, "user", subject.String, note, map[string]any{"alert_id": id, "kind": kind})); err != nil {
		return err
	}
	return tx.Commit()
}
