package checkout

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
)

// Envelope limits. Anyone's single raw card at EnvelopeMaxCents or less may
// ship in a stamped envelope; a trusted buyer may choose one up to
// TrustedEnvelopeMaxCents. Past that, the risk is too great without tracking.
const (
	EnvelopeMaxCents        = 2000
	TrustedEnvelopeMaxCents = 10000
)

var (
	ErrNotTrusted     = errors.New("the seller hasn't added you as a trusted buyer")
	ErrNoEnvelope     = errors.New("this order can't ship in an envelope: envelopes are for one raw card up to $100")
	ErrBadInvite      = errors.New("this invite link isn't valid any more; ask the seller for a new one")
	ErrTrustSelf      = errors.New("you can't add yourself")
	ErrUnknownBuyer   = &FieldError{"username", "there's no member with that username"}
	ErrNotCoveredINR  = &FieldError{"kind", "this order shipped by stamped envelope at your request, so a missing envelope isn't covered by Slabs & Singles. Message the seller, or choose another problem"}
	ErrPrintEnvelope  = &FieldError{"tracking_number", "print the tracked envelope from this order first: envelope orders over $20 need its barcode"}
	ErrOfferNotOpen   = errors.New("there's no envelope offer waiting on this order")
	ErrAlreadyOffered = errors.New("you've already offered an envelope on this order")
)

// TrustedEnvelopeEligible: one raw card that isn't set up to ship in a box,
// up to $100.
func TrustedEnvelopeEligible(cards, totalCents int, graded bool, preset string) bool {
	return cards == 1 && !graded && preset != "box" && totalCents > 0 && totalCents <= TrustedEnvelopeMaxCents
}

// IsTrusted reports whether the seller trusts the buyer.
func IsTrusted(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, sellerID, buyerID string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM trusted_buyers WHERE seller_id = $1 AND buyer_id = $2)`,
		sellerID, buyerID).Scan(&ok)
	return ok, err
}

type TrustedBuyer struct {
	Username string    `json:"username"`
	Via      string    `json:"via"`
	AddedAt  time.Time `json:"added_at"`
}

// TrustedBuyers lists a seller's trusted buyers and their invite code.
func (s *Service) TrustedBuyers(ctx context.Context, sellerID string) ([]TrustedBuyer, string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT u.username, t.via, t.added_at FROM trusted_buyers t JOIN users u ON u.id = t.buyer_id
		WHERE t.seller_id = $1 ORDER BY lower(u.username)`, sellerID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []TrustedBuyer{}
	for rows.Next() {
		var t TrustedBuyer
		if err := rows.Scan(&t.Username, &t.Via, &t.AddedAt); err != nil {
			return nil, "", err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	invite, err := s.inviteCode(ctx, sellerID, false)
	return out, invite, err
}

func newInvite() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// inviteCode returns the seller's invite code, making one if needed (or a
// new one when reset).
func (s *Service) inviteCode(ctx context.Context, sellerID string, reset bool) (string, error) {
	var cur sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT trust_invite FROM seller_profiles WHERE user_id = $1`, sellerID).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil // not a seller yet
	}
	if err != nil {
		return "", err
	}
	if cur.Valid && cur.String != "" && !reset {
		return cur.String, nil
	}
	code := newInvite()
	if _, err := s.DB.ExecContext(ctx, `UPDATE seller_profiles SET trust_invite = $2 WHERE user_id = $1`, sellerID, code); err != nil {
		return "", err
	}
	return code, nil
}

// ResetInvite replaces the invite link so the old one stops working.
func (s *Service) ResetInvite(ctx context.Context, sellerID string) (string, error) {
	return s.inviteCode(ctx, sellerID, true)
}

func (s *Service) trust(ctx context.Context, ex interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, sellerID, buyerID, via string) error {
	if sellerID == buyerID {
		return ErrTrustSelf
	}
	_, err := ex.ExecContext(ctx, `INSERT INTO trusted_buyers (seller_id, buyer_id, via) VALUES ($1, $2, $3)
		ON CONFLICT (seller_id, buyer_id) DO NOTHING`, sellerID, buyerID, via)
	return err
}

// AddTrusted adds a buyer by username.
func (s *Service) AddTrusted(ctx context.Context, sellerID, username string) error {
	var buyerID string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE lower(username) = lower($1) AND status = 'active'`,
		strings.TrimPrefix(strings.TrimSpace(username), "@")).Scan(&buyerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnknownBuyer
	}
	if err != nil {
		return err
	}
	if buyerID == sellerID {
		return &FieldError{"username", "you can't add yourself"}
	}
	return s.trust(ctx, s.DB, sellerID, buyerID, "list")
}

