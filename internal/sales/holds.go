package sales

import (
	"context"
	"time"
)

// Hold is a card set aside for a buyer (auction win or accepted offer)
// until it's paid for at checkout.
type Hold struct {
	ID           string    `json:"id"`
	ListingID    string    `json:"listing_id"`
	ListingTitle string    `json:"listing_title"`
	ListingPhoto string    `json:"listing_photo"`
	Source       string    `json:"source"` // auction or offer
	Quantity     int       `json:"quantity"`
	UnitCents    int       `json:"unit_cents"`
	Status       string    `json:"status"`
	DueAt        time.Time `json:"due_at"`
	CreatedAt    time.Time `json:"created_at"`
	Buyer        string    `json:"buyer"`
	Seller       string    `json:"seller"`
}

// HoldsFor lists a user's holds as buyer ("buying") or seller ("selling").
func (s *Service) HoldsFor(ctx context.Context, userID, side string, limit int) ([]Hold, error) {
	col := "h.buyer_id"
	if side == "selling" {
		col = "h.seller_id"
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT h.id, h.listing_id, l.title, coalesce(l.photo_urls[1], ''), h.source, h.quantity, h.unit_cents,
		       h.status, h.due_at, h.created_at, bu.username, su.username
		FROM holds h
		JOIN listings l ON l.id = h.listing_id
		JOIN users bu ON bu.id = h.buyer_id
		JOIN users su ON su.id = h.seller_id
		WHERE `+col+` = $1 ORDER BY h.created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hold{}
	for rows.Next() {
		var h Hold
		if err := rows.Scan(&h.ID, &h.ListingID, &h.ListingTitle, &h.ListingPhoto, &h.Source, &h.Quantity,
			&h.UnitCents, &h.Status, &h.DueAt, &h.CreatedAt, &h.Buyer, &h.Seller); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// RunJobs does the periodic work: close finished auctions, warn watchers,
// expire unanswered offers. main calls it every 30 seconds.
func (s *Service) RunJobs(ctx context.Context) error {
	if _, err := s.CloseDue(ctx); err != nil {
		return err
	}
	if _, err := s.NotifyEndingSoon(ctx); err != nil {
		return err
	}
	_, err := s.ExpireOffers(ctx)
	return err
}
