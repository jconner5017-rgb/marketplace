// Package sales runs auctions (proxy bidding, Buy It Now, reserve, soft
// close, relist) and Best Offer, and holds won or accepted cards for the
// buyer until checkout.
package sales

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

var (
	ErrNotFound      = errors.New("listing not found")
	ErrNotAuction    = errors.New("this listing isn't an auction")
	ErrAuctionOver   = errors.New("this auction has ended")
	ErrOwnListing    = errors.New("you can't bid on or make offers on your own listing")
	ErrNoBuyNow      = errors.New("Buy It Now is no longer available on this auction")
	ErrAlreadyLeader = errors.New("you're already the high bidder at that maximum or higher")
)

// BidTooLowError says what the lowest acceptable bid is.
type BidTooLowError struct{ MinCents int }

func (e *BidTooLowError) Error() string {
	return fmt.Sprintf("enter %s or more", money(e.MinCents))
}

const (
	SoftClose    = 2 * time.Minute // a late bid pushes the end this far past the bid
	AuctionPayBy = 4 * 24 * time.Hour
	OfferPayBy   = 48 * time.Hour
)

type Service struct {
	DB *sql.DB
}

func money(c int) string { return fmt.Sprintf("$%d.%02d", c/100, c%100) }

// Increment is the bid step at a price, in cents.
func Increment(c int) int {
	switch {
	case c < 100:
		return 5
	case c < 500:
		return 25
	case c < 2500:
		return 50
	case c < 10000:
		return 100
	case c < 25000:
		return 250
	case c < 50000:
		return 500
	default:
		return 1000
	}
}

// auction is a locked auction row.
type auction struct {
	id, sellerID, title, state, status string
	format                             string
	endsAt, now                        time.Time
	price, start, bidCount, round      int
	reserve, bin                       sql.NullInt64
	leaderID                           sql.NullString
	leaderMax                          sql.NullInt64
	capCents                           int
	autoRelist                         bool
	durationDays                       int
}

