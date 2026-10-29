package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Records covers orders, shipping, cases, refund reports, feedback and
// message logs, as staff see them.
type Records struct {
	DB    *sql.DB
	Audit *Audit
}

// OrderRow is one line of the order ledger.
type OrderRow struct {
	ID         string     `json:"id"`
	Number     int64      `json:"number"`
	Status     string     `json:"status"`
	Buyer      string     `json:"buyer"`
	BuyerID    string     `json:"buyer_id"`
	Seller     string     `json:"seller"`
	SellerID   string     `json:"seller_id"`
	Items      int        `json:"items"`
	TotalCents int        `json:"total_cents"`
	FeeCents   int        `json:"fee_cents"`
	Refunded   int        `json:"refunded_cents"`
	ShipMethod string     `json:"ship_method"`
	ShipBy     *time.Time `json:"ship_by"`
	CreatedAt  time.Time  `json:"created_at"`
	PaidAt     *time.Time `json:"paid_at"`
	ShippedAt  *time.Time `json:"shipped_at"`
	OpenCases  int        `json:"open_cases"`
	Tracking   *string    `json:"tracking"`
	ShipStatus *string    `json:"ship_status"`
	Late       bool       `json:"late"`
}

// OrderQuery filters the ledger and the shipping view.
type OrderQuery struct {
	Q, Status, Party string // Q: order number, order ID, Stripe ID or tracking; Party: username
	Shipping         string // late, awaiting, in_transit, exception, delivered
	Limit, Offset    int
}

const orderSelect = `
	SELECT o.id, o.number, o.status, b.username, b.id, s.username, s.id,
	       (SELECT coalesce(sum(quantity), 0) FROM order_items i WHERE i.order_id = o.id),
	       o.total_cents, o.fee_cents, o.refunded_cents, o.ship_method, o.ship_by, o.created_at, o.paid_at, o.shipped_at,
	       (SELECT count(*) FROM cases c WHERE c.order_id = o.id AND c.status NOT IN ('resolved_buyer', 'resolved_seller', 'closed')),
	       sh.tracking_number, sh.status,
	       (o.status = 'paid' AND o.ship_by < now())
	FROM orders o
	JOIN users b ON b.id = o.buyer_id
	JOIN users s ON s.id = o.seller_id
	LEFT JOIN LATERAL (SELECT tracking_number, status FROM shipments x WHERE x.order_id = o.id ORDER BY x.created_at DESC LIMIT 1) sh ON true`

func scanOrder(row interface{ Scan(...any) error }) (OrderRow, error) {
	var o OrderRow
	var shipBy, paid, shipped sql.NullTime
	var track, sst sql.NullString
	err := row.Scan(&o.ID, &o.Number, &o.Status, &o.Buyer, &o.BuyerID, &o.Seller, &o.SellerID, &o.Items, &o.TotalCents,
		&o.FeeCents, &o.Refunded, &o.ShipMethod, &shipBy, &o.CreatedAt, &paid, &shipped, &o.OpenCases, &track, &sst, &o.Late)
	if shipBy.Valid {
		o.ShipBy = &shipBy.Time
	}
	if paid.Valid {
		o.PaidAt = &paid.Time
	}
	if shipped.Valid {
		o.ShippedAt = &shipped.Time
	}
	if track.Valid {
		o.Tracking = &track.String
	}
	if sst.Valid {
		o.ShipStatus = &sst.String
	}
	return o, err
}

type whereBuilder struct {
	where []string
	args  []any
}

func (w *whereBuilder) add(cond string, v ...any) {
	for _, x := range v {
		w.args = append(w.args, x)
		cond = strings.Replace(cond, "?", "$"+strconv.Itoa(len(w.args)), 1)
	}
	w.where = append(w.where, cond)
}

func (w *whereBuilder) sql() string {
	if len(w.where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.where, " AND ")
}

