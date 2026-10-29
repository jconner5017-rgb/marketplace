package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

// Standing covers warnings, seller tier changes by staff, and appeals.
type Standing struct {
	DB    *sql.DB
	Audit *Audit
	Users *Users
	Mod   *Moderation
}

var (
	ErrAppealUsed      = errors.New("you've already appealed this decision; each decision can be appealed once")
	ErrNothingToAppeal = errors.New("there's no decision here to appeal")
)

// Warn gives a member a formal warning they can see and appeal.
func (st *Standing) Warn(ctx context.Context, a Actor, userID, reason string) (string, error) {
	reason, err := needReason(reason)
	if err != nil {
		return "", err
	}
	if userID == a.ID {
		return "", ErrSelf
	}
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO member_warnings (user_id, reason, created_by)
		SELECT id, $2, $3 FROM users WHERE id::text = $1 RETURNING id`, userID, reason, a.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link)
		VALUES ($1, 'system', 'A warning was added to your account', $2, '/account/standing')`, userID, reason); err != nil {
		return "", err
	}
	if err := st.Audit.Log(ctx, tx, a.entry("user.warned", "user", userID, reason, map[string]any{"warning_id": id})); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// SetTier moves a seller up or down. lock keeps the nightly job from
// changing it back until staff unlock it.
func (st *Standing) SetTier(ctx context.Context, a Actor, userID string, tier int, lock bool, reason string) error {
	reason, err := needReason(reason)
	if err != nil {
		return err
	}
	if userID == a.ID {
		return ErrSelf
	}
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	from, err := sellers.ApplyTier(ctx, tx, userID, tier, reason, a.ID, map[string]any{"by": "staff"})
	if errors.Is(err, sellers.ErrNotSeller) {
		return &FieldError{"tier", "this member isn't a seller"}
	}
	if err != nil {
		return &FieldError{"tier", err.Error()}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE seller_profiles SET tier_locked = $2 WHERE user_id = $1`, userID, lock); err != nil {
		return err
	}
	action := "seller.tier_changed"
	if tier < from {
		action = "seller.demoted"
	}
	if err := st.Audit.Log(ctx, tx, a.entry(action, "user", userID, reason,
		map[string]any{"from": from, "to": tier, "locked": lock})); err != nil {
		return err
	}
	return tx.Commit()
}

// Warning is one formal warning on an account.
type Warning struct {
	ID          string     `json:"id"`
	Reason      string     `json:"reason"`
	CreatedAt   time.Time  `json:"created_at"`
	RescindedAt *time.Time `json:"rescinded_at"`
	By          *string    `json:"by,omitempty"`
}

// Appeal is a member's one appeal against a decision.
type Appeal struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Username     string     `json:"username"`
	Email        string     `json:"email,omitempty"`
	About        string     `json:"about"`
	Ref          string     `json:"ref"`
	Message      string     `json:"message"`
	Status       string     `json:"status"`
	DecisionNote *string    `json:"decision_note"`
	DecidedBy    *string    `json:"decided_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	DecidedAt    *time.Time `json:"decided_at"`
	Decision     string     `json:"decision"` // what was appealed, in words
}

// StandingView is what a member sees about their own account.
type StandingView struct {
	Status     string     `json:"status"`
	Reason     *string    `json:"status_reason"`
	Until      *time.Time `json:"suspended_until"`
	Warnings   []Warning  `json:"warnings"`
	Appeals    []Appeal   `json:"appeals"`
	Tier       *int       `json:"tier"`
	TierReason *string    `json:"tier_reason"`
	TierRef    *string    `json:"tier_change_id"`
	Blocked    []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Reason string `json:"reason"`
	} `json:"blocked_listings"`
}