func lockAuction(ctx context.Context, tx *sql.Tx, id string) (auction, error) {
	var a auction
	var tier int
	var start, duration sql.NullInt64
	var state sql.NullString
	var endsAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT l.id, l.seller_id, l.title, l.format, l.status, l.auction_state, l.ends_at, now(),
		       l.price_cents, l.start_cents, l.bid_count, l.auction_round, l.reserve_cents, l.bin_cents,
		       l.leader_id, l.leader_max_cents, sp.tier, l.auto_relist, l.duration_days
		FROM listings l JOIN seller_profiles sp ON sp.user_id = l.seller_id
		WHERE l.id = $1 FOR UPDATE OF l`, id).Scan(
		&a.id, &a.sellerID, &a.title, &a.format, &a.status, &state, &endsAt, &a.now,
		&a.price, &start, &a.bidCount, &a.round, &a.reserve, &a.bin,
		&a.leaderID, &a.leaderMax, &tier, &a.autoRelist, &duration)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if a.format != "auction" {
		return a, ErrNotAuction
	}
	a.state, a.endsAt = state.String, endsAt.Time
	a.start, a.durationDays = int(start.Int64), int(duration.Int64)
	a.capCents = sellers.TierCapCents[tier]
	return a, nil
}

func (a auction) live() bool {
	return a.state == "live" && a.status == "active" && a.now.Before(a.endsAt)
}

// BidResult is what a bidder sees after bidding.
type BidResult struct {
	Leading      bool      `json:"leading"`
	CurrentCents int       `json:"current_cents"`
	MinNextCents int       `json:"min_next_cents"`
	BidCount     int       `json:"bid_count"`
	ReserveMet   bool      `json:"reserve_met"`
	EndsAt       time.Time `json:"ends_at"`
	Won          bool      `json:"won"` // bidding reached the seller's price limit
}

// PlaceBid records a proxy bid: maxCents is the most the bidder will pay.
func (s *Service) PlaceBid(ctx context.Context, bidderID, listingID string, maxCents int, ip, ua string) (BidResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return BidResult{}, err
	}
	defer tx.Rollback()
	a, err := lockAuction(ctx, tx, listingID)
	if err != nil {
		return BidResult{}, err
	}
	if !a.live() {
		return BidResult{}, ErrAuctionOver
	}
	if a.sellerID == bidderID {
		return BidResult{}, ErrOwnListing
	}
	if maxCents > a.capCents {
		maxCents = a.capCents // nobody can bid past the seller's price limit
	}

	minNext := a.start
	if a.bidCount > 0 {
		minNext = a.price + Increment(a.price)
	}
	prevLeader := a.leaderID.String
	price, leader, lmax := a.price, prevLeader, int(a.leaderMax.Int64)

	switch {
	case a.bidCount > 0 && prevLeader == bidderID:
		if maxCents <= lmax {
			return BidResult{}, ErrAlreadyLeader
		}
		lmax = maxCents
	case maxCents < minNext:
		return BidResult{}, &BidTooLowError{MinCents: min(minNext, a.capCents)}
	case a.bidCount == 0:
		leader, lmax, price = bidderID, maxCents, a.start
	case maxCents > lmax:
		leader, price, lmax = bidderID, min(maxCents, lmax+Increment(lmax)), maxCents
	default: // the current leader's maximum covers this bid; earlier bids win ties
		price = min(lmax, maxCents+Increment(maxCents))
	}
	// Once the leader's maximum meets the reserve, the price jumps to it.
	if a.reserve.Valid && price < int(a.reserve.Int64) && lmax >= int(a.reserve.Int64) {
		price = int(a.reserve.Int64)
	}
	price = min(price, a.capCents)

	shown := price
	if leader != bidderID {
		shown = maxCents
	}
	endsAt := a.endsAt
	if endsAt.Sub(a.now) < SoftClose {
		endsAt = a.now.Add(SoftClose)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bids (listing_id, round, bidder_id, max_cents, shown_cents, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, a.id, a.round, bidderID, maxCents, shown, ip, clip(ua, 300)); err != nil {
		return BidResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE listings SET price_cents = $2, leader_id = $3, leader_max_cents = $4,
			bid_count = bid_count + 1, ends_at = $5, updated_at = now()
		WHERE id = $1`, a.id, price, leader, lmax, endsAt); err != nil {
		return BidResult{}, err
	}
	a.price, a.bidCount, a.leaderID, a.endsAt = price, a.bidCount+1, sql.NullString{String: leader, Valid: true}, endsAt

	link := "/item/" + a.id
	if prevLeader != "" && prevLeader != leader {
		if err := inbox.Notify(ctx, tx, prevLeader, "auction", "You've been outbid on "+a.title,
			"The price is now "+money(price)+". Bid again before it ends.", link); err != nil {
			return BidResult{}, err
		}
	}
	if a.bidCount == 1 {
		if err := inbox.Notify(ctx, tx, a.sellerID, "auction", "First bid on "+a.title,
			"Your auction has its first bid at "+money(price)+".", link); err != nil {
			return BidResult{}, err
		}
	}

	res := BidResult{Leading: leader == bidderID, CurrentCents: price, BidCount: a.bidCount,
		MinNextCents: min(price+Increment(price), a.capCents), EndsAt: endsAt,
		ReserveMet: !a.reserve.Valid || price >= int(a.reserve.Int64)}
	if price >= a.capCents {
		// The price limit works like Buy It Now: the auction ends now.
		if err := s.finish(ctx, tx, a); err != nil {
			return BidResult{}, err
		}
		res.Won = leader == bidderID
	}
	return res, tx.Commit()
}

