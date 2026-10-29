package checkout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

var (
	ErrNoLabels    = errors.New("label buying isn't set up yet")
	ErrNoShipFrom  = errors.New("add your ship-from address first")
	ErrNoCard      = errors.New("add a card for labels first")
	ErrHasLabel    = errors.New("this order already has a label; void it first to buy another")
	ErrRateExpired = errors.New("that rate has expired; get new rates")
	ErrLabelUsed   = errors.New("the label has already been scanned by the carrier, so it can't be voided")
)

// SignatureOver is the order total above which labels require a signature.
const SignatureOver = 75000

// LabelSettings is a seller's label setup.
type LabelSettings struct {
	ShipFrom *shipping.Address `json:"ship_from"`
	Card     string            `json:"card"`
	Enabled  bool              `json:"enabled"` // a provider is configured
}

func (s *Service) LabelSettings(ctx context.Context, sellerID string) (LabelSettings, error) {
	var st LabelSettings
	var from []byte
	var card sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT ship_from, label_card FROM seller_profiles WHERE user_id = $1`, sellerID).Scan(&from, &card)
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	if err != nil {
		return st, err
	}
	if len(from) > 0 {
		var a shipping.Address
		if json.Unmarshal(from, &a) == nil {
			st.ShipFrom = &a
		}
	}
	st.Card, st.Enabled = card.String, s.Ship != nil && s.Platform != nil
	return st, nil
}

// SetShipFrom checks the address with the carrier and saves it.
func (s *Service) SetShipFrom(ctx context.Context, sellerID string, a shipping.Address) (shipping.Address, error) {
	a.Country = "US"
	if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Street1) == "" {
		return a, &FieldError{"line1", "enter your name and street address"}
	}
	if s.Ship != nil {
		v, err := s.Ship.ValidateAddress(ctx, a)
		if errors.Is(err, shipping.ErrBadAddress) {
			return a, &FieldError{"line1", "the carrier couldn't verify that address; check it and try again"}
		}
		if err != nil {
			return a, err
		}
		a = v
	}
	b, _ := json.Marshal(a)
	_, err := s.DB.ExecContext(ctx, `UPDATE seller_profiles SET ship_from = $2, updated_at = now() WHERE user_id = $1`, sellerID, string(b))
	return a, err
}

// StartCardSetup opens Stripe's page for saving a card for labels.
func (s *Service) StartCardSetup(ctx context.Context, sellerID, email string) (string, error) {
	if s.Platform == nil {
		return "", ErrNoLabels
	}
	var cus sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT stripe_customer_id FROM seller_profiles WHERE user_id = $1`, sellerID).Scan(&cus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if !cus.Valid {
		id, err := s.Platform.CreateCustomer(ctx, email, sellerID)
		if err != nil {
			return "", err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE seller_profiles SET stripe_customer_id = $2 WHERE user_id = $1`, sellerID, id); err != nil {
			return "", err
		}
		cus.String = id
	}
	sess, err := s.Platform.CreateSetupSession(ctx, cus.String,
		s.WebURL+"/sell/shipping?card_session={CHECKOUT_SESSION_ID}", s.WebURL+"/sell/shipping")
	return sess.URL, err
}

// FinishCardSetup records the card saved on Stripe's page.
func (s *Service) FinishCardSetup(ctx context.Context, sellerID, sessionID string) (string, error) {
	if s.Platform == nil {
		return "", ErrNoLabels
	}
	pm, label, cus, err := s.Platform.SetupResult(ctx, sessionID)
	if err != nil {
		return "", err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE seller_profiles SET label_payment_method = $2, label_card = $3, updated_at = now()
		WHERE user_id = $1 AND stripe_customer_id = $4`, sellerID, pm, label, cus)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return label, nil
}

// LabelRate is a rate with what the seller's card will be charged.
type LabelRate struct {
	shipping.Rate
	ChargeCents     int  `json:"charge_cents"`
	ProcessingCents int  `json:"processing_cents"`
	Signature       bool `json:"signature"`
}

type labelOrder struct {
	id, buyer, status, method string
	number                    int64
	total                     int
	shipTo                    shipping.Address
	from                      *shipping.Address
	customer, pm              string
	items                     []shipping.Item
}

func (s *Service) loadLabelOrder(ctx context.Context, sellerID, orderID string) (labelOrder, error) {
	var o labelOrder
	var to, from []byte
	var cus, pm sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT o.id, o.buyer_id, o.status, o.ship_method, o.number, o.total_cents, o.ship_to,
		sp.ship_from, sp.stripe_customer_id, sp.label_payment_method
		FROM orders o JOIN seller_profiles sp ON sp.user_id = o.seller_id
		WHERE (o.id::text = $2 OR o.number::text = $2) AND o.seller_id = $1`, sellerID, orderID).
		Scan(&o.id, &o.buyer, &o.status, &o.method, &o.number, &o.total, &to, &from, &cus, &pm)
	if errors.Is(err, sql.ErrNoRows) {
		return o, ErrNotFound
	}
	if err != nil {
		return o, err
	}
	var addr map[string]string
	_ = json.Unmarshal(to, &addr)
	o.shipTo = shipping.Address{Name: addr["name"], Street1: addr["line1"], Street2: addr["line2"], City: addr["city"],
		State: addr["state"], Zip: addr["zip"], Country: "US"}
	if len(from) > 0 {
		var a shipping.Address
		if json.Unmarshal(from, &a) == nil {
			o.from = &a
		}
	}
	o.customer, o.pm = cus.String, pm.String
	rows, err := s.DB.QueryContext(ctx, `SELECT coalesce(l.package_preset, 'bubble'), coalesce(l.weight_oz, 1), oi.quantity, l.grader IS NOT NULL
		FROM order_items oi LEFT JOIN listings l ON l.id = oi.listing_id WHERE oi.order_id = $1`, o.id)
	if err != nil {
		return o, err
	}
	defer rows.Close()
	for rows.Next() {
		var it shipping.Item
		if err := rows.Scan(&it.Preset, &it.WeightOz, &it.Quantity, &it.Graded); err != nil {
			return o, err
		}
		o.items = append(o.items, it)
	}
	return o, rows.Err()
}

func (s *Service) activeLabel(ctx context.Context, orderID string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM shipments WHERE order_id = $1 AND provider IS NOT NULL AND voided_at IS NULL`,
		orderID).Scan(&n)
	return n > 0, err
}

