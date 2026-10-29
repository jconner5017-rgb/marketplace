package checkout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
)

// FieldError is a problem with one input.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

var ErrState = errors.New("the order isn't at a stage where you can do that")

type Line struct {
	ListingID *string `json:"listing_id"`
	Title     string  `json:"title"`
	Photo     string  `json:"photo"`
	Source    string  `json:"source"`
	UnitCents int     `json:"unit_cents"`
	Quantity  int     `json:"quantity"`
}

type Shipment struct {
	Method    string    `json:"method"`
	Carrier   *string   `json:"carrier"`
	Tracking  *string   `json:"tracking_number"`
	CreatedAt time.Time `json:"created_at"`
}

type Feedback struct {
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type CaseRef struct {
	Number int64  `json:"number"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type Order struct {
	ID            string          `json:"id"`
	Number        int64           `json:"number"`
	Status        string          `json:"status"`
	Buyer         string          `json:"buyer"`
	Seller        string          `json:"seller"`
	SubtotalCents int             `json:"subtotal_cents"`
	DiscountCents int             `json:"discount_cents"`
	TotalCents    int             `json:"total_cents"`
	FeeCents      *int            `json:"fee_cents,omitempty"` // seller only
	RefundedCents int             `json:"refunded_cents"`
	ShipMethod    string          `json:"ship_method"`
	ShipTo        json.RawMessage `json:"ship_to"`
	ShipBy        *time.Time      `json:"ship_by"`
	CreatedAt     time.Time       `json:"created_at"`
	PaidAt        *time.Time      `json:"paid_at"`
	ShippedAt     *time.Time      `json:"shipped_at"`
	DeliveredAt   *time.Time      `json:"delivered_at"`
	Lines         []Line          `json:"lines"`
	Shipment      *Shipment       `json:"shipment"`
	MyFeedback    *Feedback       `json:"my_feedback"`
	TheirFeedback *Feedback       `json:"their_feedback"`
	Cases         []CaseRef       `json:"cases"`
	Side          string          `json:"side"` // buying or selling, from the viewer's side
	TaxCents      int             `json:"tax_cents"`
	// StripeRefundURL opens the payment in the seller's own Stripe dashboard (seller only).
	StripeRefundURL string `json:"stripe_payment_url,omitempty"`
}

const orderCols = `o.id, o.number, o.status, bu.username, su.username, o.subtotal_cents, o.discount_cents,
	o.total_cents, o.fee_cents, o.refunded_cents, o.ship_method, o.ship_to, o.ship_by, o.created_at, o.paid_at,
	o.shipped_at, o.delivered_at, o.buyer_id = $1, coalesce(o.stripe_payment_intent, ''), o.tax_cents`

const orderFrom = ` FROM orders o JOIN users bu ON bu.id = o.buyer_id JOIN users su ON su.id = o.seller_id `

func scanOrder(row interface{ Scan(...any) error }) (Order, error) {
	var o Order
	var fee int
	var shipBy, paid, shipped, deliv sql.NullTime
	var buying bool
	var pi string
	if err := row.Scan(&o.ID, &o.Number, &o.Status, &o.Buyer, &o.Seller, &o.SubtotalCents, &o.DiscountCents,
		&o.TotalCents, &fee, &o.RefundedCents, &o.ShipMethod, &o.ShipTo, &shipBy, &o.CreatedAt, &paid,
		&shipped, &deliv, &buying, &pi, &o.TaxCents); err != nil {
		return o, err
	}
	o.ShipBy, o.PaidAt, o.ShippedAt, o.DeliveredAt = nt(shipBy), nt(paid), nt(shipped), nt(deliv)
	o.Side = "selling"
	if buying {
		o.Side = "buying"
	} else {
		o.FeeCents = &fee
		if strings.HasPrefix(pi, "pi_") {
			o.StripeRefundURL = "https://dashboard.stripe.com/payments/" + pi
		}
	}
	o.Lines, o.Cases = []Line{}, []CaseRef{}
	return o, nil
}

func nt(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// Orders lists a user's paid orders as buyer ("buying") or seller ("selling").
// status narrows the list: "", "to_ship", "shipped", "delivered" or "closed".
func (s *Service) Orders(ctx context.Context, userID, side, status string, limit int) ([]Order, error) {
	col := "o.buyer_id"
	if side == "selling" {
		col = "o.seller_id"
	}
	where := col + ` = $1 AND o.status <> 'pending_payment' AND o.paid_at IS NOT NULL`
	switch status {
	case "to_ship":
		where += ` AND o.status = 'paid'`
	case "shipped":
		where += ` AND o.status = 'shipped'`
	case "delivered":
		where += ` AND o.status = 'delivered'`
	case "closed":
		where += ` AND o.status IN ('cancelled', 'refunded', 'partially_refunded')`
	}
	order := "o.paid_at DESC"
	if status == "to_ship" {
		order = "o.ship_by ASC"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+orderCols+orderFrom+` WHERE `+where+` ORDER BY `+order+` LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	out := []Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.fill(ctx, userID, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Order returns one order the user bought or sold.
func (s *Service) Order(ctx context.Context, userID, id string) (Order, error) {
	o, err := scanOrder(s.DB.QueryRowContext(ctx, `SELECT `+orderCols+orderFrom+`
		WHERE (o.id::text = $2 OR o.number::text = $2) AND (o.buyer_id = $1 OR o.seller_id = $1)
		  AND o.status <> 'pending_payment' AND o.paid_at IS NOT NULL`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return o, ErrNotFound
	}
	if err != nil {
		return o, err
	}
	return o, s.fill(ctx, userID, &o)
}

func (s *Service) fill(ctx context.Context, userID string, o *Order) error {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT oi.listing_id::text, oi.title, coalesce(l.photo_urls[1], ''), oi.source, oi.unit_cents, oi.quantity
		FROM order_items oi LEFT JOIN listings l ON l.id = oi.listing_id
		WHERE oi.order_id = $1 ORDER BY oi.id`, o.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var l Line
		var lid sql.NullString
		if err := rows.Scan(&lid, &l.Title, &l.Photo, &l.Source, &l.UnitCents, &l.Quantity); err != nil {
			rows.Close()
			return err
		}
		if lid.Valid {
			l.ListingID = &lid.String
		}
		o.Lines = append(o.Lines, l)
	}
	rows.Close()

	var sh Shipment
	err = s.DB.QueryRowContext(ctx, `SELECT method, carrier, tracking_number, created_at FROM shipments
		WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`, o.ID).Scan(&sh.Method, &sh.Carrier, &sh.Tracking, &sh.CreatedAt)
	if err == nil {
		o.Shipment = &sh
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	fr, err := s.DB.QueryContext(ctx, `SELECT from_user = $2, rating, comment, created_at FROM feedback
		WHERE order_id = $1 AND removed_at IS NULL`, o.ID, userID)
	if err != nil {
		return err
	}
	for fr.Next() {
		var mine bool
		var f Feedback
		if err := fr.Scan(&mine, &f.Rating, &f.Comment, &f.CreatedAt); err != nil {
			fr.Close()
			return err
		}
		if mine {
			o.MyFeedback = &f
		} else {
			o.TheirFeedback = &f
		}
	}
	fr.Close()

	cr, err := s.DB.QueryContext(ctx, `SELECT number, kind, status FROM cases WHERE order_id = $1 ORDER BY created_at`, o.ID)
	if err != nil {
		return err
	}
	defer cr.Close()
	for cr.Next() {
		var c CaseRef
		if err := cr.Scan(&c.Number, &c.Kind, &c.Status); err != nil {
			return err
		}
		o.Cases = append(o.Cases, c)
	}
	return cr.Err()
}

var trackingRe = regexp.MustCompile(`^[A-Za-z0-9]{8,40}$`)

var carriers = map[string]bool{"USPS": true, "UPS": true, "FedEx": true, "DHL": true}

// MarkShipped records that the seller sent the order. Label orders need a
// tracking number; plain envelopes may not have one.
func (s *Service) MarkShipped(ctx context.Context, sellerID, orderID, carrier, tracking string) error {
	tracking = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(tracking), " ", ""))
	carrier = strings.TrimSpace(carrier)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id, buyer, status, method string
	var number int64
	err = tx.QueryRowContext(ctx, `SELECT id, buyer_id, status, ship_method, number FROM orders
		WHERE (id::text = $2 OR number::text = $2) AND seller_id = $1 FOR UPDATE`, sellerID, orderID).
		Scan(&id, &buyer, &status, &method, &number)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "paid" {
		return ErrState
	}
	if tracking != "" || method == "label" {
		if !carriers[carrier] {
			return &FieldError{"carrier", "choose USPS, UPS, FedEx or DHL"}
		}
		if !trackingRe.MatchString(tracking) {
			return &FieldError{"tracking_number", "enter the tracking number (8 to 40 letters and digits)"}
		}
	}
	var c, t any
	if tracking != "" {
		c, t = carrier, tracking
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO shipments (order_id, method, carrier, tracking_number, status, last_event, last_event_at)
		VALUES ($1, $2, $3, $4, 'in_transit', 'Marked shipped by seller', now())`, id, method, c, t); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'shipped', shipped_at = now(), updated_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	body := "It's on the way in a plain envelope (no tracking)."
	if tracking != "" {
		body = fmt.Sprintf("Tracking: %s %s", carrier, tracking)
	}
	if err := inbox.Notify(ctx, tx, buyer, "order", fmt.Sprintf("Order #%d shipped", number), body, "/purchases/"+id); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkDelivered is the buyer saying the cards arrived.
func (s *Service) MarkDelivered(ctx context.Context, buyerID, orderID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE orders SET status = 'delivered', delivered_at = now(), updated_at = now()
		WHERE (id::text = $2 OR number::text = $2) AND buyer_id = $1 AND status = 'shipped'`, buyerID, orderID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrState
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE shipments SET status = 'delivered', last_event = 'Buyer confirmed delivery',
		last_event_at = now(), updated_at = now()
		WHERE order_id = (SELECT id FROM orders WHERE id::text = $1 OR number::text = $1)`, orderID)
	return err
}

// FeedbackWindow is how long after payment either side can leave feedback.
const FeedbackWindow = 60 * 24 * time.Hour

// LeaveFeedback records a positive (1), neutral (0) or negative (-1) rating.
// Buyers can rate once the order has shipped; sellers once it's paid.
func (s *Service) LeaveFeedback(ctx context.Context, userID, orderID string, rating int, comment string) error {
	if rating < -1 || rating > 1 {
		return &FieldError{"rating", "choose positive, neutral or negative"}
	}
	comment = strings.TrimSpace(comment)
	if utf8.RuneCountInString(comment) > 500 {
		return &FieldError{"comment", "keep it under 500 characters"}
	}
	if rating < 1 && utf8.RuneCountInString(comment) < 5 {
		return &FieldError{"comment", "say what went wrong (at least a few words)"}
	}
	var id, buyer, seller, status string
	var paid sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT id, buyer_id, seller_id, status, paid_at FROM orders
		WHERE (id::text = $2 OR number::text = $2) AND (buyer_id = $1 OR seller_id = $1)`, userID, orderID).
		Scan(&id, &buyer, &seller, &status, &paid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !paid.Valid || s.now().Sub(paid.Time) > FeedbackWindow {
		return ErrState
	}
	role, to := "seller_to_buyer", buyer
	if userID == buyer {
		role, to = "buyer_to_seller", seller
		if status != "shipped" && status != "delivered" {
			return ErrState
		}
	}
	if msg, ok := inboxScreen(comment); !ok {
		return &FieldError{"comment", msg}
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO feedback (order_id, from_user, to_user, role, rating, comment)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (order_id, from_user) DO NOTHING`, id, userID, to, role, rating, comment); err != nil {
		return err
	}
	if role == "buyer_to_seller" && s.Rep != nil {
		if _, _, err := s.Rep.Review(ctx, seller); err != nil {
			return err
		}
	}
	return nil
}

// ReportProblem opens a case for staff on a paid order.
func (s *Service) ReportProblem(ctx context.Context, buyerID, orderID, kind, summary string) (int64, error) {
	if kind != "inr" && kind != "snad" && kind != "other" {
		return 0, &FieldError{"kind", "choose what went wrong"}
	}
	summary = strings.TrimSpace(summary)
	if n := utf8.RuneCountInString(summary); n < 10 || n > 2000 {
		return 0, &FieldError{"summary", "describe the problem (10 to 2,000 characters)"}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id, seller, status, email string
	var number int64
	var shipBy sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT o.id, o.seller_id, o.status, o.number, o.ship_by, u.email FROM orders o
		JOIN users u ON u.id = o.buyer_id
		WHERE (o.id::text = $2 OR o.number::text = $2) AND o.buyer_id = $1 FOR UPDATE OF o`, buyerID, orderID).
		Scan(&id, &seller, &status, &number, &shipBy, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if status != "paid" && status != "shipped" && status != "delivered" {
		return 0, ErrState
	}
	if kind == "inr" && status == "paid" && shipBy.Valid && s.now().Before(shipBy.Time) {
		return 0, &FieldError{"kind", "the seller has until " + shipBy.Time.Format("Mon Jan 2") + " to ship"}
	}
	var open int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cases WHERE order_id = $1
		AND status IN ('open', 'waiting_seller', 'waiting_buyer')`, id).Scan(&open); err != nil {
		return 0, err
	}
	if open > 0 {
		return 0, &FieldError{"kind", "there's already an open case on this order"}
	}
	var caseID string
	var cnum int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO cases (order_id, opened_by, kind, summary) VALUES ($1, $2, $3, $4)
		RETURNING id, number`, id, buyerID, kind, summary).Scan(&caseID, &cnum); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO case_notes (case_id, author_id, author_email, internal, body)
		VALUES ($1, $2, $3, false, $4)`, caseID, buyerID, email, summary); err != nil {
		return 0, err
	}
	if err := inbox.Notify(ctx, tx, seller, "order", fmt.Sprintf("Problem reported on order #%d", number),
		"The buyer opened case #"+fmt.Sprint(cnum)+". Our support team will be in touch; please reply to the buyer in Messages.",
		"/sell/orders/"+id); err != nil {
		return 0, err
	}
	return cnum, tx.Commit()
}

// inboxScreen applies the message filter (no contact details or off-site payment).
func inboxScreen(text string) (string, bool) {
	if inbox.Screen(text) {
		return "leave out contact details and other payment apps", false
	}
	return "", true
}
