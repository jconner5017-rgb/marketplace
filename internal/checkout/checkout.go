// Package checkout turns a seller's part of the cart, or cards held for a
// buyer (auction wins and accepted offers), into an order paid through
// Stripe Checkout on the seller's own account, with our fee taken as an
// application fee. Payment is confirmed when the buyer returns from Stripe;
// the webhook and a periodic sweep are backups.
package checkout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/jconner5017-rgb/marketplace/internal/blocklist"
	"github.com/jconner5017-rgb/marketplace/internal/fees"
	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
	"github.com/jconner5017-rgb/marketplace/internal/shopping"
	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

var (
	ErrDisabled     = errors.New("checkout isn't switched on yet")
	ErrNotFound     = errors.New("order not found")
	ErrEmpty        = errors.New("there's nothing to pay for from this seller")
	ErrChanged      = errors.New("some cards in your cart changed or sold; review your cart and try again")
	ErrSellerClosed = errors.New("this seller can't take payments right now")
	ErrOwn          = errors.New("you can't buy your own cards")
	ErrNotPaid      = errors.New("payment hasn't gone through yet")
	ErrAlreadyPaid  = errors.New("you've already paid for these cards; see Purchases")
)

// SessionTTL is how long the buyer has on Stripe's page (Stripe's minimum is 30 minutes).
const SessionTTL = 30 * time.Minute