// BuyNow ends an auction that has no bids at its Buy It Now price.
func (s *Service) BuyNow(ctx context.Context, buyerID, listingID, ip, ua string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	a, err := lockAuction(ctx, tx, listingID)
	if err != nil {
		return err
	}
	if !a.live() {
		return ErrAuctionOver
	}
	if a.sellerID == buyerID {
		return ErrOwnListing
	}
	if a.bidCount > 0 || !a.bin.Valid {
		return ErrNoBuyNow
	}
	bin := int(a.bin.Int64)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bids (listing_id, round, bidder_id, max_cents, shown_cents, ip, user_agent)
		VALUES ($1, $2, $3, $4, $4, $5, $6)`, a.id, a.round, buyerID, bin, ip, clip(ua, 300)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE listings SET price_cents = $2, leader_id = $3, leader_max_cents = $2, bid_count = 1, updated_at = now()
		WHERE id = $1`, a.id, bin, buyerID); err != nil {
		return err
	}
	a.price, a.bidCount, a.leaderID = bin, 1, sql.NullString{String: buyerID, Valid: true}
	a.reserve = sql.NullInt64{} // Buy It Now is always at or above the reserve
	if err := s.finish(ctx, tx, a); err != nil {
		return err
	}
	return tx.Commit()
}

