package fcm

import (
	"context"
	"fmt"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
)

// StaleAfter is how long inventory software may go without checking in
// before its listings are paused.
const StaleAfter = 72 * time.Hour

// Paused is one seller whose synced listings were just paused.
type Paused struct {
	UserID, Email, Username string
	Listings                int
}

// PauseStale hides fixed-price listings sent by inventory software for
// sellers whose API keys haven't been used in StaleAfter. The seller gets a
// notification; the caller can email them too.
func (s *Service) PauseStale(ctx context.Context) ([]Paused, error) {
	cutoff := s.now().Add(-StaleAfter)
	rows, err := s.DB.QueryContext(ctx, `
		WITH stale AS (
			SELECT u.id FROM users u
			WHERE EXISTS (SELECT 1 FROM listings l WHERE l.seller_id = u.id AND l.inventory_sync AND l.status = 'active' AND l.format = 'fixed')
			  AND coalesce((SELECT max(k.last_used_at) FROM api_keys k WHERE k.user_id = u.id), '-infinity') < $1
		)
		UPDATE listings l SET status = 'hidden', sync_paused_at = now(), updated_at = now()
		FROM stale WHERE l.seller_id = stale.id AND l.inventory_sync AND l.status = 'active' AND l.format = 'fixed'
		RETURNING l.seller_id`, cutoff)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		counts[id]++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Paused, 0, len(counts))
	for id, n := range counts {
		p := Paused{UserID: id, Listings: n}
		if err := s.DB.QueryRowContext(ctx, `SELECT email, username FROM users WHERE id = $1`, id).Scan(&p.Email, &p.Username); err != nil {
			return out, err
		}
		if err := inbox.Notify(ctx, s.DB, id, "listing", fmt.Sprintf("%d FireCardManager listings paused", n),
			"FireCardManager hasn't synced in 3 days, so we paused the cards it listed to stop them selling twice. Open FireCardManager and they come back on its next sync.",
			"/sell"); err != nil {
			return out, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ResumeSynced brings back a seller's paused listings when their inventory
// software checks in again. It returns how many came back.
func (s *Service) ResumeSynced(ctx context.Context, userID string) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE listings SET status = CASE WHEN quantity > 0 THEN 'active' ELSE 'sold_out' END,
			sync_paused_at = NULL, updated_at = now()
		WHERE seller_id = $1 AND sync_paused_at IS NOT NULL AND status = 'hidden'`, userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		if err := inbox.Notify(ctx, s.DB, userID, "listing", fmt.Sprintf("%d FireCardManager listings are back", n),
			"FireCardManager synced again, so the listings we paused are for sale again.", "/sell"); err != nil {
			return int(n), err
		}
	}
	return int(n), nil
}
