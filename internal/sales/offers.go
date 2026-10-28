package sales

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

var (
	ErrOffersOff     = errors.New("this listing doesn't accept offers")
	ErrNotAvailable  = errors.New("this card is no longer available")
	ErrOfferAtPrice  = errors.New("that's the full price; use Buy It Now instead")
	ErrOfferLimit    = errors.New("you've made the most offers allowed on this listing (3)")
	ErrPendingOffer  = errors.New("you already have an offer waiting for a reply on this listing")
	ErrOfferClosed   = errors.New("this offer is no longer open")
	ErrOfferNotFound = errors.New("offer not found")
	ErrOfferNote     = errors.New("notes with emails, phone numbers or payment apps can't be sent")
	ErrNoteTooLong   = errors.New("notes can be up to 500 characters")
	ErrBadCounter    = errors.New("a counteroffer must be between the two offers")
	ErrTooManyCards  = errors.New("not that many available")
	ErrUnknownAction = errors.New("action must be accept, decline or counter")
	ErrOfferTooLow   = errors.New("enter an offer of at least $0.01")
)

const (
	MaxBuyerOffers = 3
	OfferLife      = 48 * time.Hour
	maxNote        = 500
)

// Offer is one offer or counteroffer as either side sees it.
type Offer struct {
	ID           string     `json:"id"`
	ListingID    string     `json:"listing_id"`
	ListingTitle string     `json:"listing_title"`
	ListingPhoto string     `json:"listing_photo"`
	PriceCents   int        `json:"price_cents"` // the listing's price
	FromRole     string     `json:"from_role"`   // buyer or seller
	AmountCents  int        `json:"amount_cents"`
	Quantity     int        `json:"quantity"`
	Message      string     `json:"message"`
	Status       string     `json:"status"`
	DecidedBy    *string    `json:"decided_by,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
	RespondedAt  *time.Time `json:"responded_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	Buyer        string     `json:"buyer"`
	Seller       string     `json:"seller"`
	// For the viewer:
	CanRespond bool `json:"can_respond"`
	OffersLeft int  `json:"offers_left"` // buyer's remaining offers on this listing

	buyerID, sellerID string
}

type offerListing struct {
	id, sellerID, title, status, format string
	price, quantity                     int
	enabled                             bool
	settings                            listings.OfferSettings
}

func lockOfferListing(ctx context.Context, tx *sql.Tx, id string) (offerListing, error) {
	var l offerListing
	var ak, dk sql.NullString
	var av, dv sql.NullInt64
	err := tx.QueryRowContext(ctx, `
		SELECT l.id, l.seller_id, l.title, l.status, l.format, l.price_cents, l.quantity, l.offers_enabled,
		       l.offer_accept_kind, l.offer_accept_val, l.offer_decline_kind, l.offer_decline_val
		FROM listings l
		JOIN users u ON u.id = l.seller_id AND u.status = 'active'
		JOIN seller_profiles sp ON sp.user_id = l.seller_id AND sp.charges_enabled
		WHERE l.id = $1 FOR UPDATE OF l`, id).Scan(
		&l.id, &l.sellerID, &l.title, &l.status, &l.format, &l.price, &l.quantity, &l.enabled, &ak, &av, &dk, &dv)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	l.settings = listings.OfferSettings{Enabled: l.enabled}
	if ak.Valid && av.Valid {
		v := int(av.Int64)
		l.settings.AcceptKind, l.settings.AcceptValue = &ak.String, &v
	}
	if dk.Valid && dv.Valid {
		v := int(dv.Int64)
		l.settings.DeclineKind, l.settings.DeclineValue = &dk.String, &v
	}
	return l, nil
}

func (l offerListing) open() bool {
	return l.format == "fixed" && l.status == "active" && l.quantity > 0
}

func cleanNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxNote {
		return "", ErrNoteTooLong
	}
	if note != "" && inbox.Screen(note) {
		return "", ErrOfferNote
	}
	return note, nil
}

