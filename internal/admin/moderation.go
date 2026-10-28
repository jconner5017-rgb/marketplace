package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrReportTarget   = errors.New("that can't be reported")
	ErrReportOwn      = errors.New("you can't report yourself or your own listing")
	ErrAlreadyHandled = errors.New("this report has already been handled")
)

// ReportReasons members can choose from.
var ReportReasons = []string{"counterfeit", "prohibited", "keyword_spam", "misdescribed", "off_platform", "abuse", "shill_bidding", "other"}

// Moderation covers member reports and staff takedowns of listings.
type Moderation struct {
	DB    *sql.DB
	Audit *Audit
}

// Submit records a member's report. Reporting the same thing twice while
// the first is open is fine (it's a no-op).
func (m *Moderation) Submit(ctx context.Context, reporterID, targetType, targetID, reason, details string) error {
	if !slices.Contains(ReportReasons, reason) {
		return &FieldError{"reason", "choose a reason"}
	}
	details = strings.TrimSpace(details)
	if utf8.RuneCountInString(details) > 2000 {
		return &FieldError{"details", "keep it under 2,000 characters"}
	}
	var owner string
	var err error
	switch targetType {
	case "listing":
		err = m.DB.QueryRowContext(ctx, `SELECT seller_id FROM listings WHERE id::text = $1 AND status <> 'removed'`, targetID).Scan(&owner)
	case "user":
		err = m.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE id::text = $1`, targetID).Scan(&owner)
	case "message":
		// Only someone in the conversation can report a message.
		err = m.DB.QueryRowContext(ctx, `SELECT m.sender_id FROM messages m JOIN message_threads t ON t.id = m.thread_id
			WHERE m.id::text = $1 AND (t.buyer_id = $2 OR t.seller_id = $2)`, targetID, reporterID).Scan(&owner)
	case "offer":
		err = m.DB.QueryRowContext(ctx, `SELECT CASE WHEN from_role = 'buyer' THEN buyer_id ELSE seller_id END FROM offers
			WHERE id::text = $1 AND (buyer_id = $2 OR seller_id = $2)`, targetID, reporterID).Scan(&owner)
	default:
		return ErrReportTarget
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrReportTarget
	}
	if err != nil {
		return err
	}
	if owner == reporterID {
		return ErrReportOwn
	}
	_, err = m.DB.ExecContext(ctx, `INSERT INTO reports (reporter_id, target_type, target_id, target_user_id, reason, details)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, reporterID, targetType, targetID, owner, reason, details)
	return err
}

