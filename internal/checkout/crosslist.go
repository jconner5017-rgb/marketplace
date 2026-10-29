package checkout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/fcm"
	"github.com/jconner5017-rgb/marketplace/internal/inbox"
)

// queueOrderPaid tells the seller's inventory tool which SKUs sold, so it
// can end or revise the listing on other sites.
func queueOrderPaid(ctx context.Context, tx *sql.Tx, orderID, sellerID string, number int64, paidAt time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT l.external_id, oi.quantity, oi.unit_cents, oi.listing_id::text
		FROM order_items oi JOIN listings l ON l.id = oi.listing_id
		WHERE oi.order_id = $1 AND l.external_id IS NOT NULL`, orderID)
	if err != nil {
		return err
	}
	type item struct {
		ExternalID string `json:"external_id"`
		Quantity   int    `json:"quantity"`
		UnitCents  int    `json:"unit_cents"`
		ListingID  string `json:"listing_id"`
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ExternalID, &it.Quantity, &it.UnitCents, &it.ListingID); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()
	if len(items) == 0 {
		return nil
	}
	return fcm.Enqueue(ctx, tx, sellerID, "order.paid", "evt_order_paid_"+orderID, map[string]any{
		"order_id": orderID, "order_number": number, "paid_at": paidAt.UTC().Format(time.RFC3339), "items": items,
	})
}

// ReleaseListing stops unfinished checkouts that hold a listing taken down
// elsewhere (sold on another site): Stripe closes the page before the buyer
// can pay. A checkout already paid is left for the oversell rules.
func (s *Service) ReleaseListing(ctx context.Context, listingID string) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT o.id, o.buyer_id FROM orders o JOIN order_items oi ON oi.order_id = o.id
		WHERE oi.listing_id = $1 AND oi.source = 'cart' AND o.status = 'pending_payment'`, listingID)
	if err != nil {
		return err
	}
	type pend struct{ id, buyer string }
	var list []pend
	for rows.Next() {
		var p pend
		if err := rows.Scan(&p.id, &p.buyer); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	for _, p := range list {
		err := s.Abandon(ctx, p.buyer, p.id)
		if errors.Is(err, ErrAlreadyPaid) || errors.Is(err, ErrNotPaid) {
			continue
		}
		if err != nil {
			return err
		}
		if err := inbox.Notify(ctx, s.DB, p.buyer, "order", "A card in your checkout just sold",
			"It sold on another site before you paid, so you weren't charged. The rest of your cart is still there.", "/cart"); err != nil {
			return err
		}
	}
	return nil
}

var ErrNotOurs = errors.New("that SKU isn't on this order")

// Oversold settles a card that sold here and on another site. The later
// sale loses: if ours came second, it's refunded in full (with our fee) on
// the seller's Stripe account, reason oversold_cross_listing. It returns
// "refunded" or "kept" (ours was first; end the other sale instead).
func (s *Service) Oversold(ctx context.Context, sellerID, orderID, externalID string, soldElsewhere time.Time) (string, error) {
	var oid, buyer, acct, pi, status, listingID string
	var number int64
	var paid sql.NullTime
	var total, tax int
	err := s.DB.QueryRowContext(ctx, `
		SELECT o.id, o.number, o.buyer_id, coalesce(o.stripe_account, ''), coalesce(o.stripe_payment_intent, ''), o.status,
		       o.paid_at, o.total_cents, o.tax_cents, l.id
		FROM orders o JOIN order_items oi ON oi.order_id = o.id JOIN listings l ON l.id = oi.listing_id
		WHERE (o.id::text = $1 OR o.number::text = $1) AND o.seller_id = $2 AND l.external_id = $3`,
		orderID, sellerID, externalID).Scan(&oid, &number, &buyer, &acct, &pi, &status, &paid, &total, &tax, &listingID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotOurs
	}
	if err != nil {
		return "", err
	}
	if !paid.Valid {
		return "", ErrState
	}
	var prior string
	if err := s.DB.QueryRowContext(ctx, `SELECT outcome FROM oversells WHERE order_id = $1 AND listing_id = $2`, oid, listingID).Scan(&prior); err == nil {
		return prior, nil
	}
	outcome := "kept"
	if soldElsewhere.Before(paid.Time) {
		outcome = "refunded"
		if status != "paid" {
			return "", fmt.Errorf("%w: order #%d has already %s; sort it out with the buyer", ErrState, number, status)
		}
		if s.Pay == nil || pi == "" {
			return "", ErrDisabled
		}
		ref, err := s.Pay.Refund(ctx, acct, pi, map[string]string{"reason": "oversold_cross_listing", "order_id": oid})
		if err != nil {
			return "", fmt.Errorf("refund on Stripe: %w", err)
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return "", err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'refunded', refunded_cents = total_cents + tax_cents,
			cancel_reason = 'oversold_cross_listing', cancelled_at = now(), updated_at = now() WHERE id = $1`, oid); err != nil {
			return "", err
		}
		if err := ledger(ctx, tx, oid, sellerID, "refund", total+tax, ref.ID, "Sold on another site first (oversold_cross_listing)"); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO refund_reports (order_id, source, kind, amount_cents, stripe_ref, note, status)
			VALUES ($1, 'stripe', 'refund', $2, $3, 'Automatic refund: oversold_cross_listing', 'closed') ON CONFLICT DO NOTHING`,
			oid, total+tax, ref.ID); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO oversells (order_id, seller_id, listing_id, outcome, sold_elsewhere_at)
			VALUES ($1, $2, $3, 'refunded', $4)`, oid, sellerID, listingID, soldElsewhere); err != nil {
			return "", err
		}
		if err := inbox.Notify(ctx, tx, buyer, "order", fmt.Sprintf("Order #%d refunded", number),
			"Sorry, the card sold on another site a moment before your payment. You've been refunded in full.", "/purchases/"+oid); err != nil {
			return "", err
		}
		if err := inbox.Notify(ctx, tx, sellerID, "order", fmt.Sprintf("Order #%d refunded: sold elsewhere first", number),
			"The card sold on another site before this order, so the buyer was refunded automatically. Frequent oversells lower your seller level.",
			"/sell/orders/"+oid); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		if s.Rep != nil {
			if _, _, err := s.Rep.Review(ctx, sellerID); err != nil {
				return outcome, err
			}
		}
		return outcome, nil
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO oversells (order_id, seller_id, listing_id, outcome, sold_elsewhere_at)
		VALUES ($1, $2, $3, 'kept', $4) ON CONFLICT DO NOTHING`, oid, sellerID, listingID, soldElsewhere); err != nil {
		return "", err
	}
	return outcome, nil
}