// Orders searches the ledger, newest first.
func (r *Records) Orders(ctx context.Context, q OrderQuery) ([]OrderRow, int, error) {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 50
	}
	w := &whereBuilder{}
	if s := strings.TrimSpace(strings.TrimPrefix(q.Q, "#")); s != "" {
		w.add(`(o.number::text = ? OR o.id::text = ? OR o.stripe_payment_intent = ? OR o.stripe_charge = ? OR o.stripe_checkout_id = ?
			OR EXISTS (SELECT 1 FROM shipments x WHERE x.order_id = o.id AND x.tracking_number = ?))`, s, s, s, s, s, s)
	}
	if q.Status != "" {
		w.add("o.status = ?", q.Status)
	}
	if q.Party != "" {
		w.add("(lower(b.username) = lower(?) OR lower(s.username) = lower(?))", q.Party, q.Party)
	}
	switch q.Shipping {
	case "late":
		w.add("o.status = 'paid' AND o.ship_by < now()")
	case "awaiting":
		w.add("o.status = 'paid'")
	case "in_transit", "exception", "delivered":
		w.add("sh.status = ?", q.Shipping)
	}
	from := ` FROM orders o JOIN users b ON b.id = o.buyer_id JOIN users s ON s.id = o.seller_id
		LEFT JOIN LATERAL (SELECT tracking_number, status FROM shipments x WHERE x.order_id = o.id ORDER BY x.created_at DESC LIMIT 1) sh ON true`
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT count(*)`+from+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args := append(w.args, q.Limit, q.Offset)
	order := "o.created_at DESC"
	if q.Shipping == "late" || q.Shipping == "awaiting" {
		order = "o.ship_by ASC NULLS LAST"
	}
	rows, err := r.DB.QueryContext(ctx, orderSelect+w.sql()+fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", order, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []OrderRow{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, o)
	}
	return out, total, rows.Err()
}

// OrderDetail is the full order page.
type OrderDetail struct {
	OrderRow
	SubtotalCents int             `json:"subtotal_cents"`
	DiscountCents int             `json:"discount_cents"`
	ShipTo        json.RawMessage `json:"ship_to"`
	StripeAccount *string         `json:"stripe_account"`
	CheckoutID    *string         `json:"stripe_checkout_id"`
	PaymentIntent *string         `json:"stripe_payment_intent"`
	Charge        *string         `json:"stripe_charge"`
	CancelReason  *string         `json:"cancel_reason"`
	DeliveredAt   *time.Time      `json:"delivered_at"`
	CancelledAt   *time.Time      `json:"cancelled_at"`
	Lines         []OrderLine     `json:"lines"`
	Shipments     []Shipment      `json:"shipments"`
	Refunds       []RefundReport  `json:"refunds"`
	Cases         []CaseRow       `json:"cases"`
	Feedback      []FeedbackRow   `json:"feedback"`
	ThreadIDs     []string        `json:"thread_ids"`
}

type OrderLine struct {
	ListingID *string `json:"listing_id"`
	Version   *int    `json:"listing_version"`
	Source    string  `json:"source"`
	Title     string  `json:"title"`
	UnitCents int     `json:"unit_cents"`
	Quantity  int     `json:"quantity"`
}

type Shipment struct {
	ID         string          `json:"id"`
	Method     string          `json:"method"`
	Carrier    *string         `json:"carrier"`
	Service    *string         `json:"service"`
	Tracking   *string         `json:"tracking_number"`
	LabelCents *int            `json:"label_cost_cents"`
	Status     string          `json:"status"`
	LastEvent  *string         `json:"last_event"`
	LastAt     *time.Time      `json:"last_event_at"`
	Events     json.RawMessage `json:"events"`
	CreatedAt  time.Time       `json:"created_at"`
}

func nstr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func ntime(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	return &n.Time
}

// Order loads one order with everything attached to it.
func (r *Records) Order(ctx context.Context, id string) (OrderDetail, error) {
	row, err := scanOrder(r.DB.QueryRowContext(ctx, orderSelect+` WHERE o.id::text = $1 OR o.number::text = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return OrderDetail{}, ErrNotFound
	}
	if err != nil {
		return OrderDetail{}, err
	}
	d := OrderDetail{OrderRow: row, Lines: []OrderLine{}, Shipments: []Shipment{}, ThreadIDs: []string{}}
	var acct, cs, pi, ch, reason sql.NullString
	var deliv, canc sql.NullTime
	if err := r.DB.QueryRowContext(ctx, `SELECT subtotal_cents, discount_cents, ship_to, stripe_account, stripe_checkout_id,
		stripe_payment_intent, stripe_charge, cancel_reason, delivered_at, cancelled_at FROM orders WHERE id = $1`, row.ID).Scan(
		&d.SubtotalCents, &d.DiscountCents, &d.ShipTo, &acct, &cs, &pi, &ch, &reason, &deliv, &canc); err != nil {
		return d, err
	}
	d.StripeAccount, d.CheckoutID, d.PaymentIntent, d.Charge, d.CancelReason = nstr(acct), nstr(cs), nstr(pi), nstr(ch), nstr(reason)
	d.DeliveredAt, d.CancelledAt = ntime(deliv), ntime(canc)

	rows, err := r.DB.QueryContext(ctx, `SELECT listing_id::text, listing_version, source, title, unit_cents, quantity
		FROM order_items WHERE order_id = $1 ORDER BY id`, row.ID)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var l OrderLine
		var lid sql.NullString
		var ver sql.NullInt64
		if err := rows.Scan(&lid, &ver, &l.Source, &l.Title, &l.UnitCents, &l.Quantity); err != nil {
			rows.Close()
			return d, err
		}
		l.ListingID = nstr(lid)
		if ver.Valid {
			v := int(ver.Int64)
			l.Version = &v
		}
		d.Lines = append(d.Lines, l)
	}
	rows.Close()

	srows, err := r.DB.QueryContext(ctx, `SELECT id, method, carrier, service, tracking_number, label_cost_cents, status,
		last_event, last_event_at, events, created_at FROM shipments WHERE order_id = $1 ORDER BY created_at`, row.ID)
	if err != nil {
		return d, err
	}
	for srows.Next() {
		var s Shipment
		var carrier, service, track, last sql.NullString
		var cost sql.NullInt64
		var at sql.NullTime
		if err := srows.Scan(&s.ID, &s.Method, &carrier, &service, &track, &cost, &s.Status, &last, &at, &s.Events, &s.CreatedAt); err != nil {
			srows.Close()
			return d, err
		}
		s.Carrier, s.Service, s.Tracking, s.LastEvent, s.LastAt = nstr(carrier), nstr(service), nstr(track), nstr(last), ntime(at)
		if cost.Valid {
			c := int(cost.Int64)
			s.LabelCents = &c
		}
		d.Shipments = append(d.Shipments, s)
	}
	srows.Close()

	if d.Refunds, err = r.Refunds(ctx, row.ID, "", 50); err != nil {
		return d, err
	}
	if d.Cases, err = r.Cases(ctx, CaseQuery{OrderID: row.ID}); err != nil {
		return d, err
	}
	if d.Feedback, err = r.Feedback(ctx, FeedbackQuery{OrderID: row.ID}); err != nil {
		return d, err
	}
	trows, err := r.DB.QueryContext(ctx, `SELECT DISTINCT t.id FROM message_threads t JOIN order_items i ON i.listing_id = t.listing_id
		WHERE i.order_id = $1 AND t.buyer_id = $2 AND t.seller_id = $3`, row.ID, row.BuyerID, row.SellerID)
	if err != nil {
		return d, err
	}
	for trows.Next() {
		var t string
		trows.Scan(&t)
		d.ThreadIDs = append(d.ThreadIDs, t)
	}
	trows.Close()
	return d, nil
}