// Report is one row of the reports queue.
type Report struct {
	ID             string     `json:"id"`
	TargetType     string     `json:"target_type"`
	TargetID       string     `json:"target_id"`
	TargetUserID   *string    `json:"target_user_id"`
	TargetUser     string     `json:"target_user"`
	TargetSummary  string     `json:"target_summary"`
	Reason         string     `json:"reason"`
	Details        string     `json:"details"`
	Status         string     `json:"status"`
	Reporter       string     `json:"reporter"`
	ReporterID     *string    `json:"reporter_id"`
	OpenOnTarget   int        `json:"open_on_target"` // open reports on the same thing
	ResolvedBy     string     `json:"resolved_by"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	ResolutionNote string     `json:"resolution_note"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Reports lists the queue: open ones oldest first, others newest first.
func (m *Moderation) Reports(ctx context.Context, status, targetType, targetID string, limit int) ([]Report, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	order := "r.created_at DESC"
	if status == "open" {
		order = "r.created_at ASC"
	}
	rows, err := m.DB.QueryContext(ctx, `
		SELECT r.id, r.target_type, r.target_id, r.target_user_id::text, coalesce(tu.username, ''),
		       coalesce(CASE r.target_type
		           WHEN 'listing' THEN (SELECT l.title || ' · ' || l.status FROM listings l WHERE l.id::text = r.target_id)
		           WHEN 'user' THEN (SELECT u.username || ' · ' || u.status FROM users u WHERE u.id::text = r.target_id)
		           WHEN 'message' THEN (SELECT left(m.body, 200) FROM messages m WHERE m.id::text = r.target_id)
		           WHEN 'offer' THEN (SELECT '$' || to_char(o.amount_cents / 100.0, 'FM999990.00') || ' offer · ' || o.status FROM offers o WHERE o.id::text = r.target_id)
		       END, '(gone)'),
		       r.reason, r.details, r.status, coalesce(ru.username, '(deleted)'), r.reporter_id::text,
		       (SELECT count(*) FROM reports x WHERE x.target_type = r.target_type AND x.target_id = r.target_id AND x.status = 'open'),
		       coalesce(sb.email, ''), r.resolved_at, coalesce(r.resolution_note, ''), r.created_at
		FROM reports r
		LEFT JOIN users tu ON tu.id = r.target_user_id
		LEFT JOIN users ru ON ru.id = r.reporter_id
		LEFT JOIN users sb ON sb.id = r.resolved_by
		WHERE ($1 = '' OR r.status = $1) AND ($2 = '' OR r.target_type = $2) AND ($3 = '' OR r.target_id = $3)
		ORDER BY `+order+` LIMIT $4`, status, targetType, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		var tu, rid sql.NullString
		var at sql.NullTime
		if err := rows.Scan(&r.ID, &r.TargetType, &r.TargetID, &tu, &r.TargetUser, &r.TargetSummary, &r.Reason, &r.Details,
			&r.Status, &r.Reporter, &rid, &r.OpenOnTarget, &r.ResolvedBy, &at, &r.ResolutionNote, &r.CreatedAt); err != nil {
			return nil, err
		}
		if tu.Valid {
			r.TargetUserID = &tu.String
		}
		if rid.Valid {
			r.ReporterID = &rid.String
		}
		if at.Valid {
			r.ResolvedAt = &at.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Resolve closes a report as actioned or dismissed, and with all=true every
// other open report on the same thing too. The reporter is told it was reviewed.
func (m *Moderation) Resolve(ctx context.Context, a Actor, id, status, note string, all bool) (int, error) {
	if status != "actioned" && status != "dismissed" {
		return 0, &FieldError{"status", "actioned or dismissed"}
	}
	note, err := needReason(note)
	if err != nil {
		return 0, err
	}
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var ttype, tid, cur string
	err = tx.QueryRowContext(ctx, `SELECT target_type, target_id, status FROM reports WHERE id::text = $1 FOR UPDATE`, id).Scan(&ttype, &tid, &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if cur != "open" {
		return 0, ErrAlreadyHandled
	}
	q := `UPDATE reports SET status = $2, resolved_by = $3, resolved_at = now(), resolution_note = $4 WHERE id::text = $1
		RETURNING reporter_id::text`
	args := []any{id, status, a.ID, note}
	if all {
		q = `UPDATE reports SET status = $2, resolved_by = $3, resolved_at = now(), resolution_note = $4
			WHERE (id::text = $1 OR (target_type = $5 AND target_id = $6)) AND status = 'open' RETURNING reporter_id::text`
		args = append(args, ttype, tid)
	}
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	var reporters []string
	for rows.Next() {
		var r sql.NullString
		if err := rows.Scan(&r); err != nil {
			rows.Close()
			return 0, err
		}
		if r.Valid {
			reporters = append(reporters, r.String)
		}
	}
	rows.Close()
	for _, r := range reporters {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body) VALUES ($1, 'system', $2, $3)`,
			r, "Thanks for your report", "Our team reviewed it. We don't share the outcome, but every report is read."); err != nil {
			return 0, err
		}
	}
	if err := m.Audit.Log(ctx, tx, a.entry("report."+status, ttype, tid, note,
		map[string]any{"report_id": id, "closed": len(reporters)})); err != nil {
		return 0, err
	}
	return len(reporters), tx.Commit()
}

// OpenReports counts open reports (for the dashboard).
func (m *Moderation) OpenReports(ctx context.Context) (int, error) {
	var n int
	err := m.DB.QueryRowContext(ctx, `SELECT count(*) FROM reports WHERE status = 'open'`).Scan(&n)
	return n, err
}

// ListingRow is a listing as staff see it.
type ListingRow struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Seller      string     `json:"seller"`
	SellerID    string     `json:"seller_id"`
	SKU         *string    `json:"external_id"`
	Category    string     `json:"category"`
	Game        string     `json:"game_or_sport"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
	PriceCents  int        `json:"price_cents"`
	Quantity    int        `json:"quantity"`
	BidCount    int        `json:"bid_count"`
	Auction     *string    `json:"auction_state"`
	EndsAt      *time.Time `json:"ends_at"`
	Offers      bool       `json:"offers_enabled"`
	Photo       string     `json:"photo"`
	OpenReports int        `json:"open_reports"`
	Moderation  *string    `json:"moderation_reason"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ListingQuery filters the staff listing search.
type ListingQuery struct {
	Q, Status, Seller, Format string
	Reported                  bool
	MinCents, MaxCents        int
	Limit, Offset             int
}

// Listings searches every listing in any status.
func (m *Moderation) Listings(ctx context.Context, q ListingQuery) ([]ListingRow, int, error) {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 50
	}
	where := []string{"true"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if s := strings.TrimSpace(q.Q); s != "" {
		add("(l.id::text = ? OR l.title ILIKE '%' || ? || '%' OR l.external_id = ?)", s)
	}
	if q.Status != "" {
		add("l.status = ?", q.Status)
	}
	if q.Seller != "" {
		add("lower(u.username) = lower(?)", strings.TrimSpace(q.Seller))
	}
	if q.Format == "auction" || q.Format == "fixed" {
		add("l.format = ?", q.Format)
	}
	if q.Reported {
		where = append(where, "EXISTS (SELECT 1 FROM reports r WHERE r.target_type = 'listing' AND r.target_id = l.id::text AND r.status = 'open')")
	}
	if q.MinCents > 0 {
		add("l.price_cents >= ?", q.MinCents)
	}
	if q.MaxCents > 0 {
		add("l.price_cents <= ?", q.MaxCents)
	}
	w := " WHERE " + strings.Join(where, " AND ")
	var total int
	if err := m.DB.QueryRowContext(ctx, `SELECT count(*) FROM listings l JOIN users u ON u.id = l.seller_id`+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, q.Limit, q.Offset)
	rows, err := m.DB.QueryContext(ctx, `
		SELECT l.id, l.title, u.username, u.id, l.external_id, l.category, l.game_or_sport, l.format, l.status, l.price_cents,
		       l.quantity, l.bid_count, l.auction_state, l.ends_at, l.offers_enabled, coalesce(l.photo_urls[1], ''),
		       (SELECT count(*) FROM reports r WHERE r.target_type = 'listing' AND r.target_id = l.id::text AND r.status = 'open'),
		       l.moderation_reason, l.created_at
		FROM listings l JOIN users u ON u.id = l.seller_id`+w+
		fmt.Sprintf(" ORDER BY l.created_at DESC, l.id LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []ListingRow{}
	for rows.Next() {
		var x ListingRow
		var sku, state, mod sql.NullString
		var ends sql.NullTime
		if err := rows.Scan(&x.ID, &x.Title, &x.Seller, &x.SellerID, &sku, &x.Category, &x.Game, &x.Format, &x.Status, &x.PriceCents,
			&x.Quantity, &x.BidCount, &state, &ends, &x.Offers, &x.Photo, &x.OpenReports, &mod, &x.CreatedAt); err != nil {
			return nil, 0, err
		}
		if sku.Valid {
			x.SKU = &sku.String
		}
		if state.Valid {
			x.Auction = &state.String
		}
		if ends.Valid {
			x.EndsAt = &ends.Time
		}
		if mod.Valid {
			x.Moderation = &mod.String
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

// ModerateListing takes a listing down ("remove") or puts it back
// ("restore"). A live auction that's taken down is ended; bidders and the
// seller are told. Auctions can't be restored (they'd need relisting).
func (m *Moderation) ModerateListing(ctx context.Context, a Actor, id, action, reason string) error {
	reason, err := needReason(reason)
	if err != nil {
		return err
	}
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var seller, title, status, format string
	var prev, state sql.NullString
	var qty int
	err = tx.QueryRowContext(ctx, `SELECT seller_id, title, status, format, blocked_prev_status, auction_state, quantity
		FROM listings WHERE id::text = $1 FOR UPDATE`, id).Scan(&seller, &title, &status, &format, &prev, &state, &qty)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if seller == a.ID {
		return ErrSelf
	}
	switch action {
	case "remove":
		if status == "blocked" {
			return &FieldError{"action", "this listing is already taken down"}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE listings SET blocked_prev_status = status, status = 'blocked',
			auction_state = CASE WHEN format = 'auction' AND auction_state = 'live' THEN 'ended_early' ELSE auction_state END,
			ended_at = CASE WHEN format = 'auction' AND auction_state = 'live' THEN now() ELSE ended_at END,
			moderated_by = $2, moderated_at = now(), moderation_reason = $3, version = version + 1, updated_at = now()
			WHERE id::text = $1`, id, a.ID, reason); err != nil {
			return err
		}
		// Close open offers and tell bidders the auction is off.
		if _, err := tx.ExecContext(ctx, `UPDATE offers SET status = 'declined', decided_by = 'system', responded_at = now()
			WHERE listing_id::text = $1 AND status = 'pending'`, id); err != nil {
			return err
		}
		if format == "auction" && state.String == "live" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body)
				SELECT DISTINCT bidder_id, 'auction', $2, 'Your bid is cancelled. You won''t be charged.'
				FROM bids WHERE listing_id::text = $1 AND removed_at IS NULL`, id, "An auction you bid on was removed: "+title); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'system', $2, $3, '/sell')`,
			seller, "We removed your listing: "+title, "Reason: "+reason); err != nil {
			return err
		}
	case "restore":
		if status != "blocked" {
			return &FieldError{"action", "only taken-down listings can be restored"}
		}
		if format == "auction" {
			return &FieldError{"action", "auctions can't be restored; the seller can list the card again"}
		}
		back := prev.String
		if back == "" || back == "blocked" {
			back = "active"
		}
		if back == "active" && qty == 0 {
			back = "sold_out"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE listings SET status = $2, blocked_prev_status = NULL, moderated_by = $3,
			moderated_at = now(), moderation_reason = NULL, version = version + 1, updated_at = now() WHERE id::text = $1`,
			id, back, a.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'system', $2, '', '/sell')`,
			seller, "Your listing is back up: "+title); err != nil {
			return err
		}
	default:
		return &FieldError{"action", "remove or restore"}
	}
	if err := m.Audit.Log(ctx, tx, a.entry("listing."+map[string]string{"remove": "removed", "restore": "restored"}[action],
		"listing", id, reason, map[string]any{"title": title, "seller_id": seller, "from": status})); err != nil {
		return err
	}
	return tx.Commit()
}