func (st *Standing) Warnings(ctx context.Context, userID string, staffView bool) ([]Warning, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT w.id, w.reason, w.created_at, w.rescinded_at, u.email
		FROM member_warnings w LEFT JOIN users u ON u.id = w.created_by WHERE w.user_id::text = $1 ORDER BY w.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Warning{}
	for rows.Next() {
		var w Warning
		var by sql.NullString
		if err := rows.Scan(&w.ID, &w.Reason, &w.CreatedAt, &w.RescindedAt, &by); err != nil {
			return nil, err
		}
		if staffView && by.Valid {
			w.By = &by.String
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ForMember is the member's own standing page.
func (st *Standing) ForMember(ctx context.Context, userID string) (StandingView, error) {
	var v StandingView
	if err := st.DB.QueryRowContext(ctx, `SELECT status, status_reason, suspended_until FROM users WHERE id = $1`, userID).
		Scan(&v.Status, &v.Reason, &v.Until); err != nil {
		return v, err
	}
	var err error
	if v.Warnings, err = st.Warnings(ctx, userID, false); err != nil {
		return v, err
	}
	if v.Appeals, err = st.appeals(ctx, `a.user_id = $1`, userID); err != nil {
		return v, err
	}
	var tier sql.NullInt64
	var reason, ref sql.NullString
	err = st.DB.QueryRowContext(ctx, `SELECT sp.tier, tc.reason, tc.id::text FROM seller_profiles sp
		LEFT JOIN LATERAL (SELECT id, reason FROM tier_changes WHERE user_id = sp.user_id AND to_tier < from_tier
		                   ORDER BY id DESC LIMIT 1) tc ON true
		WHERE sp.user_id = $1`, userID).Scan(&tier, &reason, &ref)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, err
	}
	if tier.Valid {
		t := int(tier.Int64)
		v.Tier = &t
		if reason.Valid {
			v.TierReason, v.TierRef = &reason.String, &ref.String
		}
	}
	rows, err := st.DB.QueryContext(ctx, `SELECT id, title, coalesce(moderation_reason, '') FROM listings
		WHERE seller_id = $1 AND status = 'blocked' ORDER BY moderated_at DESC LIMIT 50`, userID)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var b struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Reason string `json:"reason"`
		}
		if err := rows.Scan(&b.ID, &b.Title, &b.Reason); err != nil {
			return v, err
		}
		v.Blocked = append(v.Blocked, b)
	}
	return v, rows.Err()
}

// SubmitAppeal records a member's appeal. about/ref name the decision:
// warning + warning id, demotion + tier change id, listing + listing id,
// or freeze/suspension/ban (ref filled in here from the account).
func (st *Standing) SubmitAppeal(ctx context.Context, userID, about, ref, message string) (Appeal, error) {
	message = strings.TrimSpace(message)
	if n := utf8.RuneCountInString(message); n < 20 || n > 3000 {
		return Appeal{}, &FieldError{"message", "explain why the decision should change (20 to 3,000 characters)"}
	}
	var ok bool
	var err error
	switch about {
	case "warning":
		err = st.DB.QueryRowContext(ctx, `SELECT true FROM member_warnings WHERE id::text = $1 AND user_id = $2 AND rescinded_at IS NULL`,
			ref, userID).Scan(&ok)
	case "demotion":
		err = st.DB.QueryRowContext(ctx, `SELECT true FROM tier_changes WHERE id::text = $1 AND user_id = $2 AND to_tier < from_tier`,
			ref, userID).Scan(&ok)
	case "listing":
		err = st.DB.QueryRowContext(ctx, `SELECT true FROM listings WHERE id::text = $1 AND seller_id = $2 AND status = 'blocked'`,
			ref, userID).Scan(&ok)
	case "freeze", "suspension", "ban":
		want := map[string]string{"freeze": "frozen", "suspension": "suspended", "ban": "banned"}[about]
		var changed sql.NullTime
		err = st.DB.QueryRowContext(ctx, `SELECT status = $2, status_changed_at FROM users WHERE id = $1`, userID, want).Scan(&ok, &changed)
		ref = "account"
		if changed.Valid {
			ref = changed.Time.UTC().Format(time.RFC3339)
		}
	default:
		return Appeal{}, &FieldError{"about", "choose what you're appealing"}
	}
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !ok) {
		return Appeal{}, ErrNothingToAppeal
	}
	if err != nil {
		return Appeal{}, err
	}
	var id string
	err = st.DB.QueryRowContext(ctx, `INSERT INTO appeals (user_id, about, ref, message) VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, about, ref) DO NOTHING RETURNING id`, userID, about, ref, message).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Appeal{}, ErrAppealUsed
	}
	if err != nil {
		return Appeal{}, err
	}
	as, err := st.appeals(ctx, `a.id = $1`, id)
	if err != nil || len(as) == 0 {
		return Appeal{}, err
	}
	return as[0], nil
}