// ---- Cases ----

type CaseRow struct {
	ID          string     `json:"id"`
	Number      int64      `json:"number"`
	OrderID     string     `json:"order_id"`
	OrderNumber int64      `json:"order_number"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary"`
	Buyer       string     `json:"buyer"`
	Seller      string     `json:"seller"`
	OpenedBy    string     `json:"opened_by"`
	Assigned    string     `json:"assigned_to"`
	AssignedID  *string    `json:"assigned_id"`
	Outcome     *string    `json:"outcome"`
	TotalCents  int        `json:"total_cents"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
}

type CaseQuery struct {
	Status   string // open (any not closed), or an exact status
	OrderID  string
	Assigned string
	Limit    int
}

var caseOpen = []string{"open", "waiting_seller", "waiting_buyer"}
var caseStatuses = []string{"open", "waiting_seller", "waiting_buyer", "resolved_buyer", "resolved_seller", "closed"}

// Cases lists dispute cases: open ones oldest first.
func (r *Records) Cases(ctx context.Context, q CaseQuery) ([]CaseRow, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 100
	}
	w := &whereBuilder{}
	order := "c.created_at DESC"
	switch {
	case q.Status == "open":
		w.add("c.status IN ('open', 'waiting_seller', 'waiting_buyer')")
		order = "c.created_at ASC"
	case q.Status != "":
		w.add("c.status = ?", q.Status)
	}
	if q.OrderID != "" {
		w.add("c.order_id::text = ?", q.OrderID)
	}
	if q.Assigned != "" {
		w.add("c.assigned_to::text = ?", q.Assigned)
	}
	args := append(w.args, q.Limit)
	rows, err := r.DB.QueryContext(ctx, `
		SELECT c.id, c.number, o.id, o.number, c.kind, c.status, c.summary, b.username, s.username,
		       coalesce(ob.username, ''), coalesce(a.email, ''), c.assigned_to::text, c.outcome, o.total_cents,
		       c.created_at, c.updated_at, c.closed_at
		FROM cases c JOIN orders o ON o.id = c.order_id
		JOIN users b ON b.id = o.buyer_id JOIN users s ON s.id = o.seller_id
		LEFT JOIN users ob ON ob.id = c.opened_by LEFT JOIN users a ON a.id = c.assigned_to`+w.sql()+
		fmt.Sprintf(" ORDER BY %s LIMIT $%d", order, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CaseRow{}
	for rows.Next() {
		var c CaseRow
		var aid, outcome sql.NullString
		var closed sql.NullTime
		if err := rows.Scan(&c.ID, &c.Number, &c.OrderID, &c.OrderNumber, &c.Kind, &c.Status, &c.Summary, &c.Buyer, &c.Seller,
			&c.OpenedBy, &c.Assigned, &aid, &outcome, &c.TotalCents, &c.CreatedAt, &c.UpdatedAt, &closed); err != nil {
			return nil, err
		}
		c.AssignedID, c.Outcome, c.ClosedAt = nstr(aid), nstr(outcome), ntime(closed)
		out = append(out, c)
	}
	return out, rows.Err()
}

type CaseNote struct {
	ID       int64     `json:"id"`
	Author   string    `json:"author"`
	Internal bool      `json:"internal"`
	Body     string    `json:"body"`
	At       time.Time `json:"at"`
}

type CaseDetail struct {
	CaseRow
	Notes []CaseNote  `json:"notes"`
	Order OrderDetail `json:"order"`
}

// Case loads a case with its notes and order.
func (r *Records) Case(ctx context.Context, id string) (CaseDetail, error) {
	var orderID string
	err := r.DB.QueryRowContext(ctx, `SELECT order_id FROM cases WHERE id::text = $1 OR number::text = $1`, id).Scan(&orderID)
	if errors.Is(err, sql.ErrNoRows) {
		return CaseDetail{}, ErrNotFound
	}
	if err != nil {
		return CaseDetail{}, err
	}
	order, err := r.Order(ctx, orderID)
	if err != nil {
		return CaseDetail{}, err
	}
	var d CaseDetail
	for _, c := range order.Cases {
		if c.ID == id || strconv.FormatInt(c.Number, 10) == id {
			d.CaseRow = c
		}
	}
	d.Order, d.Notes = order, []CaseNote{}
	rows, err := r.DB.QueryContext(ctx, `SELECT id, author_email, internal, body, created_at FROM case_notes WHERE case_id = $1 ORDER BY id`, d.ID)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var n CaseNote
		if err := rows.Scan(&n.ID, &n.Author, &n.Internal, &n.Body, &n.At); err != nil {
			return d, err
		}
		d.Notes = append(d.Notes, n)
	}
	return d, rows.Err()
}

// OpenCase starts a case on an order (staff-opened; members open theirs from the order page).
func (r *Records) OpenCase(ctx context.Context, a Actor, orderID, kind, summary string) (string, error) {
	if kind != "inr" && kind != "snad" && kind != "other" {
		return "", &FieldError{"kind", "item not received, not as described, or other"}
	}
	summary = strings.TrimSpace(summary)
	if utf8.RuneCountInString(summary) < 5 || utf8.RuneCountInString(summary) > 2000 {
		return "", &FieldError{"summary", "describe the problem (5 to 2,000 characters)"}
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var oid, buyer, seller string
	var number int64
	err = tx.QueryRowContext(ctx, `SELECT id, number, buyer_id, seller_id FROM orders WHERE id::text = $1 OR number::text = $1`, orderID).
		Scan(&oid, &number, &buyer, &seller)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var id string
	var cnum int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO cases (order_id, opened_by, kind, summary, assigned_to) VALUES ($1, $2, $3, $4, $2)
		RETURNING id, number`, oid, a.ID, kind, summary).Scan(&id, &cnum); err != nil {
		return "", err
	}
	for _, u := range []string{buyer, seller} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body) VALUES ($1, 'order', $2, $3)`,
			u, fmt.Sprintf("Case %d opened on order %d", cnum, number), "Our team is looking into this order and may message you."); err != nil {
			return "", err
		}
	}
	if err := r.Audit.Log(ctx, tx, a.entry("case.opened", "case", id, summary, map[string]any{"order": number, "kind": kind})); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// CaseUpdate changes a case's status, owner or outcome. Nil fields stay.
type CaseUpdate struct {
	Status  *string `json:"status"`
	Assign  *string `json:"assigned_to"` // user ID, or "" to unassign
	Outcome *string `json:"outcome"`
	Reason  string  `json:"reason"`
}

func (r *Records) UpdateCase(ctx context.Context, a Actor, id string, u CaseUpdate) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cur string
	var cnum int64
	var buyer, seller string
	err = tx.QueryRowContext(ctx, `SELECT c.status, c.number, o.buyer_id, o.seller_id FROM cases c JOIN orders o ON o.id = c.order_id
		WHERE c.id::text = $1 FOR UPDATE OF c`, id).Scan(&cur, &cnum, &buyer, &seller)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	details := map[string]any{}
	if u.Status != nil {
		ok := false
		for _, s := range caseStatuses {
			ok = ok || s == *u.Status
		}
		if !ok {
			return &FieldError{"status", "unknown case status"}
		}
		closing := !contains(caseOpen, *u.Status)
		if closing && (u.Outcome == nil || strings.TrimSpace(*u.Outcome) == "") {
			return &FieldError{"outcome", "write the outcome before closing the case"}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE cases SET status = $2, closed_at = CASE WHEN $3 THEN now() ELSE NULL END,
			updated_at = now() WHERE id::text = $1`, id, *u.Status, closing); err != nil {
			return err
		}
		details["from"], details["to"] = cur, *u.Status
		if closing && contains(caseOpen, cur) {
			for _, p := range []string{buyer, seller} {
				if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body) VALUES ($1, 'order', $2, $3)`,
					p, fmt.Sprintf("Case %d is closed", cnum), strings.TrimSpace(*u.Outcome)); err != nil {
					return err
				}
			}
		}
	}
	if u.Assign != nil {
		var v any
		if *u.Assign != "" {
			v = *u.Assign
		}
		if _, err := tx.ExecContext(ctx, `UPDATE cases SET assigned_to = $2, updated_at = now() WHERE id::text = $1`, id, v); err != nil {
			return err
		}
		details["assigned_to"] = *u.Assign
	}
	if u.Outcome != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE cases SET outcome = NULLIF($2, ''), updated_at = now() WHERE id::text = $1`,
			id, strings.TrimSpace(*u.Outcome)); err != nil {
			return err
		}
		details["outcome"] = strings.TrimSpace(*u.Outcome)
	}
	if len(details) == 0 {
		return nil
	}
	if err := r.Audit.Log(ctx, tx, a.entry("case.updated", "case", id, strings.TrimSpace(u.Reason), details)); err != nil {
		return err
	}
	return tx.Commit()
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// AddCaseNote adds a note; shared notes go to the buyer and seller's bell.
func (r *Records) AddCaseNote(ctx context.Context, a Actor, id, body string, internal bool) error {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > 4000 {
		return &FieldError{"body", "write a note of up to 4,000 characters"}
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var caseID string
	var cnum int64
	var buyer, seller string
	err = tx.QueryRowContext(ctx, `SELECT c.id, c.number, o.buyer_id, o.seller_id FROM cases c JOIN orders o ON o.id = c.order_id
		WHERE c.id::text = $1`, id).Scan(&caseID, &cnum, &buyer, &seller)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO case_notes (case_id, author_id, author_email, internal, body) VALUES ($1, $2, $3, $4, $5)`,
		caseID, a.ID, a.Email, internal, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return err
	}
	if !internal {
		preview := body
		if r := []rune(preview); len(r) > 160 {
			preview = string(r[:159]) + "…"
		}
		for _, p := range []string{buyer, seller} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body) VALUES ($1, 'order', $2, $3)`,
				p, fmt.Sprintf("Update on case %d", cnum), preview); err != nil {
				return err
			}
		}
	}
	if err := r.Audit.Log(ctx, tx, a.entry("case.note_added", "case", caseID, "", map[string]any{"shared": !internal})); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- Refunds and chargebacks ----