// LabelRates gets live rates for an order's package.
func (s *Service) LabelRates(ctx context.Context, sellerID, orderID string) ([]LabelRate, error) {
	if s.Ship == nil || s.Platform == nil {
		return nil, ErrNoLabels
	}
	o, err := s.loadLabelOrder(ctx, sellerID, orderID)
	if err != nil {
		return nil, err
	}
	if o.status != "paid" {
		return nil, ErrState
	}
	if o.from == nil {
		return nil, ErrNoShipFrom
	}
	if has, err := s.activeLabel(ctx, o.id); err != nil || has {
		if err == nil {
			err = ErrHasLabel
		}
		return nil, err
	}
	sig := o.total > SignatureOver
	rates, err := s.Ship.Rates(ctx, shipping.RateRequest{From: *o.from, To: o.shipTo, Parcel: shipping.ParcelFor(o.items),
		Signature: sig, Reference: fmt.Sprintf("Order %d", o.number)})
	if err != nil {
		return nil, err
	}
	out := make([]LabelRate, 0, len(rates))
	for _, r := range rates {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO label_rates (rate_id, order_id, provider, carrier, service, amount_cents, days, signature)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (rate_id) DO NOTHING`,
			r.ID, o.id, s.Ship.Name(), r.Carrier, r.Service, r.AmountCents, r.Days, sig); err != nil {
			return nil, err
		}
		c := shipping.LabelChargeCents(r.AmountCents)
		out = append(out, LabelRate{Rate: r, ChargeCents: c, ProcessingCents: c - r.AmountCents, Signature: sig})
	}
	return out, nil
}

// BuyLabel charges the seller's saved card (label + card processing) and
// buys the label. The order is marked shipped with its tracking number.
func (s *Service) BuyLabel(ctx context.Context, sellerID, orderID, rateID string) (shipping.Label, error) {
	if s.Ship == nil || s.Platform == nil {
		return shipping.Label{}, ErrNoLabels
	}
	o, err := s.loadLabelOrder(ctx, sellerID, orderID)
	if err != nil {
		return shipping.Label{}, err
	}
	if o.status != "paid" {
		return shipping.Label{}, ErrState
	}
	if o.customer == "" || o.pm == "" {
		return shipping.Label{}, ErrNoCard
	}
	if has, err := s.activeLabel(ctx, o.id); err != nil || has {
		if err == nil {
			err = ErrHasLabel
		}
		return shipping.Label{}, err
	}
	var carrier, service string
	var cents int
	var sig bool
	err = s.DB.QueryRowContext(ctx, `SELECT carrier, service, amount_cents, signature FROM label_rates
		WHERE rate_id = $1 AND order_id = $2 AND provider = $3 AND created_at > now() - interval '12 hours'`,
		rateID, o.id, s.Ship.Name()).Scan(&carrier, &service, &cents, &sig)
	if errors.Is(err, sql.ErrNoRows) {
		return shipping.Label{}, ErrRateExpired
	}
	if err != nil {
		return shipping.Label{}, err
	}
	charge := shipping.LabelChargeCents(cents)
	pi, err := s.Platform.Charge(ctx, o.customer, o.pm, charge, fmt.Sprintf("Shipping label, order #%d", o.number),
		map[string]string{"order_id": o.id, "rate_id": rateID, "label_cents": fmt.Sprint(cents)})
	if err != nil {
		return shipping.Label{}, &FieldError{"card", "your card was declined: " + err.Error()}
	}
	l, err := s.Ship.BuyLabel(ctx, rateID)
	if err != nil {
		if rerr := s.Platform.RefundPlatform(context.WithoutCancel(ctx), pi); rerr != nil {
			return shipping.Label{}, fmt.Errorf("label failed (%v) and the card refund failed too: %w", err, rerr)
		}
		return shipping.Label{}, fmt.Errorf("the carrier didn't issue the label (you weren't charged): %w", err)
	}
	if l.Carrier == "" {
		l.Carrier = carrier
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return l, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO shipments (order_id, method, carrier, service, tracking_number, label_cost_cents, status,
		last_event, last_event_at, provider, provider_label_id, provider_rate_id, label_url, tracking_url, charge_cents,
		stripe_label_charge, signature)
		VALUES ($1, 'label', $2, $3, $4, $5, 'label_created', 'Label created', now(), $6, $7, $8, $9, $10, $11, $12, $13)`,
		o.id, l.Carrier, service, l.TrackingNumber, cents, s.Ship.Name(), l.ID, rateID, l.LabelURL, l.TrackingURL, charge, pi, sig); err != nil {
		return l, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'shipped', shipped_at = now(), updated_at = now() WHERE id = $1`, o.id); err != nil {
		return l, err
	}
	if err := ledger(ctx, tx, o.id, sellerID, "label_cost", cents, pi,
		fmt.Sprintf("%s %s label (card charged %s with processing)", l.Carrier, service, money(charge))); err != nil {
		return l, err
	}
	if err := inbox.Notify(ctx, tx, o.buyer, "order", fmt.Sprintf("Order #%d is ready to ship", o.number),
		fmt.Sprintf("The seller printed a %s label. Tracking: %s", l.Carrier, l.TrackingNumber), "/purchases/"+o.id); err != nil {
		return l, err
	}
	return l, tx.Commit()
}

// VoidLabel cancels an unused label and refunds the seller's card.
func (s *Service) VoidLabel(ctx context.Context, sellerID, orderID string) error {
	if s.Ship == nil || s.Platform == nil {
		return ErrNoLabels
	}
	var oid string
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM orders WHERE (id::text = $2 OR number::text = $2) AND seller_id = $1`,
		sellerID, orderID).Scan(&oid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return s.voidLabel(ctx, oid, sellerID, true)
}

func (s *Service) voidLabel(ctx context.Context, orderID, sellerID string, reopen bool) error {
	var shipID, labelID, provider, status, pi string
	var cents int
	err := s.DB.QueryRowContext(ctx, `SELECT id, provider_label_id, provider, status, coalesce(stripe_label_charge, ''), coalesce(label_cost_cents, 0)
		FROM shipments WHERE order_id = $1 AND provider IS NOT NULL AND voided_at IS NULL ORDER BY created_at DESC LIMIT 1`, orderID).
		Scan(&shipID, &labelID, &provider, &status, &pi, &cents)
	if errors.Is(err, sql.ErrNoRows) {
		return &FieldError{"label", "there's no label to void"}
	}
	if err != nil {
		return err
	}
	if status != "label_created" {
		return ErrLabelUsed
	}
	if provider != s.Ship.Name() {
		return fmt.Errorf("this label was bought through %s, which isn't configured now", provider)
	}
	if err := s.Ship.VoidLabel(ctx, labelID); err != nil {
		return err
	}
	if pi != "" {
		if err := s.Platform.RefundPlatform(ctx, pi); err != nil {
			return fmt.Errorf("label voided, but refunding your card failed: %w", err)
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE shipments SET status = 'voided', voided_at = now(), last_event = 'Label voided',
		last_event_at = now(), updated_at = now() WHERE id = $1`, shipID); err != nil {
		return err
	}
	if reopen {
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'paid', shipped_at = NULL, updated_at = now()
			WHERE id = $1 AND status = 'shipped'`, orderID); err != nil {
			return err
		}
	}
	if err := ledger(ctx, tx, orderID, sellerID, "adjustment", cents, pi+":void", "Label voided; seller's card refunded"); err != nil {
		return err
	}
	return tx.Commit()
}

// voidUnusedLabel is called when an order is refunded in full before the
// label was scanned: the label is voided and the seller's card refunded.
func (s *Service) voidUnusedLabel(ctx context.Context, orderID, sellerID string) {
	if s.Ship == nil || s.Platform == nil {
		return
	}
	_ = s.voidLabel(ctx, orderID, sellerID, false)
}

var trackingState = map[string]string{
	shipping.InTransit: "in_transit", shipping.Delivered: "delivered", shipping.Returned: "returned", shipping.Exception: "exception",
}

// ApplyTracking records carrier scans from a provider webhook. It returns
// how many shipments were updated.
func (s *Service) ApplyTracking(ctx context.Context, events []shipping.TrackingEvent) (int, error) {
	n := 0
	for _, e := range events {
		ev, _ := json.Marshal([]map[string]any{{"at": e.At.UTC().Format(time.RFC3339), "status": e.Status,
			"description": e.Description, "location": e.Location}})
		state := trackingState[e.Status]
		var orderID, buyer string
		var number int64
		err := s.DB.QueryRowContext(ctx, `
			UPDATE shipments sh SET events = sh.events || $2::jsonb, last_event = $3, last_event_at = $4,
				status = CASE WHEN $5 = '' THEN sh.status ELSE $5 END, updated_at = now()
			FROM orders o
			WHERE sh.tracking_number = $1 AND sh.voided_at IS NULL AND o.id = sh.order_id
			RETURNING o.id, o.buyer_id, o.number`, e.TrackingNumber, string(ev),
			strings.TrimSpace(e.Description+" "+e.Location), e.At, state).Scan(&orderID, &buyer, &number)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return n, err
		}
		n++
		if e.Status == shipping.Delivered {
			res, err := s.DB.ExecContext(ctx, `UPDATE orders SET status = 'delivered', delivered_at = $2, updated_at = now()
				WHERE id = $1 AND status = 'shipped'`, orderID, e.At)
			if err != nil {
				return n, err
			}
			if k, _ := res.RowsAffected(); k > 0 {
				if err := inbox.Notify(ctx, s.DB, buyer, "order", fmt.Sprintf("Order #%d was delivered", number),
					"How did it go? Leave feedback for the seller.", "/purchases/"+orderID); err != nil {
					return n, err
				}
			}
		}
	}
	return n, nil
}