type Service struct {
	DB      *sql.DB
	Sellers *sellers.Service
	Fees    *fees.Service
	Pay     stripe.Payments     // nil when Stripe isn't configured
	Rep     *sellers.Reputation // re-checks a seller's tier after feedback or a chargeback
	// Labels: the shipping provider and the platform's own Stripe account
	// (sellers' saved cards). Either nil = label buying is off.
	Ship     shipping.Provider
	Platform stripe.Platform
	Enabled  bool   // CHECKOUT_ENABLED
	WebURL   string // where Stripe sends the buyer back, e.g. https://slabsnsingles.com
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Ready reports whether buyers can pay.
func (s *Service) Ready() bool { return s.Enabled && s.Pay != nil }

// Request picks what to pay for: one seller's cards in the cart, or held cards.
type Request struct {
	Seller  string   `json:"seller"`   // seller username, for the cart
	HoldIDs []string `json:"hold_ids"` // auction wins / accepted offers, all from one seller
}

type line struct {
	listingID string
	version   int
	holdID    sql.NullString
	source    string
	title     string
	photo     string
	unit      int
	qty       int
	envelope  bool // raw card set up to ship in a plain envelope
}

// Start creates the order, sets the cards aside and opens a Stripe Checkout
// Session. It returns the order id and the Stripe page to send the buyer to.
func (s *Service) Start(ctx context.Context, buyerID, buyerEmail string, req Request) (string, string, error) {
	if !s.Ready() {
		return "", "", ErrDisabled
	}
	return s.start(ctx, buyerID, buyerEmail, req, false)
}

// TestMode reports whether admins can place test orders: only while real
// checkout is off, so it ends by itself when CHECKOUT_ENABLED goes on.
func (s *Service) TestMode() bool { return !s.Ready() }

// TestAddress is where test orders "ship".
var TestAddress = map[string]string{"name": "TEST ORDER (no payment taken)", "line1": "1 Test Way", "line2": "",
	"city": "New York", "state": "NY", "zip": "10001", "country": "US"}

// StartTest places an order exactly like Start but skips Stripe and marks it
// paid at once, flagged as a test. The caller must check the buyer is an admin.
func (s *Service) StartTest(ctx context.Context, buyerID, buyerEmail string, req Request) (string, string, error) {
	if !s.TestMode() {
		return "", "", ErrDisabled
	}
	return s.start(ctx, buyerID, buyerEmail, req, true)
}

func (s *Service) start(ctx context.Context, buyerID, buyerEmail string, req Request, test bool) (string, string, error) {
	// A buyer who comes back from Stripe without finishing and presses Pay
	// again: close the earlier checkout first so its cards are free.
	if err := s.closeEarlier(ctx, buyerID, req); err != nil {
		return "", "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()

	var sellerID string
	var lines []line
	var subtotal, discount int
	if len(req.HoldIDs) > 0 {
		sellerID, lines, err = s.holdLines(ctx, tx, buyerID, req.HoldIDs)
	} else {
		sellerID, lines, err = s.cartLines(ctx, tx, buyerID, req.Seller)
	}
	if err != nil {
		return "", "", err
	}
	if sellerID == buyerID {
		return "", "", ErrOwn
	}

	var acct string
	var ok, collectTax bool
	err = tx.QueryRowContext(ctx, `
		SELECT sp.stripe_account_id, u.status = 'active' AND sp.charges_enabled AND sp.review_status <> 'rejected', sp.collect_tax
		FROM seller_profiles sp JOIN users u ON u.id = sp.user_id WHERE sp.user_id = $1`, sellerID).Scan(&acct, &ok, &collectTax)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!ok || acct == "")) {
		return "", "", ErrSellerClosed
	}
	if err != nil {
		return "", "", err
	}

	cards := 0
	for _, l := range lines {
		subtotal += l.unit * l.qty
		cards += l.qty
	}
	if len(req.HoldIDs) == 0 {
		if p, err := s.Sellers.Get(ctx, sellerID); err == nil {
			discount, _ = shopping.BundleDiscount(p.BundleDiscountType, p.BundleDiscountVal, cards, subtotal)
		}
	}
	total := subtotal - discount

	sched, err := s.Fees.At(ctx, s.now())
	if err != nil {
		sched = fees.Default
	}
	fee := 0
	for _, l := range lines {
		fee += sched.For(l.unit) * l.qty
	}
	fee = min(fee, total)
	var schedID any
	if sched.ID > 0 {
		schedID = sched.ID
	}
	// Same rule as the cart: a plain envelope only for one raw card at $20 or less.
	method := "label"
	if cards == 1 && total <= 2000 && lines[0].envelope {
		method = "envelope"
	}

	var orderID string
	var number int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO orders (buyer_id, seller_id, subtotal_cents, discount_cents, total_cents, fee_cents,
			fee_schedule_id, ship_method, stripe_account, test)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id, number`,
		buyerID, sellerID, subtotal, discount, total, fee, schedID, method, acct, test).Scan(&orderID, &number); err != nil {
		return "", "", err
	}
	for _, l := range lines {
		var ver any
		if l.version > 0 {
			ver = l.version
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO order_items (order_id, listing_id, listing_version, hold_id, source, title, unit_cents, quantity)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			orderID, l.listingID, ver, l.holdID, l.source, l.title, l.unit, l.qty); err != nil {
			return "", "", err
		}
		if l.source == "cart" {
			// Set the cards aside while the buyer pays; given back if they don't.
			if _, err := tx.ExecContext(ctx, `
				UPDATE listings SET quantity = quantity - $2,
					status = CASE WHEN quantity - $2 = 0 THEN 'sold_out' ELSE status END, updated_at = now()
				WHERE id = $1`, l.listingID, l.qty); err != nil {
				return "", "", err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}

	if test {
		sess := stripe.CheckoutSession{ID: "test_" + orderID, Status: "complete", PaymentStatus: "paid", AmountTotal: total}
		sess.ShippingDetails = &struct {
			Name    string         `json:"name"`
			Address stripe.Address `json:"address"`
		}{Name: TestAddress["name"], Address: stripe.Address{Line1: TestAddress["line1"], City: TestAddress["city"],
			State: TestAddress["state"], PostalCode: TestAddress["zip"], Country: TestAddress["country"]}}
		if err := s.markPaid(ctx, orderID, sess); err != nil {
			return "", "", err
		}
		return orderID, fmt.Sprintf("%s/checkout/done?order=%s&test=1", s.WebURL, orderID), nil
	}

	params := stripe.CheckoutParams{
		OrderID:       orderID,
		FeeCents:      fee,
		CustomerEmail: buyerEmail,
		SuccessURL:    fmt.Sprintf("%s/checkout/done?order=%s&session_id={CHECKOUT_SESSION_ID}", s.WebURL, orderID),
		CancelURL:     fmt.Sprintf("%s/checkout/done?order=%s&cancelled=1", s.WebURL, orderID),
		ExpiresAt:     s.now().Add(SessionTTL + time.Minute),
		AutomaticTax:  collectTax,
	}
	if discount > 0 {
		params.Lines = []stripe.LineItem{{
			Name:      fmt.Sprintf("Order #%d: %d cards (bundle discount applied)", number, cards),
			UnitCents: total, Quantity: 1,
		}}
	} else {
		for _, l := range lines {
			params.Lines = append(params.Lines, stripe.LineItem{Name: l.title, UnitCents: l.unit, Quantity: l.qty, ImageURL: l.photo})
		}
	}
	sess, err := s.Pay.CreateCheckoutSession(ctx, acct, params)
	if err != nil {
		_ = s.cancel(context.WithoutCancel(ctx), orderID, "Stripe couldn't start checkout")
		return "", "", fmt.Errorf("start stripe checkout: %w", err)
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE orders SET stripe_checkout_id = $2, updated_at = now() WHERE id = $1`,
		orderID, sess.ID); err != nil {
		return "", "", err
	}
	return orderID, sess.URL, nil
}

