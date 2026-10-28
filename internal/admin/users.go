package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
)

var (
	ErrNotFound     = errors.New("member not found")
	ErrReason       = errors.New("give a reason; it's saved in the audit log")
	ErrSelf         = errors.New("you can't do that to your own account")
	ErrProtected    = errors.New("only a Super Admin can change another staff member's account")
	ErrLastSuper    = errors.New("there must always be at least one Super Admin")
	ErrOwner        = errors.New("this owner account is set in ADMIN_EMAILS and stays a Super Admin")
	ErrBadStatus    = errors.New("status must be active, frozen, suspended or banned")
	ErrSuspendUntil = errors.New("choose when the suspension ends")
	ErrTaken        = errors.New("that email or username is already in use")
)

// FieldError is a problem with one submitted field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

// Users is staff management of member accounts.
type Users struct {
	DB          *sql.DB
	Audit       *Audit
	OwnerEmails []string // ADMIN_EMAILS
}

// Actor is the staff member doing something, for permission checks and the log.
type Actor struct {
	ID, Email, Role, IP string
}

func (a Actor) entry(action, targetType, targetID, reason string, details map[string]any) Entry {
	return Entry{ActorID: a.ID, ActorEmail: a.Email, ActorRole: a.Role, Action: action,
		TargetType: targetType, TargetID: targetID, Reason: reason, Details: details, IP: a.IP}
}

// MemberRow is one line of the member search.
type MemberRow struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Username  string     `json:"username"`
	Status    string     `json:"status"`
	StaffRole *string    `json:"staff_role"`
	CreatedAt time.Time  `json:"created_at"`
	IsSeller  bool       `json:"is_seller"`
	Tier      *int       `json:"tier"`
	Review    *string    `json:"review_status"`
	Listings  int        `json:"active_listings"`
	LastLogin *time.Time `json:"last_login"`
}

// MemberQuery filters the member search.
type MemberQuery struct {
	Q       string // email, username or ID
	Status  string
	Staff   bool
	Sellers bool
	Limit   int
	Offset  int
}

const memberSelect = `
	SELECT u.id, u.email, u.username, u.status, u.staff_role, u.created_at,
	       sp.user_id IS NOT NULL, sp.tier, sp.review_status,
	       (SELECT count(*) FROM listings l WHERE l.seller_id = u.id AND l.status = 'active'),
	       (SELECT max(created_at) FROM login_events e WHERE e.user_id = u.id AND e.success)
	FROM users u LEFT JOIN seller_profiles sp ON sp.user_id = u.id`

func scanMember(row interface{ Scan(...any) error }) (MemberRow, error) {
	var m MemberRow
	var staff, review sql.NullString
	var tier sql.NullInt64
	var last sql.NullTime
	err := row.Scan(&m.ID, &m.Email, &m.Username, &m.Status, &staff, &m.CreatedAt, &m.IsSeller, &tier, &review, &m.Listings, &last)
	if staff.Valid {
		m.StaffRole = &staff.String
	}
	if review.Valid {
		m.Review = &review.String
	}
	if tier.Valid {
		t := int(tier.Int64)
		m.Tier = &t
	}
	if last.Valid {
		m.LastLogin = &last.Time
	}
	return m, err
}

