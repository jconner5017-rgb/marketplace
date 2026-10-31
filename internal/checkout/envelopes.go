package checkout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/imb"
	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
	"github.com/jconner5017-rgb/marketplace/internal/usps"
)

// LetterService marks a shipment tracked by its Intelligent Mail barcode.
const LetterService = "First-Class letter (Intelligent Mail)"

var (
	ErrEnvelopeOff  = errors.New("envelope tracking isn't set up yet")
	ErrNotEnvelope  = errors.New("this order ships as a package; buy a label instead")
	ErrNoShipToZIP  = errors.New("this order has no delivery ZIP code")
	envelopeStates  = map[string]bool{"paid": true, "shipped": true}
	addressLookupTO = 6 * time.Second
)

// AddressLookup finds the ZIP+4 and delivery point for an address.
type AddressLookup interface {
	Standardize(ctx context.Context, street, secondary, city, state, zip string) (usps.Standard, error)
}

// PieceTracker reads USPS scans for one barcode.
type PieceTracker interface {
	Piece(ctx context.Context, imb string) (usps.Piece, error)
}

// Letters configures envelope barcodes; a blank MailerID turns them off.
type Letters struct {
	MailerID    string // 9-digit USPS Mailer ID
	ServiceType int    // service type ID; 40 = First-Class Mail, basic tracing
	Addresses   AddressLookup
	Tracker     PieceTracker
	// Warn logs lookups that fell back to the plain ZIP (optional).
	Warn func(msg string, args ...any)
}