// closeEarlier ends the buyer's unfinished checkouts with the same seller
// (cart) or for the same held cards.
func (s *Service) closeEarlier(ctx context.Context, buyerID string, req Request) error {
	var q string
	var arg any
	if len(req.HoldIDs) > 0 {
		q = `SELECT DISTINCT o.id FROM orders o JOIN order_items oi ON oi.order_id = o.id
			WHERE o.buyer_id = $1 AND o.status = 'pending_payment' AND oi.hold_id::text = ANY($2)`
		arg = pq.Array(req.HoldIDs)
	} else {
		q = `SELECT o.id FROM orders o JOIN users su ON su.id = o.seller_id
			WHERE o.buyer_id = $1 AND o.status = 'pending_payment' AND lower(su.username) = lower($2)
			AND EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.source = 'cart')`
		arg = strings.TrimSpace(req.Seller)
	}
	ids, err := s.ids(ctx, q, buyerID, arg)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.Abandon(ctx, buyerID, id); err != nil {
			return err
		}
	}
	return nil
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

// Abandon ends an unpaid checkout the buyer backed out of: Stripe closes the
// session so it can't be paid, then the cards are released. If it turns out
// the buyer did pay, the payment is recorded and ErrAlreadyPaid returned.
func (s *Service) Abandon(ctx context.Context, buyerID, orderID string) error {
	var acct, cs, status string
	err := s.DB.QueryRowContext(ctx, `SELECT status, coalesce(stripe_account, ''), coalesce(stripe_checkout_id, '')
		FROM orders WHERE id::text = $1 AND buyer_id = $2`, orderID, buyerID).Scan(&status, &acct, &cs)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "pending_payment" {
		if status == "cancelled" {
			return nil
		}
		return ErrAlreadyPaid
	}
	if cs != "" && s.Pay != nil {
		if _, err := s.Pay.ExpireCheckoutSession(ctx, acct, cs); err != nil {
			sess, gerr := s.Pay.GetCheckoutSession(ctx, acct, cs)
			if gerr != nil {
				return fmt.Errorf("close earlier checkout: %w", err)
			}
			if sess.Status != "expired" {
				st, aerr := s.apply(ctx, orderID, sess)
				if st == "paid" {
					return ErrAlreadyPaid
				}
				if aerr != nil && !errors.Is(aerr, ErrNotPaid) {
					return aerr
				}
				return ErrNotPaid // complete but still clearing
			}
		}
	}
	return s.cancel(ctx, orderID, "Buyer left checkout")
}

