// Package blocklist keeps hashed identifiers of banned members (email,
// sign-in addresses, ship-to addresses) so they can't simply re-register.
package blocklist

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
)

// ErrBlocked is returned when a sign-up uses a banned member's email.
var ErrBlocked = errors.New("this email can't be used to open an account")

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// NormalizeEmail folds the usual tricks: case, +tags, and Gmail's dots.
func NormalizeEmail(e string) string {
	e = strings.ToLower(strings.TrimSpace(e))
	local, domain, ok := strings.Cut(e, "@")
	if !ok {
		return e
	}
	local, _, _ = strings.Cut(local, "+")
	if domain == "googlemail.com" {
		domain = "gmail.com"
	}
	if domain == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}

// NormalizeAddress keeps letters and digits of the street line plus the 5-digit ZIP.
func NormalizeAddress(line1, zip string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(line1) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	z := strings.TrimSpace(zip)
	if len(z) > 5 {
		z = z[:5]
	}
	if b.Len() == 0 || z == "" {
		return ""
	}
	return b.String() + "|" + z
}

func hash(kind, v string) string {
	h := sha256.Sum256([]byte("blocklist:" + kind + ":" + v))
	return hex.EncodeToString(h[:])
}

func normalize(kind, v string) string {
	switch kind {
	case "email":
		return NormalizeEmail(v)
	case "ip":
		return strings.TrimSpace(v)
	}
	return v
}

// Add blocks a value (already normalized for addresses). Empty values are ignored.
func Add(ctx context.Context, ex Execer, kind, value, userID string) error {
	v := normalize(kind, value)
	if v == "" {
		return nil
	}
	_, err := ex.ExecContext(ctx, `INSERT INTO blocked_identifiers (kind, value_hash, user_id) VALUES ($1, $2, $3)
		ON CONFLICT (kind, value_hash) DO NOTHING`, kind, hash(kind, v), userID)
	return err
}

// Has reports whether a value is blocked, and which banned account it came from.
func Has(ctx context.Context, ex Execer, kind, value string) (bool, string, error) {
	v := normalize(kind, value)
	if v == "" {
		return false, "", nil
	}
	var uid sql.NullString
	err := ex.QueryRowContext(ctx, `SELECT user_id::text FROM blocked_identifiers WHERE kind = $1 AND value_hash = $2`,
		kind, hash(kind, v)).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, uid.String, nil
}

// BlockMember records a banned member's email, sign-in addresses from the
// last 90 days and the ship-to addresses on their orders.
func BlockMember(ctx context.Context, tx *sql.Tx, userID string) error {
	var email string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
		return err
	}
	if err := Add(ctx, tx, "email", email, userID); err != nil {
		return err
	}
	var ips, addrs [][2]string
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT ip FROM login_events
		WHERE user_id = $1 AND success AND ip IS NOT NULL AND created_at > now() - interval '90 days'`, userID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			rows.Close()
			return err
		}
		ips = append(ips, [2]string{ip})
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT DISTINCT coalesce(ship_to->>'line1', ''), coalesce(ship_to->>'zip', '')
		FROM orders WHERE buyer_id = $1`, userID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a [2]string
		if err := rows.Scan(&a[0], &a[1]); err != nil {
			rows.Close()
			return err
		}
		addrs = append(addrs, a)
	}
	rows.Close()
	for _, ip := range ips {
		if err := Add(ctx, tx, "ip", ip[0], userID); err != nil {
			return err
		}
	}
	for _, a := range addrs {
		if err := Add(ctx, tx, "address", NormalizeAddress(a[0], a[1]), userID); err != nil {
			return err
		}
	}
	return nil
}

// Alert raises a fraud alert when a member uses a banned member's identifier.
func Alert(ctx context.Context, ex Execer, userID, kind, bannedID, where string) error {
	_, err := ex.ExecContext(ctx, `
		INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
		VALUES ('banned_identifier', 'high', $1, $2, jsonb_build_object('matched', $3::text, 'banned_user', $4::text, 'where', $5::text), $6)
		ON CONFLICT (dedupe_key) DO NOTHING`,
		userID, "Uses the same "+kind+" as a banned member ("+where+")", kind, bannedID, where,
		"banned_identifier:"+userID+":"+kind)
	return err
}