// Envelope is everything needed to print a tracked envelope.
type Envelope struct {
	OrderID       string            `json:"order_id"`
	OrderNumber   int64             `json:"order_number"`
	To            shipping.Address  `json:"to"`
	From          *shipping.Address `json:"from"`
	Barcode       string            `json:"barcode"`        // 20-31 digits, no spaces
	HumanReadable string            `json:"human_readable"` // spaced for printing under the bars
	Bars          string            `json:"bars"`           // 65 of F, A, D, T
	SVG           string            `json:"svg"`
	FullZIP       bool              `json:"full_zip"` // routed to ZIP+4 (sorts better)
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// zipRouting is the routing code from a typed ZIP: ZIP+4 when given.
func zipRouting(zip string) string {
	d := digitsOnly(zip)
	switch {
	case len(d) >= 9:
		return d[:9]
	case len(d) >= 5:
		return d[:5]
	}
	return ""
}

// EnvelopeFor returns the order's barcode, creating it the first time.
func (s *Service) EnvelopeFor(ctx context.Context, sellerID, orderID string) (Envelope, error) {
	if s.Letters.MailerID == "" {
		return Envelope{}, ErrEnvelopeOff
	}
	var e Envelope
	var status, method string
	var shipTo []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id, number, status, ship_method, ship_to FROM orders
		WHERE (id::text = $2 OR number::text = $2) AND seller_id = $1`, sellerID, orderID).
		Scan(&e.OrderID, &e.OrderNumber, &status, &method, &shipTo)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	if method != "envelope" {
		return e, ErrNotEnvelope
	}
	if !envelopeStates[status] {
		return e, ErrState
	}
	_ = json.Unmarshal(shipTo, &e.To)

	var tracking, routing string
	err = s.DB.QueryRowContext(ctx, `SELECT tracking, routing FROM envelope_barcodes WHERE order_id = $1`, e.OrderID).
		Scan(&tracking, &routing)
	if errors.Is(err, sql.ErrNoRows) {
		routing = zipRouting(e.To.Zip)
		if s.Letters.Addresses != nil && e.To.Street1 != "" {
			lctx, cancel := context.WithTimeout(ctx, addressLookupTO)
			std, lerr := s.Letters.Addresses.Standardize(lctx, e.To.Street1, e.To.Street2, e.To.City, e.To.State, e.To.Zip)
			cancel()
			switch {
			case lerr != nil:
				if s.Letters.Warn != nil {
					s.Letters.Warn("usps address lookup failed; using typed ZIP", "order", e.OrderNumber, "err", lerr)
				}
			case len(std.Routing()) > len(routing):
				routing = std.Routing()
			}
			if lerr == nil && s.Letters.Warn != nil && len(routing) < 9 {
				s.Letters.Warn("usps address lookup found no ZIP+4", "order", e.OrderNumber)
			}
		}
		if routing == "" {
			return e, ErrNoShipToZIP
		}
		var serial int
		if err := s.DB.QueryRowContext(ctx, `SELECT nextval('imb_serial_seq')`).Scan(&serial); err != nil {
			return e, err
		}
		tracking, err = imb.Tracking(0, s.Letters.ServiceType, s.Letters.MailerID, serial)
		if err != nil {
			return e, err
		}
		// Two tabs printing at once: the first insert wins and both reread it.
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO envelope_barcodes (order_id, tracking, routing) VALUES ($1, $2, $3)
			ON CONFLICT (order_id) DO NOTHING`, e.OrderID, tracking, routing); err != nil {
			return e, err
		}
		err = s.DB.QueryRowContext(ctx, `SELECT tracking, routing FROM envelope_barcodes WHERE order_id = $1`, e.OrderID).
			Scan(&tracking, &routing)
	}
	if err != nil {
		return e, err
	}

	e.Bars, err = imb.Encode(tracking, routing)
	if err != nil {
		return e, err
	}
	e.Barcode, e.HumanReadable, e.SVG = tracking+routing, imb.HumanReadable(tracking, routing), imb.SVG(e.Bars)
	e.FullZIP = len(routing) >= 9

	var from sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT ship_from FROM seller_profiles WHERE user_id = $1`, sellerID).Scan(&from); err == nil && from.Valid {
		var a shipping.Address
		if json.Unmarshal([]byte(from.String), &a) == nil && a.Street1 != "" {
			e.From = &a
		}
	}
	return e, nil
}

// envelopeBarcode is the order's printed barcode, if it has one.
func envelopeBarcode(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, orderID string) (string, bool) {
	var t, r string
	if err := q.QueryRowContext(ctx, `SELECT tracking, routing FROM envelope_barcodes WHERE order_id = $1`, orderID).Scan(&t, &r); err != nil {
		return "", false
	}
	return t + r, true
}

// RefreshLetterScans asks USPS for new scans on tracked envelopes sent in
// the last 30 days and records them on the shipment. It returns how many
// shipments changed.
func (s *Service) RefreshLetterScans(ctx context.Context) (int, error) {
	if s.Letters.Tracker == nil {
		return 0, nil
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT sh.id, sh.tracking_number, jsonb_array_length(sh.events), o.id, o.buyer_id, o.number
		FROM shipments sh JOIN orders o ON o.id = sh.order_id
		WHERE sh.service = $1 AND sh.voided_at IS NULL AND sh.status = 'in_transit'
		  AND sh.created_at > now() - interval '30 days'
		  AND (sh.scans_checked_at IS NULL OR sh.scans_checked_at < now() - interval '50 minutes')
		ORDER BY sh.scans_checked_at NULLS FIRST LIMIT 300`, LetterService)
	if err != nil {
		return 0, err
	}
	type job struct {
		id, barcode, orderID, buyer string
		had                         int
		number                      int64
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.barcode, &j.had, &j.orderID, &j.buyer, &j.number); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()

	changed := 0
	for _, j := range jobs {
		p, err := s.Letters.Tracker.Piece(ctx, j.barcode)
		if err != nil {
			return changed, fmt.Errorf("scans for order %d: %w", j.number, err)
		}
		events := letterEvents(p)
		if len(events) <= j.had {
			if _, err := s.DB.ExecContext(ctx, `UPDATE shipments SET scans_checked_at = now() WHERE id = $1`, j.id); err != nil {
				return changed, err
			}
			continue
		}
		b, _ := json.Marshal(events)
		last := events[len(events)-1]
		at, _ := time.Parse(time.RFC3339, last["at"].(string))
		if _, err := s.DB.ExecContext(ctx, `UPDATE shipments SET events = $2::jsonb, last_event = $3, last_event_at = $4,
			scans_checked_at = now(), updated_at = now() WHERE id = $1`, j.id, string(b), last["description"], at); err != nil {
			return changed, err
		}
		changed++
		if j.had == 0 {
			if err := inbox.Notify(ctx, s.DB, j.buyer, "order", fmt.Sprintf("Order #%d is moving through USPS", j.number),
				last["description"].(string), "/purchases/"+j.orderID); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

// letterEvents turns USPS scans into the shipment's event list, oldest
// first, in the same shape as package tracking events.
func letterEvents(p usps.Piece) []map[string]any {
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, sc := range p.Scans {
		t, ok := usps.ParseTime(sc.DateTime)
		if !ok {
			continue
		}
		d := sc.Describe()
		key := t.UTC().Format(time.RFC3339) + d
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, map[string]any{"at": t.UTC().Format(time.RFC3339), "status": "in_transit", "description": d,
			"location": strings.TrimSpace(sc.City + " " + sc.State)})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a]["at"].(string) < out[b]["at"].(string) })
	return out
}