func (s *Service) cartLines(ctx context.Context, tx *sql.Tx, buyerID, seller string) (string, []line, error) {
	seller = strings.TrimSpace(seller)
	if seller == "" {
		return "", nil, ErrEmpty
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT l.id, l.seller_id, l.version, l.title, coalesce(l.photo_urls[1], ''), l.price_cents, c.quantity,
		       l.quantity, l.status = 'active' AND l.format = 'fixed', l.grader IS NULL AND l.package_preset = 'envelope'
		FROM cart_items c
		JOIN listings l ON l.id = c.listing_id
		JOIN users su ON su.id = l.seller_id
		WHERE c.user_id = $1 AND lower(su.username) = lower($2)
		ORDER BY c.added_at
		FOR UPDATE OF l`, buyerID, seller)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var sellerID string
	var out []line
	changed := false
	for rows.Next() {
		var l line
		var have int
		var open bool
		if err := rows.Scan(&l.listingID, &sellerID, &l.version, &l.title, &l.photo, &l.unit, &l.qty, &have, &open, &l.envelope); err != nil {
			return "", nil, err
		}
		if !open || l.qty > have {
			changed = true
			continue
		}
		l.source = "cart"
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if changed {
		return "", nil, ErrChanged
	}
	if len(out) == 0 {
		return "", nil, ErrEmpty
	}
	return sellerID, out, nil
}

func (s *Service) holdLines(ctx context.Context, tx *sql.Tx, buyerID string, ids []string) (string, []line, error) {
	if len(ids) > 50 {
		return "", nil, ErrEmpty
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT h.id, h.seller_id, h.listing_id, l.version, l.title, coalesce(l.photo_urls[1], ''), h.unit_cents,
		       h.quantity, h.source, l.grader IS NULL AND l.package_preset = 'envelope'
		FROM holds h JOIN listings l ON l.id = h.listing_id
		WHERE h.buyer_id = $1 AND h.id::text = ANY($2) AND h.status = 'awaiting_payment' AND h.due_at > now()
		  AND NOT EXISTS (SELECT 1 FROM order_items oi JOIN orders o ON o.id = oi.order_id
		                  WHERE oi.hold_id = h.id AND o.status = 'pending_payment')
		ORDER BY h.created_at
		FOR UPDATE OF h`, buyerID, pq.Array(ids))
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var sellerID string
	var out []line
	for rows.Next() {
		var l line
		var seller string
		if err := rows.Scan(&l.holdID, &seller, &l.listingID, &l.version, &l.title, &l.photo, &l.unit, &l.qty, &l.source, &l.envelope); err != nil {
			return "", nil, err
		}
		if sellerID != "" && seller != sellerID {
			return "", nil, errors.New("pay for one seller's cards at a time")
		}
		sellerID = seller
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if len(out) != len(ids) {
		return "", nil, ErrChanged
	}
	return sellerID, out, nil
}

// Confirm checks with Stripe when the buyer comes back and records the
// payment. It's safe to call more than once.
func (s *Service) Confirm(ctx context.Context, buyerID, orderID, sessionID string) (string, error) {
	var status, acct, cs string
	err := s.DB.QueryRowContext(ctx, `
		SELECT status, coalesce(stripe_account, ''), coalesce(stripe_checkout_id, '')
		FROM orders WHERE id::text = $1 AND buyer_id = $2`, orderID, buyerID).Scan(&status, &acct, &cs)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if status != "pending_payment" {
		return status, nil
	}
	if s.Pay == nil || cs == "" || (sessionID != "" && sessionID != cs) {
		return status, ErrNotPaid
	}
	sess, err := s.Pay.GetCheckoutSession(ctx, acct, cs)
	if err != nil {
		return status, err
	}
	return s.apply(ctx, orderID, sess)
}

// apply records what Stripe says about a session.
func (s *Service) apply(ctx context.Context, orderID string, sess stripe.CheckoutSession) (string, error) {
	switch {
	case sess.Status == "complete" && sess.PaymentStatus == "paid":
		if err := s.markPaid(ctx, orderID, sess); err != nil {
			return "", err
		}
		return "paid", nil
	case sess.Status == "expired":
		if err := s.cancel(ctx, orderID, "Checkout expired before payment"); err != nil {
			return "", err
		}
		return "cancelled", nil
	}
	return "pending_payment", ErrNotPaid
}

// ShipBy is three business days after payment, end of day Eastern.
func ShipBy(paid time.Time) time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	d := paid.In(loc)
	for n := 0; n < 3; {
		d = d.AddDate(0, 0, 1)
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n++
		}
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 0, 0, loc)
}