// finish closes a locked auction: a winner gets a hold to pay within four
// days; an unsold auction relists once if the seller asked for it.
func (s *Service) finish(ctx context.Context, tx *sql.Tx, a auction) error {
	link := "/item/" + a.id
	met := !a.reserve.Valid || a.price >= int(a.reserve.Int64)
	if a.bidCount > 0 && met && a.leaderID.Valid {
		winner := a.leaderID.String
		if _, err := tx.ExecContext(ctx, `
			UPDATE listings SET auction_state = 'won', winner_id = $2, status = 'ended', quantity = 0,
				ended_at = now(), updated_at = now()
			WHERE id = $1`, a.id, winner); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO holds (listing_id, buyer_id, seller_id, source, quantity, unit_cents, due_at)
			VALUES ($1, $2, $3, 'auction', 1, $4, now() + $5::interval)`,
			a.id, winner, a.sellerID, a.price, fmt.Sprintf("%d seconds", int(AuctionPayBy.Seconds()))); err != nil {
			return err
		}
		if err := inbox.Notify(ctx, tx, winner, "auction", "You won "+a.title,
			"You won for "+money(a.price)+". Pay within 4 days from Purchases.", "/purchases"); err != nil {
			return err
		}
		return inbox.Notify(ctx, tx, a.sellerID, "auction", a.title+" sold for "+money(a.price),
			"The buyer has 4 days to pay. Ship once payment arrives.", link)
	}
	if a.autoRelist && a.round == 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE listings SET auction_round = 1, starts_at = now(), ends_at = now() + make_interval(days => duration_days),
				bid_count = 0, leader_id = NULL, leader_max_cents = NULL, price_cents = start_cents,
				ending_notice = false, updated_at = now()
			WHERE id = $1`, a.id); err != nil {
			return err
		}
		return inbox.Notify(ctx, tx, a.sellerID, "auction", a.title+" didn't sell and was relisted",
			fmt.Sprintf("It runs another %d days at the same settings.", a.durationDays), link)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE listings SET auction_state = 'unsold', status = 'ended', ended_at = now(), updated_at = now()
		WHERE id = $1`, a.id); err != nil {
		return err
	}
	why := "There were no bids."
	if a.bidCount > 0 {
		why = "Bidding didn't reach your reserve price."
	}
	return inbox.Notify(ctx, tx, a.sellerID, "auction", a.title+" didn't sell", why+" You can list it again.", link)
}

// CloseDue ends every auction whose time is up. It returns how many closed.
func (s *Service) CloseDue(ctx context.Context) (int, error) {
	n := 0
	for {
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return n, err
		}
		var id string
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM listings
			WHERE auction_state = 'live' AND status = 'active' AND ends_at <= now()
			ORDER BY ends_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			tx.Rollback()
			return n, nil
		}
		if err != nil {
			tx.Rollback()
			return n, err
		}
		a, err := lockAuction(ctx, tx, id)
		if err == nil {
			err = s.finish(ctx, tx, a)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			tx.Rollback()
			return n, err
		}
		n++
	}
}

// NotifyEndingSoon tells watchers when an auction has under an hour left.
func (s *Service) NotifyEndingSoon(ctx context.Context) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
		WITH due AS (
			UPDATE listings SET ending_notice = true
			WHERE auction_state = 'live' AND status = 'active' AND NOT ending_notice
			  AND ends_at > now() AND ends_at <= now() + interval '1 hour'
			RETURNING id, title, price_cents
		)
		INSERT INTO notifications (user_id, kind, title, body, link)
		SELECT w.user_id, 'auction', due.title || ' ends within the hour',
		       'Current bid $' || to_char(due.price_cents / 100.0, 'FM999990.00') || '.', '/item/' || due.id
		FROM due JOIN watchlist w ON w.listing_id = due.id`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// BidView is one row of an auction's public bid history.
type BidView struct {
	Bidder string    `json:"bidder"` // masked username
	Cents  int       `json:"cents"`
	At     time.Time `json:"at"`
	You    bool      `json:"you"`
}

type BidHistory struct {
	Bids    []BidView `json:"bids"`
	Bidders int       `json:"bidders"`
	// For the signed-in viewer only:
	Leading  bool `json:"leading"`
	MyMax    *int `json:"my_max_cents,omitempty"`
	Won      bool `json:"won"`
	Outbid   bool `json:"outbid"`
	MinNext  int  `json:"min_next_cents"`
	CapCents int  `json:"cap_cents"`
}

// Bids returns the current round's bid history, newest first.
func (s *Service) Bids(ctx context.Context, listingID, viewerID string) (BidHistory, error) {
	var round, price, start, count int
	var leader, winner sql.NullString
	var tier int
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.auction_round, l.price_cents, coalesce(l.start_cents, 0), l.bid_count, l.leader_id, l.winner_id, sp.tier
		FROM listings l JOIN seller_profiles sp ON sp.user_id = l.seller_id
		WHERE l.id = $1 AND l.format = 'auction'`, listingID).Scan(&round, &price, &start, &count, &leader, &winner, &tier)
	if errors.Is(err, sql.ErrNoRows) {
		return BidHistory{}, ErrNotFound
	}
	if err != nil {
		return BidHistory{}, err
	}
	h := BidHistory{Bids: []BidView{}, CapCents: sellers.TierCapCents[tier]}
	h.MinNext = start
	if count > 0 {
		h.MinNext = min(price+Increment(price), h.CapCents)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT u.username, b.shown_cents, b.created_at, b.bidder_id::text = $3
		FROM bids b JOIN users u ON u.id = b.bidder_id
		WHERE b.listing_id = $1 AND b.round = $2 AND b.removed_at IS NULL
		ORDER BY b.id DESC LIMIT 200`, listingID, round, viewerID)
	if err != nil {
		return h, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var b BidView
		var name string
		if err := rows.Scan(&name, &b.Cents, &b.At, &b.You); err != nil {
			return h, err
		}
		seen[name] = true
		b.Bidder = Mask(name)
		h.Bids = append(h.Bids, b)
	}
	h.Bidders = len(seen)
	if viewerID != "" {
		h.Leading = leader.Valid && leader.String == viewerID
		h.Won = winner.Valid && winner.String == viewerID
		var myMax sql.NullInt64
		s.DB.QueryRowContext(ctx, `SELECT max(max_cents) FROM bids
			WHERE listing_id = $1 AND round = $2 AND bidder_id = $3 AND removed_at IS NULL`,
			listingID, round, viewerID).Scan(&myMax)
		if myMax.Valid {
			v := int(myMax.Int64)
			h.MyMax = &v
			h.Outbid = !h.Leading
		}
	}
	return h, rows.Err()
}

// Mask shortens a username for public bid history: "jconner5017" → "j***7".
func Mask(name string) string {
	r := []rune(name)
	if len(r) < 2 {
		return "***"
	}
	return string(r[0]) + "***" + string(r[len(r)-1])
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