func (st *Standing) appeals(ctx context.Context, where string, args ...any) ([]Appeal, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT a.id, a.user_id, u.username, u.email, a.about, a.ref, a.message, a.status,
		a.decision_note, d.email, a.created_at, a.decided_at,
		CASE a.about
			WHEN 'warning' THEN (SELECT reason FROM member_warnings WHERE id::text = a.ref)
			WHEN 'demotion' THEN (SELECT 'Tier ' || from_tier || ' → ' || to_tier || ': ' || reason FROM tier_changes WHERE id::text = a.ref)
			WHEN 'listing' THEN (SELECT title || ': ' || coalesce(moderation_reason, '') FROM listings WHERE id::text = a.ref)
			ELSE initcap(a.about) || ': ' || coalesce(u.status_reason, '')
		END
		FROM appeals a JOIN users u ON u.id = a.user_id LEFT JOIN users d ON d.id = a.decided_by
		WHERE `+where+` ORDER BY a.created_at DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Appeal{}
	for rows.Next() {
		var x Appeal
		var decision sql.NullString
		if err := rows.Scan(&x.ID, &x.UserID, &x.Username, &x.Email, &x.About, &x.Ref, &x.Message, &x.Status,
			&x.DecisionNote, &x.DecidedBy, &x.CreatedAt, &x.DecidedAt, &decision); err != nil {
			return nil, err
		}
		x.Decision = decision.String
		out = append(out, x)
	}
	return out, rows.Err()
}

// Appeals lists appeals for staff; status "" = all.
func (st *Standing) Appeals(ctx context.Context, status string) ([]Appeal, error) {
	if status == "" {
		return st.appeals(ctx, `true`)
	}
	return st.appeals(ctx, `a.status = $1`, status)
}

// Decide upholds or overturns an appeal. Overturning undoes the decision:
// the account is reinstated, the warning rescinded, the tier restored or the
// listing put back.
func (st *Standing) Decide(ctx context.Context, a Actor, id, decision, note string) error {
	note, err := needReason(note)
	if err != nil {
		return err
	}
	if decision != "upheld" && decision != "overturned" {
		return &FieldError{"decision", "uphold or overturn"}
	}
	as, err := st.appeals(ctx, `a.id::text = $1`, id)
	if err != nil {
		return err
	}
	if len(as) == 0 {
		return ErrNotFound
	}
	ap := as[0]
	if ap.Status != "open" {
		return &FieldError{"decision", "this appeal was already decided"}
	}
	if ap.UserID == a.ID {
		return ErrSelf
	}
	if decision == "overturned" {
		switch ap.About {
		case "freeze", "suspension", "ban":
			if ap.About == "ban" && !Can(a.Role, UsersBan) {
				return ErrForbidden
			}
			if err := st.Users.SetStatus(ctx, a, ap.UserID, StatusChange{Status: "active", Reason: "Appeal upheld: " + note}); err != nil {
				return err
			}
			if ap.About == "ban" {
				if _, err := st.DB.ExecContext(ctx, `DELETE FROM blocked_identifiers WHERE user_id = $1`, ap.UserID); err != nil {
					return err
				}
			}
		case "listing":
			if err := st.Mod.ModerateListing(ctx, a, ap.Ref, "restore", "Appeal upheld: "+note); err != nil {
				return err
			}
		case "demotion":
			var from int
			if err := st.DB.QueryRowContext(ctx, `SELECT from_tier FROM tier_changes WHERE id::text = $1`, ap.Ref).Scan(&from); err != nil {
				return err
			}
			if err := st.SetTier(ctx, a, ap.UserID, from, false, "Appeal upheld: "+note); err != nil {
				return err
			}
		}
	}
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if decision == "overturned" && ap.About == "warning" {
		if _, err := tx.ExecContext(ctx, `UPDATE member_warnings SET rescinded_at = now() WHERE id::text = $1`, ap.Ref); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE appeals SET status = $2, decision_note = $3, decided_by = $4, decided_at = now()
		WHERE id = $1`, ap.ID, decision, note, a.ID); err != nil {
		return err
	}
	title := "Your appeal was reviewed: the decision stands"
	if decision == "overturned" {
		title = "Your appeal was accepted"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'system', $2, $3, '/account/standing')`,
		ap.UserID, title, note); err != nil {
		return err
	}
	if err := st.Audit.Log(ctx, tx, a.entry("appeal."+decision, "user", ap.UserID, note,
		map[string]any{"appeal_id": ap.ID, "about": ap.About, "ref": ap.Ref})); err != nil {
		return err
	}
	return tx.Commit()
}

func (x Appeal) String() string { return fmt.Sprintf("%s appeal by %s", x.About, x.Username) }