func (s *Service) markPaid(ctx context.Context, orderID string, sess stripe.CheckoutSession) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	shipTo := []byte("{}")
	if sd := sess.ShippingDetails; sd != nil {
		shipTo, _ = json.Marshal(map[string]string{
			"name": sd.Name, "line1": sd.Address.Line1, "line2": sd.Address.Line2, "city": sd.Address.City,
			"state": sd.Address.State, "zip": sd.Address.PostalCode, "country": sd.Address.Country,
		})
	}
	now := s.now()
	var buyer, seller string
	var number int64
	var total, fee, discount int
	err = tx.QueryRowContext(ctx, `
		UPDATE orders SET status = 'paid', paid_at = $2, ship_by = $3, ship_to = $4,
			stripe_payment_intent = nullif($5, ''), tax_cents = greatest($6 - total_cents, 0), updated_at = now()
		WHERE id = $1 AND status = 'pending_payment'
		RETURNING buyer_id, seller_id, number, total_cents, fee_cents, discount_cents`,
		orderID, now, ShipBy(now), string(shipTo), sess.PaymentIntent, sess.AmountTotal).Scan(&buyer, &seller, &number, &total, &fee, &discount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // already recorded
	}
	if err != nil {
		return err
	}
	ref := sess.PaymentIntent
	if ref == "" {
		ref = sess.ID
	}
	note := "Paid through Stripe Checkout"
	if strings.HasPrefix(ref, "test_") {
		note = "TEST ORDER: no payment taken"
	}
	paid := total
	if tax := sess.AmountTotal - total; tax > 0 {
		paid += tax
		note += fmt.Sprintf(" (includes %s sales tax)", money(tax))
	}
	if err := ledger(ctx, tx, orderID, seller, "charge", paid, ref, note); err != nil {
		return err
	}
	if err := ledger(ctx, tx, orderID, seller, "application_fee", fee, ref, "Marketplace fee"); err != nil {
		return err
	}
	if discount > 0 {
		if err := ledger(ctx, tx, orderID, seller, "discount", discount, ref, "Multi-card discount"); err != nil {
			return err
		}
	}
	if err := s.flagBlockedAddress(ctx, tx, buyer, orderID, shipTo); err != nil {
		return err
	}
	if err := queueOrderPaid(ctx, tx, orderID, seller, number, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE holds SET status = 'paid', updated_at = now()
		WHERE id IN (SELECT hold_id FROM order_items WHERE order_id = $1 AND hold_id IS NOT NULL)`, orderID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cart_items WHERE user_id = $2
		AND listing_id IN (SELECT listing_id FROM order_items WHERE order_id = $1 AND source = 'cart')`, orderID, buyer); err != nil {
		return err
	}
	link := "/sell/orders/" + orderID
	if err := inbox.Notify(ctx, tx, seller, "order", fmt.Sprintf("New order #%d: %s", number, money(total)),
		"Payment received. Ship by "+ShipBy(now).Format("Mon Jan 2")+".", link); err != nil {
		return err
	}
	if err := inbox.Notify(ctx, tx, buyer, "order", fmt.Sprintf("Order #%d confirmed", number),
		"Thanks! The seller has been told to ship it.", "/purchases/"+orderID); err != nil {
		return err
	}
	return tx.Commit()
}

// cancel ends an unpaid order and gives cart cards back to the listing.
// Held cards stay held for the buyer until their pay-by time.
// flagBlockedAddress raises a fraud alert when an order ships to a banned member's address.
func (s *Service) flagBlockedAddress(ctx context.Context, tx *sql.Tx, buyerID, orderID string, shipTo []byte) error {
	var a struct{ Line1, Zip string }
	_ = json.Unmarshal(shipTo, &a)
	hit, banned, err := blocklist.Has(ctx, tx, "address", blocklist.NormalizeAddress(a.Line1, a.Zip))
	if err != nil || !hit || banned == buyerID {
		return err
	}
	return blocklist.Alert(ctx, tx, buyerID, "ship-to address", banned, "order "+orderID)
}

func (s *Service) cancel(ctx context.Context, orderID, reason string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = 'cancelled', cancelled_at = now(), cancel_reason = $2, updated_at = now()
		WHERE id = $1 AND status = 'pending_payment'`, orderID, reason)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE listings l SET quantity = l.quantity + oi.quantity,
			status = CASE WHEN l.status = 'sold_out' THEN 'active' ELSE l.status END, updated_at = now()
		FROM order_items oi WHERE oi.order_id = $1 AND oi.source = 'cart' AND oi.listing_id = l.id`, orderID); err != nil {
		return err
	}
	return tx.Commit()
}

// HandleEvent applies a checkout.session.* webhook event. It reports
// whether the event was one of ours.
func (s *Service) HandleEvent(ctx context.Context, ev stripe.Event) (bool, error) {
	if ev.Type != "checkout.session.completed" && ev.Type != "checkout.session.expired" &&
		ev.Type != "checkout.session.async_payment_succeeded" {
		return false, nil
	}
	var sess stripe.CheckoutSession
	if err := json.Unmarshal(ev.Data.Object, &sess); err != nil {
		return true, err
	}
	sess.Normalize()
	var orderID, acct string
	err := s.DB.QueryRowContext(ctx, `SELECT id, coalesce(stripe_account, '') FROM orders WHERE stripe_checkout_id = $1`, sess.ID).
		Scan(&orderID, &acct)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil // not an order of ours
	}
	if err != nil {
		return true, err
	}
	if ev.Account != "" && ev.Account != acct {
		return true, nil
	}
	_, err = s.apply(ctx, orderID, sess)
	if errors.Is(err, ErrNotPaid) {
		err = nil
	}
	return true, err
}