// Search finds members, newest first.
func (u *Users) Search(ctx context.Context, q MemberQuery) ([]MemberRow, int, error) {
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
		add("(u.id::text = ? OR lower(u.email) LIKE '%' || lower(?) || '%' OR lower(u.username) LIKE '%' || lower(?) || '%')", s)
	}
	if q.Status != "" {
		add("u.status = ?", q.Status)
	}
	if q.Staff {
		where = append(where, "u.staff_role IS NOT NULL")
	}
	if q.Sellers {
		where = append(where, "sp.user_id IS NOT NULL")
	}
	w := " WHERE " + strings.Join(where, " AND ")
	var total int
	if err := u.DB.QueryRowContext(ctx, `SELECT count(*) FROM users u LEFT JOIN seller_profiles sp ON sp.user_id = u.id`+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, q.Limit, q.Offset)
	rows, err := u.DB.QueryContext(ctx, memberSelect+w+fmt.Sprintf(" ORDER BY u.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []MemberRow{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// Member is everything staff see on a member's page.
type Member struct {
	MemberRow
	EmailVerified  bool           `json:"email_verified"`
	SuspendedUntil *time.Time     `json:"suspended_until"`
	StatusReason   *string        `json:"status_reason"`
	StatusChanged  *time.Time     `json:"status_changed_at"`
	Seller         *SellerInfo    `json:"seller"`
	Counts         map[string]int `json:"counts"`
	Logins         []LoginEvent   `json:"logins"`
	Notes          []Note         `json:"notes"`
	History        []Entry        `json:"history"`
	MFAEnrolled    bool           `json:"mfa_enrolled"`
	Owner          bool           `json:"owner"`
}

type SellerInfo struct {
	StripeAccount    string     `json:"stripe_account_id"`
	ChargesEnabled   bool       `json:"charges_enabled"`
	DetailsSubmitted bool       `json:"details_submitted"`
	Tier             int        `json:"tier"`
	ReviewStatus     string     `json:"review_status"`
	ReviewNote       *string    `json:"review_note"`
	ReviewedAt       *time.Time `json:"reviewed_at"`
	BundleDiscount   *string    `json:"bundle_discount_type"`
	Since            time.Time  `json:"since"`
}

type LoginEvent struct {
	At      time.Time `json:"at"`
	IP      string    `json:"ip"`
	Agent   string    `json:"user_agent"`
	Success bool      `json:"success"`
	Reason  string    `json:"reason"`
}

type Note struct {
	ID     int64     `json:"id"`
	Author string    `json:"author"`
	Body   string    `json:"body"`
	At     time.Time `json:"at"`
}

func (u *Users) isOwner(email string) bool {
	for _, o := range u.OwnerEmails {
		if strings.EqualFold(strings.TrimSpace(o), email) {
			return true
		}
	}
	return false
}

// Get loads a member's page.
func (u *Users) Get(ctx context.Context, id string) (Member, error) {
	row, err := scanMember(u.DB.QueryRowContext(ctx, memberSelect+` WHERE u.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, err
	}
	m := Member{MemberRow: row, Counts: map[string]int{}, Logins: []LoginEvent{}, Notes: []Note{}}
	m.Owner = u.isOwner(m.Email)
	var until, changed sql.NullTime
	var reason sql.NullString
	if err := u.DB.QueryRowContext(ctx, `SELECT email_verified_at IS NOT NULL, suspended_until, status_reason, status_changed_at,
		EXISTS (SELECT 1 FROM staff_mfa WHERE user_id = $1 AND confirmed_at IS NOT NULL)
		FROM users WHERE id = $1`, id).Scan(&m.EmailVerified, &until, &reason, &changed, &m.MFAEnrolled); err != nil {
		return m, err
	}
	if until.Valid {
		m.SuspendedUntil = &until.Time
	}
	if reason.Valid {
		m.StatusReason = &reason.String
	}
	if changed.Valid {
		m.StatusChanged = &changed.Time
	}
	if m.IsSeller {
		var si SellerInfo
		var note, bundle sql.NullString
		var reviewed sql.NullTime
		if err := u.DB.QueryRowContext(ctx, `SELECT stripe_account_id, charges_enabled, details_submitted, tier, review_status,
			review_note, reviewed_at, bundle_discount_type, created_at FROM seller_profiles WHERE user_id = $1`, id).Scan(
			&si.StripeAccount, &si.ChargesEnabled, &si.DetailsSubmitted, &si.Tier, &si.ReviewStatus, &note, &reviewed, &bundle, &si.Since); err != nil {
			return m, err
		}
		if note.Valid {
			si.ReviewNote = &note.String
		}
		if reviewed.Valid {
			si.ReviewedAt = &reviewed.Time
		}
		if bundle.Valid {
			si.BundleDiscount = &bundle.String
		}
		m.Seller = &si
	}
	counts := map[string]string{
		"listings_total":   `SELECT count(*) FROM listings WHERE seller_id = $1`,
		"auctions_live":    `SELECT count(*) FROM listings WHERE seller_id = $1 AND auction_state = 'live' AND status = 'active'`,
		"bids":             `SELECT count(*) FROM bids WHERE bidder_id = $1`,
		"offers_made":      `SELECT count(*) FROM offers WHERE buyer_id = $1 AND from_role = 'buyer'`,
		"won_or_accepted":  `SELECT count(*) FROM holds WHERE buyer_id = $1`,
		"sold_holds":       `SELECT count(*) FROM holds WHERE seller_id = $1`,
		"threads":          `SELECT count(*) FROM message_threads WHERE buyer_id = $1 OR seller_id = $1`,
		"blocked_messages": `SELECT count(*) FROM messages WHERE sender_id = $1 AND blocked IS NOT NULL`,
		"watching":         `SELECT count(*) FROM watchlist WHERE user_id = $1`,
		"sessions":         `SELECT count(*) FROM sessions WHERE user_id = $1 AND expires_at > now()`,
	}
	for k, q := range counts {
		var n int
		if err := u.DB.QueryRowContext(ctx, q, id).Scan(&n); err != nil {
			return m, err
		}
		m.Counts[k] = n
	}
	rows, err := u.DB.QueryContext(ctx, `SELECT created_at, coalesce(ip, ''), coalesce(user_agent, ''), success, coalesce(reason, '')
		FROM login_events WHERE user_id = $1 ORDER BY id DESC LIMIT 25`, id)
	if err != nil {
		return m, err
	}
	for rows.Next() {
		var e LoginEvent
		if err := rows.Scan(&e.At, &e.IP, &e.Agent, &e.Success, &e.Reason); err != nil {
			rows.Close()
			return m, err
		}
		m.Logins = append(m.Logins, e)
	}
	rows.Close()
	nrows, err := u.DB.QueryContext(ctx, `SELECT id, author_email, body, created_at FROM staff_notes WHERE user_id = $1 ORDER BY id DESC LIMIT 100`, id)
	if err != nil {
		return m, err
	}
	for nrows.Next() {
		var n Note
		if err := nrows.Scan(&n.ID, &n.Author, &n.Body, &n.At); err != nil {
			nrows.Close()
			return m, err
		}
		m.Notes = append(m.Notes, n)
	}
	nrows.Close()
	m.History, err = u.Audit.List(ctx, Filter{TargetType: "user", TargetID: id, Limit: 25})
	return m, err
}

func needReason(r string) (string, error) {
	r = strings.TrimSpace(r)
	if utf8.RuneCountInString(r) < 3 {
		return "", ErrReason
	}
	if utf8.RuneCountInString(r) > 1000 {
		return "", &FieldError{"reason", "keep the reason under 1,000 characters"}
	}
	return r, nil
}

type target struct {
	email, username, status string
	staff                   sql.NullString
}

func (u *Users) lockTarget(ctx context.Context, tx *sql.Tx, id string) (target, error) {
	var t target
	err := tx.QueryRowContext(ctx, `SELECT email, username, status, staff_role FROM users WHERE id = $1 FOR UPDATE`, id).
		Scan(&t.email, &t.username, &t.status, &t.staff)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// guard: nobody acts on their own account through these tools, and only a
// Super Admin acts on another staff member.
func guard(a Actor, id string, t target) error {
	if a.ID == id {
		return ErrSelf
	}
	if t.staff.Valid && a.Role != SuperAdmin {
		return ErrProtected
	}
	return nil
}

// ProfileChange edits a member's details. Nil fields stay the same.
type ProfileChange struct {
	Email         *string `json:"email"`
	Username      *string `json:"username"`
	EmailVerified *bool   `json:"email_verified"`
	Reason        string  `json:"reason"`
}

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_]{3,30}$`)

// UpdateProfile changes email, username or verification.
func (u *Users) UpdateProfile(ctx context.Context, a Actor, id string, p ProfileChange) error {
	reason, err := needReason(p.Reason)
	if err != nil {
		return err
	}
	tx, err := u.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := u.lockTarget(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := guard(a, id, t); err != nil {
		return err
	}
	before, after := map[string]any{}, map[string]any{}
	if p.Email != nil {
		e := strings.TrimSpace(*p.Email)
		if addr, err := mail.ParseAddress(e); err != nil || addr.Address != e {
			return &FieldError{"email", "enter a valid email address"}
		}
		if u.isOwner(t.email) {
			return ErrOwner
		}
		if !strings.EqualFold(e, t.email) {
			before["email"], after["email"] = t.email, e
			if _, err := tx.ExecContext(ctx, `UPDATE users SET email = $2, email_verified_at = NULL, updated_at = now() WHERE id = $1`, id, e); err != nil {
				return mapUnique(err)
			}
		}
	}
	if p.Username != nil {
		n := strings.TrimSpace(*p.Username)
		if !usernameRE.MatchString(n) {
			return &FieldError{"username", "3-30 characters: letters, numbers, underscore"}
		}
		if n != t.username {
			before["username"], after["username"] = t.username, n
			if _, err := tx.ExecContext(ctx, `UPDATE users SET username = $2, updated_at = now() WHERE id = $1`, id, n); err != nil {
				return mapUnique(err)
			}
		}
	}
	if p.EmailVerified != nil {
		after["email_verified"] = *p.EmailVerified
		if _, err := tx.ExecContext(ctx, `UPDATE users SET email_verified_at = CASE WHEN $2 THEN coalesce(email_verified_at, now()) END,
			updated_at = now() WHERE id = $1`, id, *p.EmailVerified); err != nil {
			return err
		}
	}
	if len(after) == 0 {
		return nil
	}
	if err := u.Audit.Log(ctx, tx, a.entry("user.profile_edited", "user", id, reason, map[string]any{"before": before, "after": after})); err != nil {
		return err
	}
	return tx.Commit()
}

func mapUnique(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return ErrTaken
	}
	return err
}

// StatusChange freezes, suspends, bans or reinstates a member.
type StatusChange struct {
	Status string     `json:"status"`
	Until  *time.Time `json:"until"` // required for suspended
	Reason string     `json:"reason"`
}

// SetStatus changes account status. Suspending or banning signs the member
// out everywhere. Banning needs the ban permission.
func (u *Users) SetStatus(ctx context.Context, a Actor, id string, c StatusChange) error {
	reason, err := needReason(c.Reason)
	if err != nil {
		return err
	}
	switch c.Status {
	case "active", "frozen", "banned":
		c.Until = nil
	case "suspended":
		if c.Until == nil || c.Until.Before(time.Now().Add(time.Minute)) {
			return ErrSuspendUntil
		}
	default:
		return ErrBadStatus
	}
	if c.Status == "banned" && !Can(a.Role, UsersBan) {
		return ErrForbidden
	}
	tx, err := u.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := u.lockTarget(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := guard(a, id, t); err != nil {
		return err
	}
	if c.Status != "active" && u.isOwner(t.email) {
		return ErrOwner
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET status = $2, suspended_until = $3, status_reason = $4,
		status_changed_at = now(), updated_at = now() WHERE id = $1`, id, c.Status, c.Until, reason); err != nil {
		return err
	}
	if c.Status == "suspended" || c.Status == "banned" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, id); err != nil {
			return err
		}
	}
	details := map[string]any{"from": t.status, "to": c.Status}
	if c.Until != nil {
		details["until"] = c.Until.UTC().Format(time.RFC3339)
	}
	action := map[string]string{"active": "user.reinstated", "frozen": "user.frozen", "suspended": "user.suspended", "banned": "user.banned"}[c.Status]
	if err := u.Audit.Log(ctx, tx, a.entry(action, "user", id, reason, details)); err != nil {
		return err
	}
	msg := map[string]string{
		"active":    "Your account is active again.",
		"frozen":    "Your account is frozen while we review it. You can browse but not buy, sell, bid or message.",
		"suspended": "", "banned": "",
	}[c.Status]
	if msg != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body) VALUES ($1, 'system', $2, '')`, id, msg); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var ErrForbidden = errors.New("your staff role can't do that")

// SignOutEverywhere ends all of a member's sessions.
func (u *Users) SignOutEverywhere(ctx context.Context, a Actor, id, reason string) error {
	reason, err := needReason(reason)
	if err != nil {
		return err
	}
	tx, err := u.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := u.lockTarget(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := guard(a, id, t); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if err := u.Audit.Log(ctx, tx, a.entry("user.signed_out", "user", id, reason, map[string]any{"sessions": n})); err != nil {
		return err
	}
	return tx.Commit()
}

// AddNote adds an internal note to a member's page.
func (u *Users) AddNote(ctx context.Context, a Actor, id, body string) (Note, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > 4000 {
		return Note{}, &FieldError{"body", "write a note of up to 4,000 characters"}
	}
	tx, err := u.DB.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, err
	}
	defer tx.Rollback()
	n := Note{Author: a.Email, Body: body}
	err = tx.QueryRowContext(ctx, `INSERT INTO staff_notes (user_id, author_id, author_email, body) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`, id, a.ID, a.Email, body).Scan(&n.ID, &n.At)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23503" {
		return Note{}, ErrNotFound
	}
	if err != nil {
		return Note{}, err
	}
	if err := u.Audit.Log(ctx, tx, a.entry("user.note_added", "user", id, "", map[string]any{"note_id": n.ID})); err != nil {
		return Note{}, err
	}
	return n, tx.Commit()
}

// ResetMFA removes a staff member's authenticator so they set it up again.
func (u *Users) ResetMFA(ctx context.Context, a Actor, id, reason string) error {
	reason, err := needReason(reason)
	if err != nil {
		return err
	}
	tx, err := u.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := u.lockTarget(ctx, tx, id); err != nil {
		return err
	}
	if a.ID == id {
		return ErrSelf
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM staff_mfa WHERE user_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET mfa_verified_at = NULL WHERE user_id = $1`, id); err != nil {
		return err
	}
	if err := u.Audit.Log(ctx, tx, a.entry("staff.mfa_reset", "user", id, reason, nil)); err != nil {
		return err
	}
	return tx.Commit()
}

// SetStaffRole adds, changes or removes ("") a member's staff role.
func (u *Users) SetStaffRole(ctx context.Context, a Actor, id, role, reason string) error {
	reason, err := needReason(reason)
	if err != nil {
		return err
	}
	if role != "" && !ValidRole(role) {
		return &FieldError{"role", "choose Super Admin, Moderator, Support Agent or Financial Auditor"}
	}
	tx, err := u.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7240002)`); err != nil {
		return err
	}
	t, err := u.lockTarget(ctx, tx, id)
	if err != nil {
		return err
	}
	if a.ID == id {
		return ErrSelf
	}
	if u.isOwner(t.email) && role != SuperAdmin {
		return ErrOwner
	}
	if t.status != "active" && role != "" {
		return &FieldError{"role", "reinstate the account before giving it a staff role"}
	}
	if t.staff.String == SuperAdmin && role != SuperAdmin {
		var supers int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE staff_role = 'super_admin' AND status = 'active'`).Scan(&supers); err != nil {
			return err
		}
		if supers <= 1 {
			return ErrLastSuper
		}
	}
	var newRole any
	if role != "" {
		newRole = role
	}
	legacy := "user"
	if role == SuperAdmin {
		legacy = "admin"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET staff_role = $2, role = $3, updated_at = now() WHERE id = $1`, id, newRole, legacy); err != nil {
		return err
	}
	// A changed role takes effect at the next authenticator check.
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET mfa_verified_at = NULL WHERE user_id = $1`, id); err != nil {
		return err
	}
	action := "staff.role_changed"
	if role == "" {
		action = "staff.removed"
	} else if !t.staff.Valid {
		action = "staff.added"
	}
	if err := u.Audit.Log(ctx, tx, a.entry(action, "user", id, reason, map[string]any{"from": t.staff.String, "to": role})); err != nil {
		return err
	}
	return tx.Commit()
}

// IDByEmail finds a member by email (for adding staff).
func (u *Users) IDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := u.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, strings.TrimSpace(email)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// SellerRow is one line of the seller review queue.
type SellerRow struct {
	UserID           string     `json:"user_id"`
	Username         string     `json:"username"`
	Email            string     `json:"email"`
	Status           string     `json:"status"`
	StripeAccount    string     `json:"stripe_account_id"`
	ChargesEnabled   bool       `json:"charges_enabled"`
	DetailsSubmitted bool       `json:"details_submitted"`
	Tier             int        `json:"tier"`
	ReviewStatus     string     `json:"review_status"`
	ReviewNote       *string    `json:"review_note"`
	ReviewedAt       *time.Time `json:"reviewed_at"`
	Listings         int        `json:"active_listings"`
	Since            time.Time  `json:"since"`
}

// Sellers lists sellers by review status (oldest waiting first for pending).
func (u *Users) Sellers(ctx context.Context, review string, limit int) ([]SellerRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	order := "sp.created_at DESC"
	if review == "pending" {
		order = "sp.charges_enabled DESC, sp.created_at ASC"
	}
	rows, err := u.DB.QueryContext(ctx, `
		SELECT u.id, u.username, u.email, u.status, sp.stripe_account_id, sp.charges_enabled, sp.details_submitted,
		       sp.tier, sp.review_status, sp.review_note, sp.reviewed_at,
		       (SELECT count(*) FROM listings l WHERE l.seller_id = u.id AND l.status = 'active'), sp.created_at
		FROM seller_profiles sp JOIN users u ON u.id = sp.user_id
		WHERE ($1 = '' OR sp.review_status = $1)
		ORDER BY `+order+` LIMIT $2`, review, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SellerRow{}
	for rows.Next() {
		var s SellerRow
		var note sql.NullString
		var at sql.NullTime
		if err := rows.Scan(&s.UserID, &s.Username, &s.Email, &s.Status, &s.StripeAccount, &s.ChargesEnabled, &s.DetailsSubmitted,
			&s.Tier, &s.ReviewStatus, &note, &at, &s.Listings, &s.Since); err != nil {
			return nil, err
		}
		if note.Valid {
			s.ReviewNote = &note.String
		}
		if at.Valid {
			s.ReviewedAt = &at.Time
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReviewSeller approves, rejects or resets a seller. Rejected sellers'
// listings are hidden until they're approved.
func (u *Users) ReviewSeller(ctx context.Context, a Actor, id, decision, note string) error {
	status := map[string]string{"approve": "approved", "reject": "rejected", "pending": "pending"}[decision]
	if status == "" {
		return &FieldError{"decision", "approve, reject or pending"}
	}
	note = strings.TrimSpace(note)
	if status == "rejected" {
		var err error
		if note, err = needReason(note); err != nil {
			return err
		}
	}
	tx, err := u.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var prev string
	err = tx.QueryRowContext(ctx, `SELECT review_status FROM seller_profiles WHERE user_id = $1 FOR UPDATE`, id).Scan(&prev)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if a.ID == id {
		return ErrSelf
	}
	if _, err := tx.ExecContext(ctx, `UPDATE seller_profiles SET review_status = $2, review_note = NULLIF($3, ''),
		reviewed_by = $4, reviewed_at = now(), updated_at = now() WHERE user_id = $1`, id, status, note, a.ID); err != nil {
		return err
	}
	if err := u.Audit.Log(ctx, tx, a.entry("seller."+status, "user", id, note, map[string]any{"from": prev})); err != nil {
		return err
	}
	msg := map[string]string{
		"approved": "Your seller account is approved.",
		"rejected": "Your seller account wasn't approved, so your listings are hidden. Contact us if you think this is a mistake.",
	}[status]
	if msg != "" && prev != status {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'system', $2, '', '/sell')`, id, msg); err != nil {
			return err
		}
	}
	return tx.Commit()
}
