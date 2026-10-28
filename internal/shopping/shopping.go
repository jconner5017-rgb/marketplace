// Package shopping holds buyers' watchlists and carts.
package shopping

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

var (
	ErrNotAvailable = errors.New("this card is no longer available")
	ErrOwnListing   = errors.New("you can't buy or watch your own listing")
	ErrTooMany      = errors.New("not that many available")
	ErrCartFull     = errors.New("your cart is full")
	ErrAuctionCart  = errors.New("auctions can't go in the cart; place a bid instead")
)

const MaxCartLines = 50

type Service struct {
	DB       *sql.DB
	Listings *listings.Service
	Sellers  *sellers.Service
}

// available fetches a listing buyers can purchase and checks it isn't the user's own.
func (s *Service) available(ctx context.Context, userID, listingID string) (listings.Listing, error) {
	l, err := s.Listings.GetPublic(ctx, listingID)
	if errors.Is(err, listings.ErrNotFound) {
		return l, ErrNotAvailable
	}
	if err != nil {
		return l, err
	}
	if l.SellerID == userID {
		return l, ErrOwnListing
	}
	if !l.Open() {
		return l, ErrNotAvailable
	}
	return l, nil
}

// ---- Watchlist ----

func (s *Service) Watch(ctx context.Context, userID, listingID string) error {
	if _, err := s.available(ctx, userID, listingID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO watchlist (user_id, listing_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, listingID)
	return err
}

func (s *Service) Unwatch(ctx context.Context, userID, listingID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM watchlist WHERE user_id = $1 AND listing_id = $2`, userID, listingID)
	return err
}

func (s *Service) IsWatching(ctx context.Context, userID, listingID string) (bool, error) {
	var ok bool
	err := s.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM watchlist WHERE user_id = $1 AND listing_id = $2)`, userID, listingID).Scan(&ok)
	return ok, err
}

type WatchItem struct {
	Listing   listings.Listing `json:"listing"`
	Available bool             `json:"available"`
	InCart    bool             `json:"in_cart"`
}

// Watchlist returns watched cards, newest first. Sold or removed cards stay
// visible, marked unavailable, until the user removes them.
func (s *Service) Watchlist(ctx context.Context, userID string) ([]WatchItem, error) {
	ids, err := s.ids(ctx, `SELECT listing_id FROM watchlist WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	all, err := s.Listings.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	pub, err := s.Listings.PublicIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	inCart, err := s.idSet(ctx, `SELECT listing_id FROM cart_items WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	out := []WatchItem{}
	for _, id := range ids {
		l, ok := all[id]
		if !ok || l.Status == "removed" {
			continue
		}
		l.ExternalID = nil
		out = append(out, WatchItem{Listing: l, Available: pub[id], InCart: inCart[id]})
	}
	return out, nil
}

// ---- Cart ----

// SetCartQuantity adds a card to the cart or changes its quantity.
func (s *Service) SetCartQuantity(ctx context.Context, userID, listingID string, qty int) error {
	l, err := s.available(ctx, userID, listingID)
	if err != nil {
		return err
	}
	if l.Format == listings.FormatAuction {
		return ErrAuctionCart
	}
	if qty < 1 {
		qty = 1
	}
	if qty > l.Quantity {
		return ErrTooMany
	}
	var lines int
	if err := s.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM cart_items WHERE user_id = $1 AND listing_id <> $2`, userID, listingID).Scan(&lines); err != nil {
		return err
	}
	if lines >= MaxCartLines {
		return ErrCartFull
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO cart_items (user_id, listing_id, quantity) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, listing_id) DO UPDATE SET quantity = EXCLUDED.quantity`, userID, listingID, qty)
	return err
}

