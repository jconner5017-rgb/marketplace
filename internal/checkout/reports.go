package checkout

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"strconv"
	"time"
)

// SalesRow is one order in a seller's sales report.
type SalesRow struct {
	Number        int64
	PaidAt        time.Time
	Status        string
	Cards         int
	GrossCents    int // cards before discount
	DiscountCents int
	TaxCents      int
	FeeCents      int
	FeeRefunded   int
	LabelCents    int
	RefundedCents int
	ShipState     string
	ShipZip       string
}

// Net is what the seller keeps before Stripe's processing fee (tax is passed through).
func (r SalesRow) Net() int {
	return r.GrossCents - r.DiscountCents - r.FeeCents + r.FeeRefunded - r.LabelCents - r.RefundedCents
}

func (s *Service) salesRows(ctx context.Context, sellerID string, from, to time.Time) ([]SalesRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT o.number, o.paid_at, o.status,
		       (SELECT coalesce(sum(quantity), 0) FROM order_items WHERE order_id = o.id),
		       o.subtotal_cents, o.discount_cents, o.tax_cents, o.fee_cents,
		       (SELECT coalesce(sum(amount_cents), 0) FROM ledger_entries WHERE order_id = o.id AND kind = 'fee_refund'),
		       (SELECT coalesce(sum(amount_cents), 0) FROM ledger_entries WHERE order_id = o.id AND kind = 'label_cost'),
		       o.refunded_cents, coalesce(o.ship_to->>'state', ''), coalesce(o.ship_to->>'zip', '')
		FROM orders o
		WHERE o.seller_id = $1 AND o.paid_at >= $2 AND o.paid_at < $3
		ORDER BY o.paid_at`, sellerID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SalesRow
	for rows.Next() {
		var r SalesRow
		if err := rows.Scan(&r.Number, &r.PaidAt, &r.Status, &r.Cards, &r.GrossCents, &r.DiscountCents, &r.TaxCents,
			&r.FeeCents, &r.FeeRefunded, &r.LabelCents, &r.RefundedCents, &r.ShipState, &r.ShipZip); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dollars(c int) string { return fmt.Sprintf("%d.%02d", c/100, abs(c%100)) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// SalesCSV writes a seller's orders paid in [from, to) as CSV, one row per
// order, with the buyer's ship-to state for destination-based sales tax.
func (s *Service) SalesCSV(ctx context.Context, w io.Writer, sellerID string, from, to time.Time) error {
	rows, err := s.salesRows(ctx, sellerID, from, to)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Write([]string{"order", "paid_at", "status", "cards", "gross_sales", "discounts", "sales_tax_collected",
		"platform_fee", "fee_refunded", "label_cost", "refunds", "net", "ship_to_state", "ship_to_zip"})
	for _, r := range rows {
		cw.Write([]string{strconv.FormatInt(r.Number, 10), r.PaidAt.UTC().Format(time.RFC3339), r.Status, strconv.Itoa(r.Cards),
			dollars(r.GrossCents), dollars(r.DiscountCents), dollars(r.TaxCents), dollars(r.FeeCents), dollars(r.FeeRefunded),
			dollars(r.LabelCents), dollars(r.RefundedCents), dollars(r.Net()), r.ShipState, r.ShipZip})
	}
	cw.Flush()
	return cw.Error()
}

// MonthTotals is one month of a seller's annual summary.
type MonthTotals struct {
	Month         string `json:"month"` // 2026-01
	Orders        int    `json:"orders"`
	Cards         int    `json:"cards"`
	GrossCents    int    `json:"gross_cents"`
	DiscountCents int    `json:"discount_cents"`
	TaxCents      int    `json:"tax_cents"`
	FeeCents      int    `json:"fee_cents"`
	LabelCents    int    `json:"label_cents"`
	RefundedCents int    `json:"refunded_cents"`
	NetCents      int    `json:"net_cents"`
}

// Summary totals a seller's year by month (US Eastern), plus a year line.
func (s *Service) Summary(ctx context.Context, sellerID string, year int) ([]MonthTotals, MonthTotals, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	from := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	rows, err := s.salesRows(ctx, sellerID, from, from.AddDate(1, 0, 0))
	if err != nil {
		return nil, MonthTotals{}, err
	}
	months := make([]MonthTotals, 12)
	for i := range months {
		months[i].Month = fmt.Sprintf("%d-%02d", year, i+1)
	}
	total := MonthTotals{Month: strconv.Itoa(year)}
	for _, r := range rows {
		for _, m := range []*MonthTotals{&months[r.PaidAt.In(loc).Month()-1], &total} {
			m.Orders++
			m.Cards += r.Cards
			m.GrossCents += r.GrossCents
			m.DiscountCents += r.DiscountCents
			m.TaxCents += r.TaxCents
			m.FeeCents += r.FeeCents - r.FeeRefunded
			m.LabelCents += r.LabelCents
			m.RefundedCents += r.RefundedCents
			m.NetCents += r.Net()
		}
	}
	return months, total, nil
}

var evidencePage = template.Must(template.New("e").Funcs(template.FuncMap{
	"money": money,
	"when":  func(t time.Time) string { return t.UTC().Format("Jan 2, 2006 15:04 MST") },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Order #{{.Number}} evidence</title>
<style>
body{font:14px/1.5 system-ui,-apple-system,Segoe UI,sans-serif;max-width:52rem;margin:2rem auto;padding:0 1rem;color:#111}
h1{font-size:22px;margin:0}h2{font-size:17px;margin:28px 0 6px;border-bottom:1px solid #ddd;padding-bottom:4px}
table{border-collapse:collapse;width:100%}td,th{text-align:left;padding:4px 8px 4px 0;vertical-align:top;border-bottom:1px solid #eee}
.muted{color:#666}.msg{border-left:3px solid #ddd;padding:2px 10px;margin:8px 0}.blocked{color:#a00}
img{max-height:180px;margin:4px 8px 4px 0;border:1px solid #eee}
@media print{a{color:#111}}
</style></head><body>
<h1>Order #{{.Number}} — evidence packet</h1>
<p class="muted">Prepared {{when .Generated}} by Slabs &amp; Singles for the seller's chargeback response. Listings are shown as they were when the buyer paid.</p>

<h2>Order</h2>
<table>
<tr><th>Order</th><td>#{{.Number}} ({{.ID}})</td></tr>
<tr><th>Buyer</th><td>{{.Buyer}}</td></tr>
<tr><th>Seller</th><td>{{.Seller}}</td></tr>
<tr><th>Placed</th><td>{{when .CreatedAt}}</td></tr>
{{with .PaidAt}}<tr><th>Paid</th><td>{{when .}}</td></tr>{{end}}
<tr><th>Status</th><td>{{.Status}}</td></tr>
<tr><th>Cards</th><td>{{money .Subtotal}}</td></tr>
{{if .Discount}}<tr><th>Discount</th><td>−{{money .Discount}}</td></tr>{{end}}
{{if .Tax}}<tr><th>Sales tax</th><td>{{money .Tax}}</td></tr>{{end}}
<tr><th>Total paid</th><td>{{money .Paid}}</td></tr>
{{if .Refunded}}<tr><th>Refunded</th><td>{{money .Refunded}}</td></tr>{{end}}
<tr><th>Ship to</th><td>{{.ShipTo}}</td></tr>
<tr><th>Stripe payment</th><td>{{.PaymentIntent}}</td></tr>
</table>

<h2>Cards sold</h2>
{{range .Items}}
<div style="margin:12px 0">
<p><strong>{{.Title}}</strong> — {{.Quantity}} × {{money .Unit}} ({{.Source}})</p>
{{if .Snapshot}}<table>
{{range .Snapshot}}<tr><th>{{.K}}</th><td>{{.V}}</td></tr>{{end}}
</table>{{end}}
{{if .Description}}<p style="white-space:pre-line">{{.Description}}</p>{{end}}
{{range .Photos}}<a href="{{.}}"><img src="{{.}}" alt=""></a>{{end}}
</div>
{{end}}

<h2>Shipping and tracking</h2>
{{range .Shipments}}
<p>{{.Method}}{{with .Carrier}} · {{.}}{{end}}{{with .Tracking}} · tracking <strong>{{.}}</strong>{{end}} · marked shipped {{when .Created}} · status {{.Status}}</p>
{{if .Events}}<table>{{range .Events}}<tr><td>{{.}}</td></tr>{{end}}</table>{{end}}
{{else}}<p class="muted">No shipment recorded.</p>{{end}}
{{with .DeliveredAt}}<p>Buyer confirmed delivery {{when .}}.</p>{{end}}

<h2>Messages between buyer and seller</h2>
{{range .Messages}}
<div class="msg"><p class="muted">{{.From}} · {{when .At}}{{if .Blocked}} · <span class="blocked">not delivered ({{.Blocked}})</span>{{end}}{{with .Listing}} · about “{{.}}”{{end}}</p>
<p style="white-space:pre-line">{{.Body}}</p></div>
{{else}}<p class="muted">No messages.</p>{{end}}

<h2>Feedback</h2>
{{range .Feedback}}<p>{{.}}</p>{{else}}<p class="muted">None.</p>{{end}}

<h2>Money events</h2>
<table><tr><th>When</th><th>Event</th><th>Amount</th><th>Stripe</th></tr>
{{range .Ledger}}<tr><td>{{.CreatedAt}}</td><td>{{.Kind}}</td><td>{{money .AmountCents}}</td><td>{{.StripeObject}}</td></tr>{{end}}
</table>
</body></html>`))