// RunJobs sweeps unpaid orders and unpaid holds. main calls it every 30 seconds.
func (s *Service) RunJobs(ctx context.Context) error {
	if err := s.sweepOrders(ctx); err != nil {
		return err
	}
	if !s.Ready() {
		// Buyers can't pay yet, so nobody is late: keep at least two days
		// on every hold until checkout is switched on.
		_, err := s.DB.ExecContext(ctx, `UPDATE holds SET due_at = now() + interval '2 days', updated_at = now()
			WHERE status = 'awaiting_payment' AND due_at < now() + interval '2 days'`)
		return err
	}
	_, err := s.ExpireHolds(ctx)
	return err
}

// sweepOrders settles checkouts older than the session lifetime: asks Stripe
// first, in case the buyer paid but never came back.
func (s *Service) sweepOrders(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, coalesce(stripe_account, ''), coalesce(stripe_checkout_id, ''), created_at FROM orders
		WHERE status = 'pending_payment' AND created_at < $1 ORDER BY created_at LIMIT 100`,
		s.now().Add(-SessionTTL-5*time.Minute))
	if err != nil {
		return err
	}
	type pend struct {
		id, acct, cs string
		at           time.Time
	}
	var list []pend
	for rows.Next() {
		var p pend
		if err := rows.Scan(&p.id, &p.acct, &p.cs, &p.at); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	for _, p := range list {
		if s.Pay != nil && p.cs != "" {
			sess, err := s.Pay.GetCheckoutSession(ctx, p.acct, p.cs)
			if err == nil {
				if sess.Status == "open" || sess.Status == "complete" && sess.PaymentStatus != "paid" {
					continue // still on Stripe's page, or a bank payment clearing
				}
				if _, err := s.apply(ctx, p.id, sess); err != nil && !errors.Is(err, ErrNotPaid) {
					return err
				}
				continue
			}
			if s.now().Sub(p.at) < 24*time.Hour {
				continue // Stripe unreachable; try again later
			}
		}
		if err := s.cancel(ctx, p.id, "Checkout expired before payment"); err != nil {
			return err
		}
	}
	return nil
}

// ExpireHolds ends holds the buyer didn't pay for in time. The buyer gets a
// strike; accepted-offer cards go back on sale; auction sellers are told
// they can relist.
func (s *Service) ExpireHolds(ctx context.Context) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT h.id FROM holds h
		WHERE h.status = 'awaiting_payment' AND h.due_at < now()
		  AND NOT EXISTS (SELECT 1 FROM order_items oi JOIN orders o ON o.id = oi.order_id
		                  WHERE oi.hold_id = h.id AND o.status = 'pending_payment')
		LIMIT 100`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		ok, err := s.expireHold(ctx, id)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

func (s *Service) expireHold(ctx context.Context, id string) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var listingID, buyer, seller, source, title string
	var qty int
	err = tx.QueryRowContext(ctx, `
		UPDATE holds h SET status = 'expired', updated_at = now()
		FROM listings l
		WHERE h.id = $1 AND h.status = 'awaiting_payment' AND l.id = h.listing_id
		RETURNING h.listing_id, h.buyer_id, h.seller_id, h.source, h.quantity, l.title`, id).
		Scan(&listingID, &buyer, &seller, &source, &qty, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET buyer_strikes = buyer_strikes + 1 WHERE id = $1`, buyer); err != nil {
		return false, err
	}
	sellerNote := "The buyer didn't pay in time. You can list the card again from Selling."
	if source == "offer" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE listings SET quantity = quantity + $2,
				status = CASE WHEN status = 'sold_out' THEN 'active' ELSE status END, updated_at = now()
			WHERE id = $1`, listingID, qty); err != nil {
			return false, err
		}
		sellerNote = "The buyer didn't pay in time, so the card is back on sale."
	}
	if err := inbox.Notify(ctx, tx, buyer, "order", "Payment window closed: "+title,
		"You didn't pay in time, so the card was released. Unpaid items count against your account.", "/purchases"); err != nil {
		return false, err
	}
	if err := inbox.Notify(ctx, tx, seller, "order", "Unpaid: "+title, sellerNote, "/item/"+listingID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func money(c int) string { return "$" + strconv.Itoa(c/100) + "." + fmt.Sprintf("%02d", c%100) }