func buyerOfferCount(ctx context.Context, tx *sql.Tx, listingID, buyerID string) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM offers
		WHERE listing_id = $1 AND buyer_id = $2 AND from_role = 'buyer'`, listingID, buyerID).Scan(&n)
	return n, err
}

// MakeOffer sends a buyer's offer. The seller's automatic rules may accept
// or decline it on the spot; the returned offer shows the outcome.
func (s *Service) MakeOffer(ctx context.Context, buyerID, listingID string, amountCents, qty int, note string) (Offer, error) {
	note, err := cleanNote(note)
	if err != nil {
		return Offer{}, err
	}
	if qty < 1 {
		qty = 1
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Offer{}, err
	}
	defer tx.Rollback()
	l, err := lockOfferListing(ctx, tx, listingID)
	if err != nil {
		return Offer{}, err
	}
	switch {
	case !l.open():
		return Offer{}, ErrNotAvailable
	case !l.enabled:
		return Offer{}, ErrOffersOff
	case l.sellerID == buyerID:
		return Offer{}, ErrOwnListing
	case qty > l.quantity:
		return Offer{}, ErrTooManyCards
	case amountCents < 1:
		return Offer{}, ErrOfferTooLow
	case amountCents >= l.price:
		return Offer{}, ErrOfferAtPrice
	}
	n, err := buyerOfferCount(ctx, tx, listingID, buyerID)
	if err != nil {
		return Offer{}, err
	}
	if n >= MaxBuyerOffers {
		return Offer{}, ErrOfferLimit
	}
	var pending bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM offers
		WHERE listing_id = $1 AND buyer_id = $2 AND status = 'pending')`, listingID, buyerID).Scan(&pending); err != nil {
		return Offer{}, err
	}
	if pending {
		return Offer{}, ErrPendingOffer
	}
	id, err := insertOffer(ctx, tx, l, buyerID, nil, "buyer", amountCents, qty, note)
	if err != nil {
		return Offer{}, err
	}
	if err := s.applyRules(ctx, tx, l, id, buyerID, amountCents, qty); err != nil {
		return Offer{}, err
	}
	if err := tx.Commit(); err != nil {
		return Offer{}, err
	}
	return s.get(ctx, id, buyerID)
}

func insertOffer(ctx context.Context, tx *sql.Tx, l offerListing, buyerID string, parent *string, from string, amount, qty int, note string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `
		INSERT INTO offers (listing_id, buyer_id, seller_id, parent_id, from_role, amount_cents, quantity, message, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now() + interval '48 hours') RETURNING id`,
		l.id, buyerID, l.sellerID, parent, from, amount, qty, note).Scan(&id)
	return id, err
}

// applyRules runs the seller's auto-accept and auto-decline thresholds on a
// buyer's offer, or tells the seller it's waiting.
func (s *Service) applyRules(ctx context.Context, tx *sql.Tx, l offerListing, offerID, buyerID string, amount, qty int) error {
	acc, hasAcc := listings.Threshold(l.settings.AcceptKind, l.settings.AcceptValue, l.price)
	dec, hasDec := listings.Threshold(l.settings.DeclineKind, l.settings.DeclineValue, l.price)
	switch {
	case hasAcc && amount >= acc:
		return s.accept(ctx, tx, l, offerID, buyerID, amount, qty, "auto")
	case hasDec && amount < dec:
		if err := setStatus(ctx, tx, offerID, "declined", "auto"); err != nil {
			return err
		}
		return inbox.Notify(ctx, tx, buyerID, "offer", "Your offer on "+l.title+" was declined",
			"The seller is looking for a higher offer. You can try again.", "/offers")
	default:
		return inbox.Notify(ctx, tx, l.sellerID, "offer", "New offer on "+l.title,
			fmt.Sprintf("%s for %d. Reply within 48 hours.", money(amount), qty), "/offers")
	}
}

func setStatus(ctx context.Context, tx *sql.Tx, offerID, status, by string) error {
	_, err := tx.ExecContext(ctx, `UPDATE offers SET status = $2, decided_by = $3, responded_at = now() WHERE id = $1`,
		offerID, status, by)
	return err
}