type kv struct{ K, V string }

type evidenceItem struct {
	Title, Source, Description string
	Unit, Quantity             int
	Snapshot                   []kv
	Photos                     []string
}

type evidenceShipment struct {
	Method, Status    string
	Carrier, Tracking *string
	Created           time.Time
	Events            []string
}

type evidenceMessage struct {
	From, Body, Blocked, Listing string
	At                           time.Time
}

// Evidence writes a printable, self-contained HTML record of an order for
// the seller's chargeback response. staff=true skips the seller check.
func (s *Service) Evidence(ctx context.Context, w io.Writer, userID, orderID string, staff bool) error {
	var d struct {
		ID, Buyer, Seller, Status, ShipTo, PaymentIntent string
		Number                                           int64
		Subtotal, Discount, Tax, Paid, Refunded          int
		CreatedAt, Generated                             time.Time
		PaidAt, DeliveredAt                              *time.Time
		Items                                            []evidenceItem
		Shipments                                        []evidenceShipment
		Messages                                         []evidenceMessage
		Feedback                                         []string
		Ledger                                           []LedgerEntry
	}
	var buyerID, sellerID string
	var shipTo []byte
	var pi sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT o.id, o.number, o.buyer_id, o.seller_id, bu.username, su.username, o.status, o.subtotal_cents, o.discount_cents,
		       o.tax_cents, o.total_cents + o.tax_cents, o.refunded_cents, o.ship_to, o.stripe_payment_intent, o.created_at,
		       o.paid_at, o.delivered_at
		FROM orders o JOIN users bu ON bu.id = o.buyer_id JOIN users su ON su.id = o.seller_id
		WHERE (o.id::text = $1 OR o.number::text = $1) AND o.paid_at IS NOT NULL AND ($3 OR o.seller_id::text = $2)`,
		orderID, userID, staff).Scan(&d.ID, &d.Number, &buyerID, &sellerID, &d.Buyer, &d.Seller, &d.Status, &d.Subtotal,
		&d.Discount, &d.Tax, &d.Paid, &d.Refunded, &shipTo, &pi, &d.CreatedAt, &d.PaidAt, &d.DeliveredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	d.Generated, d.PaymentIntent = s.now(), pi.String
	var addr map[string]string
	_ = json.Unmarshal(shipTo, &addr)
	d.ShipTo = fmt.Sprintf("%s, %s %s, %s, %s %s", addr["name"], addr["line1"], addr["line2"], addr["city"], addr["state"], addr["zip"])

	rows, err := s.DB.QueryContext(ctx, `
		SELECT oi.title, oi.source, oi.unit_cents, oi.quantity, lv.snapshot
		FROM order_items oi LEFT JOIN listing_versions lv ON lv.listing_id = oi.listing_id AND lv.version = oi.listing_version
		WHERE oi.order_id = $1 ORDER BY oi.id`, d.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var it evidenceItem
		var snap []byte
		if err := rows.Scan(&it.Title, &it.Source, &it.Unit, &it.Quantity, &snap); err != nil {
			rows.Close()
			return err
		}
		var l struct {
			SetName     string   `json:"set_name"`
			Year        *int     `json:"year"`
			CardNumber  *string  `json:"card_number"`
			Condition   *string  `json:"condition"`
			Grader      *string  `json:"grader"`
			Grade       *float64 `json:"grade"`
			CertNumber  *string  `json:"cert_number"`
			Description string   `json:"description"`
			PhotoURLs   []string `json:"photo_urls"`
			Version     int      `json:"version"`
		}
		if len(snap) > 0 && json.Unmarshal(snap, &l) == nil {
			add := func(k string, v *string) {
				if v != nil && *v != "" {
					it.Snapshot = append(it.Snapshot, kv{k, *v})
				}
			}
			add("Set", &l.SetName)
			if l.Year != nil {
				y := strconv.Itoa(*l.Year)
				add("Year", &y)
			}
			add("Card number", l.CardNumber)
			add("Condition", l.Condition)
			add("Grader", l.Grader)
			if l.Grade != nil {
				g := strconv.FormatFloat(*l.Grade, 'f', -1, 64)
				add("Grade", &g)
			}
			add("Cert number", l.CertNumber)
			v := strconv.Itoa(l.Version)
			add("Listing version", &v)
			it.Description, it.Photos = l.Description, l.PhotoURLs
		}
		d.Items = append(d.Items, it)
	}
	rows.Close()

	rows, err = s.DB.QueryContext(ctx, `SELECT method, carrier, tracking_number, status, created_at, events
		FROM shipments WHERE order_id = $1 ORDER BY created_at`, d.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var sh evidenceShipment
		var ev []byte
		if err := rows.Scan(&sh.Method, &sh.Carrier, &sh.Tracking, &sh.Status, &sh.Created, &ev); err != nil {
			rows.Close()
			return err
		}
		var events []map[string]any
		_ = json.Unmarshal(ev, &events)
		for _, e := range events {
			sh.Events = append(sh.Events, fmt.Sprintf("%v · %v · %v", e["at"], e["status"], e["description"]))
		}
		d.Shipments = append(d.Shipments, sh)
	}
	rows.Close()

	rows, err = s.DB.QueryContext(ctx, `
		SELECT u.username, m.body, coalesce(m.blocked, ''), coalesce(l.title, ''), m.created_at
		FROM message_threads t JOIN messages m ON m.thread_id = t.id JOIN users u ON u.id = m.sender_id
		LEFT JOIN listings l ON l.id = t.listing_id
		WHERE t.buyer_id = $1 AND t.seller_id = $2 ORDER BY m.created_at`, buyerID, sellerID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m evidenceMessage
		if err := rows.Scan(&m.From, &m.Body, &m.Blocked, &m.Listing, &m.At); err != nil {
			rows.Close()
			return err
		}
		d.Messages = append(d.Messages, m)
	}
	rows.Close()

	rows, err = s.DB.QueryContext(ctx, `SELECT u.username, f.rating, f.comment, f.created_at FROM feedback f
		JOIN users u ON u.id = f.from_user WHERE f.order_id = $1 AND f.removed_at IS NULL ORDER BY f.created_at`, d.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var who, c string
		var r int
		var at time.Time
		if err := rows.Scan(&who, &r, &c, &at); err != nil {
			rows.Close()
			return err
		}
		d.Feedback = append(d.Feedback, fmt.Sprintf("%s: %s, “%s” (%s)", who, map[int]string{1: "positive", 0: "neutral", -1: "negative"}[r], c,
			at.UTC().Format("Jan 2, 2006")))
	}
	rows.Close()

	if d.Ledger, err = s.Ledger(ctx, d.ID); err != nil {
		return err
	}
	return evidencePage.Execute(w, d)
}
