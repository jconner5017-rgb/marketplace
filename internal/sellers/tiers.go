package sellers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var TierName = map[int]string{1: "New", 2: "Standard", 3: "Trusted"}

// ApplyTier moves a seller to tier (1–3) inside tx and records why. On a
// demotion, fixed-price listings above the new cap are hidden (not
// deleted); on a promotion, listings hidden that way come back if they now
// fit. It returns the previous tier; no change when it's already there.
func ApplyTier(ctx context.Context, tx *sql.Tx, userID string, tier int, reason, byStaff string, stats map[string]any) (int, error) {
	if _, ok := TierCapCents[tier]; !ok {
		return 0, errors.New("tier must be 1, 2 or 3")
	}
	var from int
	if err := tx.QueryRowContext(ctx, `SELECT tier FROM seller_profiles WHERE user_id = $1 FOR UPDATE`, userID).Scan(&from); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotSeller
		}
		return 0, err
	}
	if from == tier {
		return from, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE seller_profiles SET tier = $2, updated_at = now() WHERE user_id = $1`, userID, tier); err != nil {
		return 0, err
	}
	cap := TierCapCents[tier]
	var hidden, restored int64
	if tier < from {
		res, err := tx.ExecContext(ctx, `UPDATE listings SET status = 'hidden', tier_hidden = true, updated_at = now()
			WHERE seller_id = $1 AND status = 'active' AND format = 'fixed' AND price_cents > $2`, userID, cap)
		if err != nil {
			return 0, err
		}
		hidden, _ = res.RowsAffected()
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE listings SET status = CASE WHEN quantity > 0 THEN 'active' ELSE 'sold_out' END,
			tier_hidden = false, updated_at = now()
			WHERE seller_id = $1 AND tier_hidden AND status = 'hidden' AND price_cents <= $2`, userID, cap)
		if err != nil {
			return 0, err
		}
		restored, _ = res.RowsAffected()
	}
	if stats == nil {
		stats = map[string]any{}
	}
	stats["listings_hidden"], stats["listings_restored"] = hidden, restored
	b, _ := json.Marshal(stats)
	var by any
	if byStaff != "" {
		by = byStaff
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tier_changes (user_id, from_tier, to_tier, reason, by_staff, stats)
		VALUES ($1, $2, $3, $4, $5, $6)`, userID, from, tier, reason, by, string(b)); err != nil {
		return 0, err
	}
	title := fmt.Sprintf("You're now a %s seller: cards up to $%d", TierName[tier], cap/100)
	body := reason
	if hidden > 0 {
		body += fmt.Sprintf(" %d listings above the new limit are hidden until you lower their price or move back up.", hidden)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'system', $2, $3, '/sell')`,
		userID, title, body); err != nil {
		return 0, err
	}
	return from, nil
}