// accept takes the cards off the listing and holds them for the buyer.
func (s *Service) accept(ctx context.Context, tx *sql.Tx, l offerListing, offerID, buyerID string, amount, qty int, by string) error {
	if !l.open() || qty > l.quantity {
		if err := setStatus(ctx, tx, offerID, "declined", "system"); err != nil {
			return err
		}
		return inbox.Notify(ctx, tx, buyerID, "offer", l.title+" is no longer available",
			"Your offer couldn't be accepted because the card sold.", "/offers")
	}
	if err := setStatus(ctx, tx, offerID, "accepted", by); err != nil {
		return err
	}
	left := l.quantity - qty
	if _, err := tx.ExecContext(ctx, `
		UPDATE listings SET quantity = $2, status = CASE WHEN $2 = 0 THEN 'sold_out' ELSE status END, updated_at = now()
		WHERE id = $1`, l.id, left); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO holds (listing_id, buyer_id, seller_id, source, offer_id, quantity, unit_cents, due_at)
		VALUES ($1, $2, $3, 'offer', $4, $5, $6, now() + interval '48 hours')`,
		l.id, buyerID, l.sellerID, offerID, qty, amount); err != nil {
		return err
	}
	if err := inbox.Notify(ctx, tx, buyerID, "offer", "Offer accepted: "+l.title,
		fmt.Sprintf("Your offer of %s was accepted. Pay within 48 hours from Purchases.", money(amount)), "/purchases"); err != nil {
		return err
	}
	if err := inbox.Notify(ctx, tx, l.sellerID, "offer", "Offer accepted: "+l.title,
		fmt.Sprintf("Sold %d for %s each. The buyer has 48 hours to pay.", qty, money(amount)), "/offers"); err != nil {
		return err
	}
	if left == 0 {
		return closeOthers(ctx, tx, l, offerID)
	}
	return nil
}

// closeOthers declines every other open offer once the card is gone.
func closeOthers(ctx context.Context, tx *sql.Tx, l offerListing, keep string) error {
	rows, err := tx.QueryContext(ctx, `
		UPDATE offers SET status = 'declined', decided_by = 'system', responded_at = now()
		WHERE listing_id = $1 AND status = 'pending' AND id <> $2 RETURNING buyer_id`, l.id, keep)
	if err != nil {
		return err
	}
	var buyers []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		buyers = append(buyers, b)
	}
	rows.Close()
	for _, b := range buyers {
		if err := inbox.Notify(ctx, tx, b, "offer", l.title+" sold",
			"The card sold to another buyer, so your offer was closed.", "/offers"); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Respond answers an offer: accept, decline, or counter with a new amount.
// Sellers answer buyers' offers; buyers answer sellers' counteroffers.
func (s *Service) Respond(ctx context.Context, userID, offerID, action string, counterCents int, note string) (Offer, error) {
	note, err := cleanNote(note)
	if err != nil {
		return Offer{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Offer{}, err
	}
	defer tx.Rollback()
	var listingID, buyerID, sellerID, from, status string
	var amount, qty int
	var expired bool
	err = tx.QueryRowContext(ctx, `
		SELECT listing_id, buyer_id, seller_id, from_role, status, amount_cents, quantity, expires_at <= now()
		FROM offers WHERE id = $1 FOR UPDATE`, offerID).Scan(
		&listingID, &buyerID, &sellerID, &from, &status, &amount, &qty, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		return Offer{}, ErrOfferNotFound
	}
	if err != nil {
		return Offer{}, err
	}
	recipient := sellerID
	if from == "seller" {
		recipient = buyerID
	}
	if userID != recipient {
		if userID == buyerID || userID == sellerID {
			return Offer{}, ErrOfferClosed // waiting on the other side
		}
		return Offer{}, ErrOfferNotFound
	}
	if status != "pending" || expired {
		return Offer{}, ErrOfferClosed
	}
	l, err := lockOfferListing(ctx, tx, listingID)
	if err != nil {
		return Offer{}, err
	}
	result := offerID
	switch action {
	case "accept":
		if err := s.accept(ctx, tx, l, offerID, buyerID, amount, qty, "person"); err != nil {
			return Offer{}, err
		}
	case "decline":
		if err := setStatus(ctx, tx, offerID, "declined", "person"); err != nil {
			return Offer{}, err
		}
		to, what := buyerID, "Your offer on "+l.title+" was declined"
		if from == "seller" {
			to, what = sellerID, "Your counteroffer on "+l.title+" was declined"
		}
		if err := inbox.Notify(ctx, tx, to, "offer", what, "", "/offers"); err != nil {
			return Offer{}, err
		}
	case "counter":
		if !l.open() || qty > l.quantity {
			return Offer{}, ErrNotAvailable
		}
		if from == "buyer" { // seller counters: above the buyer's offer, up to the price
			if counterCents <= amount || counterCents > l.price {
				return Offer{}, ErrBadCounter
			}
		} else { // buyer counters back: below the seller's counter
			if counterCents < 1 || counterCents >= amount {
				return Offer{}, ErrBadCounter
			}
			n, err := buyerOfferCount(ctx, tx, listingID, buyerID)
			if err != nil {
				return Offer{}, err
			}
			if n >= MaxBuyerOffers {
				return Offer{}, ErrOfferLimit
			}
		}
		if err := setStatus(ctx, tx, offerID, "countered", "person"); err != nil {
			return Offer{}, err
		}
		role := "seller"
		if from == "seller" {
			role = "buyer"
		}
		id, err := insertOffer(ctx, tx, l, buyerID, &offerID, role, counterCents, qty, note)
		if err != nil {
			return Offer{}, err
		}
		result = id
		if role == "seller" {
			if err := inbox.Notify(ctx, tx, buyerID, "offer", "Counteroffer on "+l.title,
				"The seller countered at "+money(counterCents)+". Reply within 48 hours.", "/offers"); err != nil {
				return Offer{}, err
			}
		} else if err := s.applyRules(ctx, tx, l, id, buyerID, counterCents, qty); err != nil {
			return Offer{}, err
		}
	default:
		return Offer{}, ErrUnknownAction
	}
	if err := tx.Commit(); err != nil {
		return Offer{}, err
	}
	return s.get(ctx, result, userID)
}

// ExpireOffers closes offers nobody answered within 48 hours.
func (s *Service) ExpireOffers(ctx context.Context) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
		WITH gone AS (
			UPDATE offers o SET status = 'expired', decided_by = 'system', responded_at = now()
			WHERE o.status = 'pending' AND o.expires_at <= now()
			RETURNING o.listing_id, CASE WHEN o.from_role = 'buyer' THEN o.buyer_id ELSE o.seller_id END AS sender
		)
		INSERT INTO notifications (user_id, kind, title, body, link)
		SELECT gone.sender, 'offer', 'Your offer on ' || l.title || ' expired', 'It wasn''t answered within 48 hours.', '/offers'
		FROM gone JOIN listings l ON l.id = gone.listing_id`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

const offerSelect = `
	SELECT o.id, o.listing_id, l.title, coalesce(l.photo_urls[1], ''), l.price_cents, o.from_role,
	       o.amount_cents, o.quantity, o.message, o.status, o.decided_by, o.expires_at, o.responded_at, o.created_at,
	       bu.username, su.username, o.buyer_id, o.seller_id,
	       (SELECT count(*) FROM offers x WHERE x.listing_id = o.listing_id AND x.buyer_id = o.buyer_id AND x.from_role = 'buyer')
	FROM offers o
	JOIN listings l ON l.id = o.listing_id
	JOIN users bu ON bu.id = o.buyer_id
	JOIN users su ON su.id = o.seller_id`

func scanOffer(row interface{ Scan(...any) error }, viewer string) (Offer, error) {
	var o Offer
	var by sql.NullString
	var resp sql.NullTime
	var made int
	err := row.Scan(&o.ID, &o.ListingID, &o.ListingTitle, &o.ListingPhoto, &o.PriceCents, &o.FromRole,
		&o.AmountCents, &o.Quantity, &o.Message, &o.Status, &by, &o.ExpiresAt, &resp, &o.CreatedAt,
		&o.Buyer, &o.Seller, &o.buyerID, &o.sellerID, &made)
	if err != nil {
		return o, err
	}
	if by.Valid {
		o.DecidedBy = &by.String
	}
	if resp.Valid {
		t := resp.Time
		o.RespondedAt = &t
	}
	o.OffersLeft = max(0, MaxBuyerOffers-made)
	recipient := o.sellerID
	if o.FromRole == "seller" {
		recipient = o.buyerID
	}
	o.CanRespond = o.Status == "pending" && viewer == recipient && time.Now().Before(o.ExpiresAt)
	return o, nil
}

func (s *Service) get(ctx context.Context, id, viewer string) (Offer, error) {
	o, err := scanOffer(s.DB.QueryRowContext(ctx, offerSelect+` WHERE o.id = $1`, id), viewer)
	if errors.Is(err, sql.ErrNoRows) {
		return o, ErrOfferNotFound
	}
	return o, err
}

// OffersFor lists offers where the user is the buyer ("buying") or the
// seller ("selling"), newest first, optionally for one listing.
func (s *Service) OffersFor(ctx context.Context, userID, side, listingID string, limit int) ([]Offer, error) {
	col := "o.buyer_id"
	if side == "selling" {
		col = "o.seller_id"
	}
	q := offerSelect + ` WHERE ` + col + ` = $1 AND ($2 = '' OR o.listing_id::text = $2) ORDER BY o.created_at DESC LIMIT $3`
	rows, err := s.DB.QueryContext(ctx, q, userID, listingID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Offer{}
	for rows.Next() {
		o, err := scanOffer(rows, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
