package checkout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/postage"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

// PostageService marks an envelope shipped with postage bought here.
const PostageService = "First-Class letter (postage)"

// PostageProvider is the name stored on shipments and rates.
const PostageProvider = "easypost"

// Postage top-ups a seller can choose, in cents.
const (
	TopUpMinCents = 500
	TopUpMaxCents = 20000
)

var (
	ErrPostageOff  = errors.New("envelope postage isn't set up yet")
	ErrLowBalance  = &FieldError{"pay", "your postage balance is too low; add funds or pay with your card"}
	ErrPostageOver = errors.New("that postage is more than 30 days old, so USPS won't refund it")
)

// PostageBuyer buys USPS letter postage (EasyPost).
type PostageBuyer interface {
	LetterRates(ctx context.Context, l postage.Letter) (postage.Shipment, error)
	Buy(ctx context.Context, shipmentID, rateID string) (postage.Label, error)
	Refund(ctx context.Context, shipmentID string) error
	Track(ctx context.Context, shipmentID string) (postage.Tracking, error)
	TestMode() bool
}

// PostageQuote is what an envelope's postage costs the seller.
type PostageQuote struct {
	RateID          string  `json:"rate_id"`
	CostCents       int     `json:"cost_cents"`        // paid from the balance
	CardChargeCents int     `json:"card_charge_cents"` // paid by card (adds card processing)
	BalanceCents    int     `json:"balance_cents"`
	Card            string  `json:"card"` // "Visa ending 4242", or "" when none is saved
	WeightOz        float64 `json:"weight_oz"`
	Rigid           bool    `json:"rigid"`
	TestMode        bool    `json:"test_mode"` // Test key: VOID labels, nothing is charged
}

// PostageCredit is one change to a seller's postage balance.
type PostageCredit struct {
	Kind         string    `json:"kind"`
	AmountCents  int       `json:"amount_cents"`
	BalanceAfter int       `json:"balance_after"`
	OrderNumber  *int64    `json:"order_number"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

// PostageAccount is a seller's postage balance and recent history.
type PostageAccount struct {
	Enabled      bool            `json:"enabled"`
	TestMode     bool            `json:"test_mode"`
	BalanceCents int             `json:"balance_cents"`
	Card         string          `json:"card"`
	MinCents     int             `json:"min_cents"`
	MaxCents     int             `json:"max_cents"`
	History      []PostageCredit `json:"history"`
}

// TopUpChargeCents is what a seller's card pays to add cents to the balance.
func TopUpChargeCents(cents int) int { return shipping.LabelChargeCents(cents) }

func (s *Service) postageTest() bool { return s.Postage != nil && s.Postage.TestMode() }

// PostageAccountFor returns the seller's balance and its last 25 changes.
func (s *Service) PostageAccountFor(ctx context.Context, sellerID string) (PostageAccount, error) {
	a := PostageAccount{Enabled: s.Postage != nil, TestMode: s.postageTest(), MinCents: TopUpMinCents, MaxCents: TopUpMaxCents,
		History: []PostageCredit{}}
	var card sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT postage_balance_cents, label_card FROM seller_profiles WHERE user_id = $1`, sellerID).
		Scan(&a.BalanceCents, &card)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a.Card = card.String
	rows, err := s.DB.QueryContext(ctx, `SELECT pc.kind, pc.amount_cents, pc.balance_after, o.number, pc.note, pc.created_at
		FROM postage_credits pc LEFT JOIN orders o ON o.id = pc.order_id
		WHERE pc.seller_id = $1 ORDER BY pc.created_at DESC LIMIT 25`, sellerID)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var c PostageCredit
		if err := rows.Scan(&c.Kind, &c.AmountCents, &c.BalanceAfter, &c.OrderNumber, &c.Note, &c.CreatedAt); err != nil {
			return a, err
		}
		a.History = append(a.History, c)
	}
	return a, rows.Err()
}