func (s *Service) RemoveFromCart(ctx context.Context, userID, listingID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM cart_items WHERE user_id = $1 AND listing_id = $2`, userID, listingID)
	return err
}

type CartLine struct {
	Listing   listings.Listing `json:"listing"`
	Quantity  int              `json:"quantity"`
	LineCents int              `json:"line_cents"`
	Available bool             `json:"available"`
	Problem   string           `json:"problem,omitempty"` // shown to the buyer
}

type SellerGroup struct {
	SellerUsername string     `json:"seller_username"`
	Lines          []CartLine `json:"lines"`
	CardCount      int        `json:"card_count"`
	SubtotalCents  int        `json:"subtotal_cents"`
	DiscountCents  int        `json:"discount_cents"`
	DiscountLabel  string     `json:"discount_label,omitempty"`
	TotalCents     int        `json:"total_cents"`
	ShipMethod     string     `json:"ship_method"` // envelope or label
}

type Cart struct {
	Groups     []SellerGroup `json:"groups"`
	CardCount  int           `json:"card_count"`
	TotalCents int           `json:"total_cents"`
	Problems   int           `json:"problems"`
}

// GetCart returns the cart grouped by seller with each seller's
// multi-card discount applied. Shipping is always free.
func (s *Service) GetCart(ctx context.Context, userID string) (Cart, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT listing_id, quantity FROM cart_items WHERE user_id = $1 ORDER BY added_at`, userID)
	if err != nil {
		return Cart{}, err
	}
	var ids []string
	qty := map[string]int{}
	for rows.Next() {
		var id string
		var q int
		if err := rows.Scan(&id, &q); err != nil {
			rows.Close()
			return Cart{}, err
		}
		ids = append(ids, id)
		qty[id] = q
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Cart{}, err
	}

	all, err := s.Listings.GetMany(ctx, ids)
	if err != nil {
		return Cart{}, err
	}
	pub, err := s.Listings.PublicIDs(ctx, ids)
	if err != nil {
		return Cart{}, err
	}

	groups := map[string]*SellerGroup{}
	order := []string{}
	cart := Cart{Groups: []SellerGroup{}}
	for _, id := range ids {
		l, ok := all[id]
		if !ok {
			continue
		}
		l.ExternalID = nil
		line := CartLine{Listing: l, Quantity: qty[id], Available: pub[id]}
		switch {
		case !line.Available:
			line.Problem = "No longer available"
		case line.Quantity > l.Quantity:
			line.Problem = "Only " + itoa(l.Quantity) + " left"
			line.Available = false
		}
		if line.Available {
			line.LineCents = l.PriceCents * line.Quantity
		}
		g, ok := groups[l.SellerID]
		if !ok {
			g = &SellerGroup{SellerUsername: l.SellerUsername}
			groups[l.SellerID] = g
			order = append(order, l.SellerID)
		}
		g.Lines = append(g.Lines, line)
		if line.Available {
			g.CardCount += line.Quantity
			g.SubtotalCents += line.LineCents
		} else {
			cart.Problems++
		}
	}

	for _, sellerID := range order {
		g := groups[sellerID]
		if p, err := s.Sellers.Get(ctx, sellerID); err == nil {
			g.DiscountCents, g.DiscountLabel = BundleDiscount(p.BundleDiscountType, p.BundleDiscountVal, g.CardCount, g.SubtotalCents)
		}
		g.TotalCents = g.SubtotalCents - g.DiscountCents
		g.ShipMethod = ShipMethod(*g)
		cart.CardCount += g.CardCount
		cart.TotalCents += g.TotalCents
		cart.Groups = append(cart.Groups, *g)
	}
	// Sellers with the most in the cart first, like a shopping bag.
	sort.SliceStable(cart.Groups, func(i, j int) bool { return cart.Groups[i].TotalCents > cart.Groups[j].TotalCents })
	return cart, nil
}

// Counts are the numbers shown in the header badges.
type Counts struct {
	CartCards int `json:"cart_cards"`
	Watching  int `json:"watching"`
}

func (s *Service) Counts(ctx context.Context, userID string) (Counts, error) {
	var c Counts
	err := s.DB.QueryRowContext(ctx, `
		SELECT (SELECT coalesce(sum(quantity), 0) FROM cart_items WHERE user_id = $1),
		       (SELECT count(*) FROM watchlist WHERE user_id = $1)`, userID).Scan(&c.CartCards, &c.Watching)
	return c, err
}

func (s *Service) ids(ctx context.Context, q string, args ...any) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Service) idSet(ctx context.Context, q string, args ...any) (map[string]bool, error) {
	ids, err := s.ids(ctx, q, args...)
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out, err
}