type RefundReport struct {
	ID          string    `json:"id"`
	OrderID     *string   `json:"order_id"`
	OrderNumber *int64    `json:"order_number"`
	Source      string    `json:"source"`
	Kind        string    `json:"kind"`
	AmountCents int       `json:"amount_cents"`
	StripeRef   *string   `json:"stripe_ref"`
	Note        string    `json:"note"`
	ReportedBy  string    `json:"reported_by"`
	Seller      string    `json:"seller"`
	CreatedAt   time.Time `json:"created_at"`
}

// Refunds lists refund and chargeback reports, optionally for one order or kind.
func (r *Records) Refunds(ctx context.Context, orderID, kind string, limit int) ([]RefundReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT f.id, f.order_id::text, o.number, f.source, f.kind, f.amount_cents, f.stripe_ref, f.note,
		       coalesce(u.username, ''), coalesce(s.username, ''), f.created_at
		FROM refund_reports f LEFT JOIN orders o ON o.id = f.order_id
		LEFT JOIN users u ON u.id = f.reported_by LEFT JOIN users s ON s.id = o.seller_id
		WHERE ($1 = '' OR f.order_id::text = $1) AND ($2 = '' OR f.kind = $2 OR ($2 = 'chargebacks' AND f.kind LIKE 'chargeback%'))
		ORDER BY f.created_at DESC LIMIT $3`, orderID, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RefundReport{}
	for rows.Next() {
		var f RefundReport
		var oid, ref sql.NullString
		var num sql.NullInt64
		if err := rows.Scan(&f.ID, &oid, &num, &f.Source, &f.Kind, &f.AmountCents, &ref, &f.Note, &f.ReportedBy, &f.Seller, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.OrderID, f.StripeRef = nstr(oid), nstr(ref)
		if num.Valid {
			f.OrderNumber = &num.Int64
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// RecordRefund records a refund or chargeback against an order (staff entry).
func (r *Records) RecordRefund(ctx context.Context, a Actor, orderID, kind string, amount int, ref, note string) error {
	if !contains([]string{"refund", "partial_refund", "chargeback", "chargeback_won", "chargeback_lost"}, kind) {
		return &FieldError{"kind", "refund, partial refund, or a chargeback outcome"}
	}
	if amount < 0 || amount > 1_000_000 {
		return &FieldError{"amount_cents", "enter the amount"}
	}
	note, err := needReason(note)
	if err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oid string
	var total, refunded int
	err = tx.QueryRowContext(ctx, `SELECT id, total_cents, refunded_cents FROM orders WHERE id::text = $1 OR number::text = $1 FOR UPDATE`,
		orderID).Scan(&oid, &total, &refunded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO refund_reports (order_id, reported_by, source, kind, amount_cents, stripe_ref, note)
		VALUES ($1, $2, 'staff', $3, $4, NULLIF($5, ''), $6)`, oid, a.ID, kind, amount, strings.TrimSpace(ref), note); err != nil {
		return err
	}
	if kind == "refund" || kind == "partial_refund" || kind == "chargeback_lost" {
		refunded = min(total, refunded+amount)
		status := "partially_refunded"
		if refunded >= total {
			status = "refunded"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET refunded_cents = $2, status = $3, updated_at = now() WHERE id = $1`,
			oid, refunded, status); err != nil {
			return err
		}
	}
	if err := r.Audit.Log(ctx, tx, a.entry("refund.recorded", "order", oid, note, map[string]any{"kind": kind, "amount_cents": amount, "stripe_ref": ref})); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- Feedback ----

type FeedbackRow struct {
	ID          string     `json:"id"`
	OrderID     string     `json:"order_id"`
	OrderNumber int64      `json:"order_number"`
	From        string     `json:"from"`
	FromID      string     `json:"from_id"`
	To          string     `json:"to"`
	ToID        string     `json:"to_id"`
	Role        string     `json:"role"`
	Rating      int        `json:"rating"`
	Comment     string     `json:"comment"`
	Reply       *string    `json:"reply"`
	CreatedAt   time.Time  `json:"created_at"`
	RemovedAt   *time.Time `json:"removed_at"`
	RemovedWhy  *string    `json:"removed_reason"`
	Reported    int        `json:"open_reports"`
	State       string     `json:"state"`
	Visible     bool       `json:"visible"`
	Scrubbed    *string    `json:"scrubbed_comment"`
	Item        *int       `json:"item_rating"`
	Shipping    *int       `json:"shipping_rating"`
	Comm        *int       `json:"communication_rating"`
	Auto        bool       `json:"auto"`
}

type FeedbackQuery struct {
	Filter  string // negative, removed, reported, mediation, held, scrubbed
	OrderID string
	User    string // username (given or received)
	Limit   int
}

func (r *Records) Feedback(ctx context.Context, q FeedbackQuery) ([]FeedbackRow, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 100
	}
	w := &whereBuilder{}
	switch q.Filter {
	case "negative":
		w.add("f.rating < 0 AND f.removed_at IS NULL")
	case "removed":
		w.add("f.removed_at IS NOT NULL")
	case "mediation":
		w.add("f.state = 'mediation' AND f.removed_at IS NULL")
	case "held":
		w.add("f.state = 'held' AND f.removed_at IS NULL")
	case "scrubbed":
		w.add("f.scrubbed_comment IS NOT NULL")
	case "reported":
		w.add("EXISTS (SELECT 1 FROM reports x WHERE x.target_type = 'user' AND x.target_id = f.from_user::text AND x.status = 'open')")
	}
	if q.OrderID != "" {
		w.add("f.order_id::text = ?", q.OrderID)
	}
	if q.User != "" {
		w.add("(lower(fu.username) = lower(?) OR lower(tu.username) = lower(?))", q.User, q.User)
	}
	args := append(w.args, q.Limit)
	rows, err := r.DB.QueryContext(ctx, `
		SELECT f.id, f.order_id, o.number, fu.username, fu.id, tu.username, tu.id, f.role, f.rating, f.comment, f.reply,
		       f.created_at, f.removed_at, f.removed_reason,
		       (SELECT count(*) FROM reports x WHERE x.target_type = 'user' AND x.target_id = f.from_user::text AND x.status = 'open'),
		       f.state, f.visible_at IS NOT NULL, f.scrubbed_comment, f.item_rating, f.shipping_rating, f.communication_rating, f.auto
		FROM feedback f JOIN orders o ON o.id = f.order_id
		JOIN users fu ON fu.id = f.from_user JOIN users tu ON tu.id = f.to_user`+w.sql()+
		fmt.Sprintf(" ORDER BY f.created_at DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeedbackRow{}
	for rows.Next() {
		var f FeedbackRow
		var reply, why sql.NullString
		var removed sql.NullTime
		if err := rows.Scan(&f.ID, &f.OrderID, &f.OrderNumber, &f.From, &f.FromID, &f.To, &f.ToID, &f.Role, &f.Rating, &f.Comment,
			&reply, &f.CreatedAt, &removed, &why, &f.Reported, &f.State, &f.Visible, &f.Scrubbed, &f.Item, &f.Shipping, &f.Comm,
			&f.Auto); err != nil {
			return nil, err
		}
		f.Reply, f.RemovedAt, f.RemovedWhy = nstr(reply), ntime(removed), nstr(why)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ModerateFeedback removes (or restores) feedback. Removed feedback stops
// counting toward seller levels and leaves the public profile.
func (r *Records) ModerateFeedback(ctx context.Context, a Actor, id, action, reason string) error {
	reason, err := needReason(reason)
	if err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var removed sql.NullTime
	var rating int
	var to, state string
	err = tx.QueryRowContext(ctx, `SELECT removed_at, rating, to_user, state FROM feedback WHERE id::text = $1 FOR UPDATE`, id).
		Scan(&removed, &rating, &to, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	switch action {
	case "remove":
		if removed.Valid {
			return &FieldError{"action", "already removed"}
		}
		_, err = tx.ExecContext(ctx, `UPDATE feedback SET removed_at = now(), removed_by = $2, removed_reason = $3 WHERE id::text = $1`, id, a.ID, reason)
	case "restore":
		if !removed.Valid {
			return &FieldError{"action", "it isn't removed"}
		}
		_, err = tx.ExecContext(ctx, `UPDATE feedback SET removed_at = NULL, removed_by = NULL, removed_reason = NULL WHERE id::text = $1`, id)
	case "publish":
		// Mediation done: the new seller's negative stands.
		if state != "mediation" || removed.Valid {
			return &FieldError{"action", "only feedback waiting in mediation can be published"}
		}
		_, err = tx.ExecContext(ctx, `UPDATE feedback SET state = 'published', updated_at = now() WHERE id::text = $1`, id)
	default:
		return &FieldError{"action", "remove, restore or publish"}
	}
	if err != nil {
		return err
	}
	if err := r.Audit.Log(ctx, tx, a.entry("feedback."+map[string]string{"remove": "removed", "restore": "restored", "publish": "published"}[action],
		"feedback", id, reason, map[string]any{"rating": rating, "to_user": to})); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- Message logs ----

type ThreadRow struct {
	ID           string    `json:"id"`
	ListingID    *string   `json:"listing_id"`
	ListingTitle string    `json:"listing_title"`
	Buyer        string    `json:"buyer"`
	BuyerID      string    `json:"buyer_id"`
	Seller       string    `json:"seller"`
	SellerID     string    `json:"seller_id"`
	Messages     int       `json:"messages"`
	Blocked      int       `json:"blocked"`
	LastAt       time.Time `json:"last_message_at"`
}

type ThreadQuery struct {
	User, Listing string
	BlockedOnly   bool
	Limit         int
}

// Threads lists conversations for review. It doesn't show message text.
func (r *Records) Threads(ctx context.Context, q ThreadQuery) ([]ThreadRow, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 100
	}
	w := &whereBuilder{}
	if q.User != "" {
		w.add("(lower(b.username) = lower(?) OR lower(s.username) = lower(?) OR b.id::text = ? OR s.id::text = ?)", q.User, q.User, q.User, q.User)
	}
	if q.Listing != "" {
		w.add("t.listing_id::text = ?", q.Listing)
	}
	if q.BlockedOnly {
		w.add("EXISTS (SELECT 1 FROM messages m WHERE m.thread_id = t.id AND m.blocked IS NOT NULL)")
	}
	args := append(w.args, q.Limit)
	rows, err := r.DB.QueryContext(ctx, `
		SELECT t.id, t.listing_id::text, coalesce(l.title, 'Removed listing'), b.username, b.id, s.username, s.id,
		       (SELECT count(*) FROM messages m WHERE m.thread_id = t.id),
		       (SELECT count(*) FROM messages m WHERE m.thread_id = t.id AND m.blocked IS NOT NULL),
		       t.last_message_at
		FROM message_threads t JOIN users b ON b.id = t.buyer_id JOIN users s ON s.id = t.seller_id
		LEFT JOIN listings l ON l.id = t.listing_id`+w.sql()+fmt.Sprintf(" ORDER BY t.last_message_at DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ThreadRow{}
	for rows.Next() {
		var t ThreadRow
		var lid sql.NullString
		if err := rows.Scan(&t.ID, &lid, &t.ListingTitle, &t.Buyer, &t.BuyerID, &t.Seller, &t.SellerID, &t.Messages, &t.Blocked, &t.LastAt); err != nil {
			return nil, err
		}
		t.ListingID = nstr(lid)
		out = append(out, t)
	}
	return out, rows.Err()
}

type LogMessage struct {
	ID      int64     `json:"id"`
	Sender  string    `json:"sender"`
	Role    string    `json:"role"` // buyer or seller
	Body    string    `json:"body"`
	Blocked *string   `json:"blocked"`
	At      time.Time `json:"at"`
}

// ThreadLog returns every message in a conversation, blocked ones too.
// Each view is written to the audit log with the reason given.
func (r *Records) ThreadLog(ctx context.Context, a Actor, id, reason string) (ThreadRow, []LogMessage, error) {
	reason, err := needReason(reason)
	if err != nil {
		return ThreadRow{}, nil, err
	}
	ts, err := r.threadByID(ctx, id)
	if err != nil {
		return ThreadRow{}, nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT m.id, u.username, CASE WHEN m.sender_id = t.buyer_id THEN 'buyer' ELSE 'seller' END,
		m.body, m.blocked, m.created_at FROM messages m JOIN message_threads t ON t.id = m.thread_id JOIN users u ON u.id = m.sender_id
		WHERE m.thread_id::text = $1 ORDER BY m.id`, id)
	if err != nil {
		return ts, nil, err
	}
	defer rows.Close()
	out := []LogMessage{}
	for rows.Next() {
		var m LogMessage
		var blocked sql.NullString
		if err := rows.Scan(&m.ID, &m.Sender, &m.Role, &m.Body, &blocked, &m.At); err != nil {
			return ts, nil, err
		}
		m.Blocked = nstr(blocked)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return ts, nil, err
	}
	if err := r.Audit.Log(ctx, nil, a.entry("messages.viewed", "thread", id, reason,
		map[string]any{"buyer": ts.Buyer, "seller": ts.Seller, "messages": len(out)})); err != nil {
		return ts, nil, err
	}
	return ts, out, nil
}

func (r *Records) threadByID(ctx context.Context, id string) (ThreadRow, error) {
	var t ThreadRow
	var lid sql.NullString
	err := r.DB.QueryRowContext(ctx, `
		SELECT t.id, t.listing_id::text, coalesce(l.title, 'Removed listing'), b.username, b.id, s.username, s.id,
		       (SELECT count(*) FROM messages m WHERE m.thread_id = t.id),
		       (SELECT count(*) FROM messages m WHERE m.thread_id = t.id AND m.blocked IS NOT NULL), t.last_message_at
		FROM message_threads t JOIN users b ON b.id = t.buyer_id JOIN users s ON s.id = t.seller_id
		LEFT JOIN listings l ON l.id = t.listing_id WHERE t.id::text = $1`, id).Scan(
		&t.ID, &lid, &t.ListingTitle, &t.Buyer, &t.BuyerID, &t.Seller, &t.SellerID, &t.Messages, &t.Blocked, &t.LastAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	t.ListingID = nstr(lid)
	return t, err
}