// credit changes a seller's balance by cents and records why. A spend that
// would take the balance below zero returns ErrLowBalance.
func credit(ctx context.Context, tx *sql.Tx, sellerID, kind string, cents int, orderID, stripeObj, note string) (int, error) {
	var bal int
	err := tx.QueryRowContext(ctx, `UPDATE seller_profiles SET postage_balance_cents = postage_balance_cents + $2, updated_at = now()
		WHERE user_id = $1 AND postage_balance_cents + $2 >= 0 RETURNING postage_balance_cents`, sellerID, cents).Scan(&bal)
	if errors.Is(err, sql.ErrNoRows) {
		if cents < 0 {
			return 0, ErrLowBalance
		}
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var oid, obj any
	if orderID != "" {
		oid = orderID
	}
	if stripeObj != "" {
		obj = stripeObj
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO postage_credits (seller_id, kind, amount_cents, balance_after, order_id, stripe_object, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, sellerID, kind, cents, bal, oid, obj, note)
	return bal, err
}

func (s *Service) creditTx(ctx context.Context, sellerID, kind string, cents int, orderID, stripeObj, note string) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	bal, err := credit(ctx, tx, sellerID, kind, cents, orderID, stripeObj, note)
	if err != nil {
		return 0, err
	}
	return bal, tx.Commit()
}

// TopUpPostage charges the seller's saved card and adds cents to their
// postage balance. The card pays the amount plus card processing.
func (s *Service) TopUpPostage(ctx context.Context, sellerID string, cents int) (int, error) {
	if s.Platform == nil || s.Postage == nil {
		return 0, ErrPostageOff
	}
	if cents < TopUpMinCents || cents > TopUpMaxCents {
		return 0, &FieldError{"amount_cents", fmt.Sprintf("add %s to %s", money(TopUpMinCents), money(TopUpMaxCents))}
	}
	var cus, pm sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT stripe_customer_id, label_payment_method FROM seller_profiles WHERE user_id = $1`, sellerID).
		Scan(&cus, &pm)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if cus.String == "" || pm.String == "" {
		return 0, ErrNoCard
	}
	charge := TopUpChargeCents(cents)
	pi, err := s.Platform.Charge(ctx, cus.String, pm.String, charge, "Slabs & Singles postage balance",
		map[string]string{"seller_id": sellerID, "postage_cents": fmt.Sprint(cents)})
	if err != nil {
		return 0, &FieldError{"card", "your card was declined: " + err.Error()}
	}
	bal, err := s.creditTx(ctx, sellerID, "topup", cents, "", pi,
		fmt.Sprintf("Added %s (card charged %s with processing)", money(cents), money(charge)))
	if err != nil {
		if rerr := s.Platform.RefundPlatform(context.WithoutCancel(ctx), pi); rerr != nil {
			return 0, fmt.Errorf("saving the top-up failed (%v) and the card refund failed too: %w", err, rerr)
		}
		return 0, err
	}
	return bal, nil
}

// envelopeWeight is the order's weight in ounces, at least 1.
func envelopeWeight(items []shipping.Item) float64 {
	w := 0.0
	for _, it := range items {
		q := it.Quantity
		if q < 1 {
			q = 1
		}
		w += it.WeightOz * float64(q)
	}
	if w < 1 {
		w = 1
	}
	return math.Ceil(w*10) / 10
}

func (s *Service) postageOrder(ctx context.Context, sellerID, orderID string) (labelOrder, error) {
	if s.Postage == nil {
		return labelOrder{}, ErrPostageOff
	}
	o, err := s.loadLabelOrder(ctx, sellerID, orderID)
	if err != nil {
		return o, err
	}
	if o.method != "envelope" {
		return o, &FieldError{"order", "this order ships as a package; buy a label instead"}
	}
	if o.status != "paid" {
		return o, ErrState
	}
	if o.from == nil {
		return o, ErrNoShipFrom
	}
	if has, err := s.activeLabel(ctx, o.id); err != nil || has {
		if err == nil {
			err = &FieldError{"order", "this order already has postage; void it first to buy again"}
		}
		return o, err
	}
	return o, nil
}

// QuotePostage gets the USPS First-Class letter price for an envelope order.
// Rigid envelopes (a toploader that won't bend) pay the non-machinable price.
func (s *Service) QuotePostage(ctx context.Context, sellerID, orderID string, rigid bool) (PostageQuote, error) {
	o, err := s.postageOrder(ctx, sellerID, orderID)
	if err != nil {
		return PostageQuote{}, err
	}
	w := envelopeWeight(o.items)
	if w > postage.MaxOz("Letter") {
		return PostageQuote{}, &FieldError{"order", fmt.Sprintf("this order weighs about %g oz, more than a letter can (3.5 oz); ship it as a package", w)}
	}
	sh, err := s.Postage.LetterRates(ctx, postage.Letter{From: *o.from, To: o.shipTo, Package: "Letter", WeightOz: w,
		LabelSize: "7x3", Format: "PNG", NonMachinable: rigid, Reference: fmt.Sprintf("Order %d", o.number)})
	if err != nil {
		return PostageQuote{}, err
	}
	r, ok := sh.FirstClass()
	if !ok {
		why := ""
		for _, m := range sh.Messages {
			if strings.EqualFold(m.Carrier, "USPS") && m.Message != "" {
				why = ": " + m.Message
				break
			}
		}
		return PostageQuote{}, fmt.Errorf("USPS didn't offer First-Class letter postage for this envelope%s", why)
	}
	cents, err := r.Cents()
	if err != nil {
		return PostageQuote{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO label_rates (rate_id, order_id, provider, carrier, service, amount_cents, shipment_id)
		VALUES ($1, $2, $3, 'USPS', 'First', $4, $5) ON CONFLICT (rate_id) DO NOTHING`, r.ID, o.id, PostageProvider, cents, sh.ID); err != nil {
		return PostageQuote{}, err
	}
	q := PostageQuote{RateID: r.ID, CostCents: cents, CardChargeCents: shipping.LabelChargeCents(cents), WeightOz: w, Rigid: rigid,
		TestMode: s.Postage.TestMode()}
	var card sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT postage_balance_cents, label_card FROM seller_profiles WHERE user_id = $1`, sellerID).
		Scan(&q.BalanceCents, &card); err != nil {
		return q, err
	}
	q.Card = card.String
	return q, nil
}

// BoughtPostage is the result of buying envelope postage.
type BoughtPostage struct {
	Tracking     string `json:"tracking_number"`
	ImageURL     string `json:"image_url"`
	PaidBy       string `json:"paid_by"`
	CostCents    int    `json:"cost_cents"`
	ChargeCents  int    `json:"charge_cents"`
	BalanceCents int    `json:"balance_cents"`
}

// BuyPostage buys a quoted rate, paid from the balance ("balance") or the
// saved card ("card"), and marks the order shipped. With a Test key the
// label is a VOID sample and nothing is charged.
func (s *Service) BuyPostage(ctx context.Context, sellerID, orderID, rateID, pay string) (BoughtPostage, error) {
	o, err := s.postageOrder(ctx, sellerID, orderID)
	if err != nil {
		return BoughtPostage{}, err
	}
	var cents int
	var shipmentID string
	err = s.DB.QueryRowContext(ctx, `SELECT amount_cents, coalesce(shipment_id, '') FROM label_rates
		WHERE rate_id = $1 AND order_id = $2 AND provider = $3 AND created_at > now() - interval '12 hours'`,
		rateID, o.id, PostageProvider).Scan(&cents, &shipmentID)
	if errors.Is(err, sql.ErrNoRows) || shipmentID == "" {
		return BoughtPostage{}, ErrRateExpired
	}
	if err != nil {
		return BoughtPostage{}, err
	}
	b := BoughtPostage{CostCents: cents, PaidBy: pay}
	note := fmt.Sprintf("Postage for order #%d", o.number)
	var pi string
	// undo gives the money back when EasyPost doesn't issue the postage.
	undo := func() error { return nil }
	switch {
	case s.Postage.TestMode():
		b.PaidBy = "test"
	case pay == "balance":
		if b.BalanceCents, err = s.creditTx(ctx, sellerID, "postage", -cents, o.id, "", note); err != nil {
			return b, err
		}
		b.ChargeCents = cents
		undo = func() error {
			_, err := s.creditTx(context.WithoutCancel(ctx), sellerID, "refund", cents, o.id, "", note+" wasn't issued; returned")
			return err
		}
	case pay == "card":
		if s.Platform == nil {
			return b, ErrNoLabels
		}
		if o.customer == "" || o.pm == "" {
			return b, ErrNoCard
		}
		b.ChargeCents = shipping.LabelChargeCents(cents)
		pi, err = s.Platform.Charge(ctx, o.customer, o.pm, b.ChargeCents, fmt.Sprintf("Envelope postage, order #%d", o.number),
			map[string]string{"order_id": o.id, "rate_id": rateID, "postage_cents": fmt.Sprint(cents)})
		if err != nil {
			return b, &FieldError{"card", "your card was declined: " + err.Error()}
		}
		undo = func() error { return s.Platform.RefundPlatform(context.WithoutCancel(ctx), pi) }
	default:
		return b, &FieldError{"pay", "choose your postage balance or your card"}
	}
	l, err := s.Postage.Buy(ctx, shipmentID, rateID)
	if err != nil {
		if uerr := undo(); uerr != nil {
			return b, fmt.Errorf("postage failed (%v) and giving the money back failed too: %w", err, uerr)
		}
		return b, fmt.Errorf("USPS postage wasn't issued (you weren't charged): %w", err)
	}
	b.Tracking, b.ImageURL = l.Tracking, l.ImageURL
	if b.ImageURL == "" {
		b.ImageURL = l.LabelURL
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return b, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO shipments (order_id, method, carrier, service, tracking_number, label_cost_cents, status,
		last_event, last_event_at, provider, provider_label_id, provider_rate_id, label_url, charge_cents, stripe_label_charge, postage_paid_by)
		VALUES ($1, 'envelope', 'USPS', $2, $3, $4, 'label_created', 'Postage printed', now(), $5, $6, $7, $8, $9, $10, $11)`,
		o.id, PostageService, nullable(l.Tracking), cents, PostageProvider, l.ShipmentID, rateID, b.ImageURL, b.ChargeCents,
		nullable(pi), b.PaidBy); err != nil {
		return b, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'shipped', shipped_at = now(), updated_at = now() WHERE id = $1`, o.id); err != nil {
		return b, err
	}
	if b.PaidBy != "test" {
		obj := pi
		if obj == "" {
			obj = "easypost:" + l.ShipmentID
		}
		if err := ledger(ctx, tx, o.id, sellerID, "label_cost", cents, obj,
			fmt.Sprintf("USPS First-Class letter postage (paid by %s)", b.PaidBy)); err != nil {
			return b, err
		}
	}
	body := "It's on the way in an envelope with USPS postage."
	if l.Tracking != "" {
		body += " Tracking: " + l.Tracking
	}
	if err := inbox.Notify(ctx, tx, o.buyer, "order", fmt.Sprintf("Order #%d shipped", o.number), body, "/purchases/"+o.id); err != nil {
		return b, err
	}
	return b, tx.Commit()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// voidPostage refunds unused envelope postage: back to the balance, the
// card, or nothing for Test postage.
func (s *Service) voidPostage(ctx context.Context, orderID, sellerID, shipID, labelID, paidBy, pi string, cents int, bought time.Time, reopen bool) error {
	if s.Postage == nil {
		return ErrPostageOff
	}
	if s.now().Sub(bought) > 30*24*time.Hour {
		return ErrPostageOver
	}
	if err := s.Postage.Refund(ctx, labelID); err != nil {
		return err
	}
	if paidBy == "card" && pi != "" {
		if s.Platform == nil {
			return ErrNoLabels
		}
		if err := s.Platform.RefundPlatform(ctx, pi); err != nil {
			return fmt.Errorf("postage voided, but refunding your card failed: %w", err)
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if paidBy == "balance" {
		if _, err := credit(ctx, tx, sellerID, "refund", cents, orderID, "", "Postage voided"); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE shipments SET status = 'voided', voided_at = now(), last_event = 'Postage voided',
		last_event_at = now(), updated_at = now() WHERE id = $1`, shipID); err != nil {
		return err
	}
	if reopen {
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'paid', shipped_at = NULL, updated_at = now()
			WHERE id = $1 AND status = 'shipped'`, orderID); err != nil {
			return err
		}
	}
	if paidBy != "test" {
		obj := pi
		if obj == "" {
			obj = "easypost:" + labelID
		}
		if err := ledger(ctx, tx, orderID, sellerID, "adjustment", cents, obj+":void", "Postage voided; refunded to "+paidBy); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RefreshPostageTracking reads EasyPost's tracker for envelopes sent with
// bought postage (every two hours each, for 45 days or until delivered) and
// records new scans through ApplyTracking, which tells the buyer when it
// moves, is out for delivery, is delivered or runs into a problem.
func (s *Service) RefreshPostageTracking(ctx context.Context) (int, error) {
	if s.Postage == nil {
		return 0, nil
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, provider_label_id, tracking_number, coalesce(last_event_at, created_at)
		FROM shipments
		WHERE provider = $1 AND voided_at IS NULL AND provider_label_id IS NOT NULL AND tracking_number IS NOT NULL
		  AND status IN ('label_created', 'in_transit', 'exception') AND created_at > now() - interval '45 days'
		  AND (scans_checked_at IS NULL OR scans_checked_at < now() - interval '110 minutes')
		ORDER BY scans_checked_at NULLS FIRST LIMIT 200`, PostageProvider)
	if err != nil {
		return 0, err
	}
	type job struct {
		id, shipment, tracking string
		since                  time.Time
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.shipment, &j.tracking, &j.since); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	changed := 0
	var firstErr error
	for _, j := range jobs {
		t, err := s.Postage.Track(ctx, j.shipment)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("tracking for shipment %s: %w", j.id, err)
			}
			continue
		}
		var fresh []shipping.TrackingEvent
		for _, e := range t.Events {
			if e.At.After(j.since) {
				e.TrackingNumber = j.tracking
				fresh = append(fresh, e)
			}
		}
		sort.SliceStable(fresh, func(a, b int) bool { return fresh[a].At.Before(fresh[b].At) })
		if len(fresh) > 0 {
			n, err := s.ApplyTracking(ctx, fresh)
			if err != nil {
				return changed, err
			}
			if n > 0 {
				changed++
			}
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE shipments SET scans_checked_at = now() WHERE id = $1`, j.id); err != nil {
			return changed, err
		}
	}
	return changed, firstErr
}