// RemoveTrusted takes a buyer off the list. Orders already placed keep how
// they were agreed to ship.
func (s *Service) RemoveTrusted(ctx context.Context, sellerID, username string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM trusted_buyers t USING users u
		WHERE t.buyer_id = u.id AND t.seller_id = $1 AND lower(u.username) = lower($2)`, sellerID, username)
	return err
}

// InviteSeller returns whose invite link this is.
func (s *Service) InviteSeller(ctx context.Context, code string) (sellerID, username string, err error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "", "", ErrBadInvite
	}
	err = s.DB.QueryRowContext(ctx, `SELECT u.id, u.username FROM seller_profiles sp JOIN users u ON u.id = sp.user_id
		WHERE sp.trust_invite = $1 AND u.status = 'active'`, code).Scan(&sellerID, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrBadInvite
	}
	return sellerID, username, err
}

// AcceptInvite puts the buyer on the seller's trusted list.
func (s *Service) AcceptInvite(ctx context.Context, buyerID, code string) (string, error) {
	sellerID, username, err := s.InviteSeller(ctx, code)
	if err != nil {
		return "", err
	}
	if err := s.trust(ctx, s.DB, sellerID, buyerID, "invite"); err != nil {
		return "", err
	}
	_ = inbox.Notify(ctx, s.DB, sellerID, "account", "New trusted buyer",
		"A buyer accepted your trusted buyer invite. See Sell → Trusted buyers.", "/sell/trusted")
	return username, nil
}

// envelopeOrder loads what's needed to decide whether an order can switch
// to an envelope.
type envelopeOrder struct {
	id, buyer, seller, status, method string
	number                            int64
	total, cards                      int
	graded                            bool
	preset                            string
	offer                             sql.NullString
	test                              bool
}

func (s *Service) loadEnvelopeOrder(ctx context.Context, tx *sql.Tx, where, userID, orderID string) (envelopeOrder, error) {
	var o envelopeOrder
	err := tx.QueryRowContext(ctx, `SELECT o.id, o.buyer_id, o.seller_id, o.status, o.ship_method, o.number, o.total_cents, o.envelope_offer, o.test,
			(SELECT coalesce(sum(oi.quantity), 0) FROM order_items oi WHERE oi.order_id = o.id),
			EXISTS (SELECT 1 FROM order_items oi JOIN listings l ON l.id = oi.listing_id WHERE oi.order_id = o.id AND l.grader IS NOT NULL),
			coalesce((SELECT l.package_preset FROM order_items oi JOIN listings l ON l.id = oi.listing_id WHERE oi.order_id = o.id LIMIT 1), '')
		FROM orders o WHERE (o.id::text = $2 OR o.number::text = $2) AND `+where+` = $1 FOR UPDATE OF o`, userID, orderID).
		Scan(&o.id, &o.buyer, &o.seller, &o.status, &o.method, &o.number, &o.total, &o.offer, &o.test, &o.cards, &o.graded, &o.preset)
	if errors.Is(err, sql.ErrNoRows) {
		return o, ErrNotFound
	}
	return o, err
}

// OfferEnvelope: after a sale, the seller trusts the buyer and offers to
// ship in a stamped envelope. The order switches only if the buyer accepts.
func (s *Service) OfferEnvelope(ctx context.Context, sellerID, orderID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := s.loadEnvelopeOrder(ctx, tx, "o.seller_id", sellerID, orderID)
	if err != nil {
		return err
	}
	if o.status != "paid" || o.method != "label" {
		return ErrState
	}
	if o.offer.Valid {
		return ErrAlreadyOffered
	}
	if !TrustedEnvelopeEligible(o.cards, o.total, o.graded, o.preset) {
		return ErrNoEnvelope
	}
	if has, err := s.activeLabel(ctx, o.id); err != nil || has {
		if err == nil {
			err = ErrHasLabel
		}
		return err
	}
	if err := s.trust(ctx, tx, sellerID, o.buyer, "order"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET envelope_offer = 'offered', updated_at = now() WHERE id = $1`, o.id); err != nil {
		return err
	}
	if err := inbox.Notify(ctx, tx, o.buyer, "order", fmt.Sprintf("Order #%d: ship in an envelope?", o.number),
		"Your seller added you as a trusted buyer and offered to send this card in a stamped envelope instead of a tracked package. "+
			"Envelopes have no delivery confirmation, and a missing envelope isn't covered by Slabs & Singles. Open the order to accept or decline.",
		"/purchases/"+o.id); err != nil {
		return err
	}
	return tx.Commit()
}

// AnswerEnvelope is the buyer accepting or declining the seller's offer.
func (s *Service) AnswerEnvelope(ctx context.Context, buyerID, orderID string, accept bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := s.loadEnvelopeOrder(ctx, tx, "o.buyer_id", buyerID, orderID)
	if err != nil {
		return err
	}
	if !o.offer.Valid || o.offer.String != "offered" {
		return ErrOfferNotOpen
	}
	if o.status != "paid" || o.method != "label" {
		return ErrState // shipped already, or a label was bought meanwhile
	}
	if has, err := s.activeLabel(ctx, o.id); err != nil || has {
		if err == nil {
			err = ErrState
		}
		return err
	}
	answer, title, body := "declined", "declined the envelope", "Ship it in a tracked package as usual."
	if accept {
		answer, title, body = "accepted", "accepted the envelope", "Print the tracked envelope from the order, add a stamp and mail it."
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET ship_method = 'envelope', envelope_trusted = true WHERE id = $1`, o.id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET envelope_offer = $2, updated_at = now() WHERE id = $1`, o.id, answer); err != nil {
		return err
	}
	if err := inbox.Notify(ctx, tx, o.seller, "order", fmt.Sprintf("Order #%d: buyer %s", o.number, title), body,
		"/sell/orders/"+o.id); err != nil {
		return err
	}
	return tx.Commit()
}
