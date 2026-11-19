package httpapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/postage"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

type fakePostage struct {
	track    []shipping.TrackingEvent
	test     bool
	n        int
	refunded []string
	rigid    bool
}

func (f *fakePostage) LetterRates(_ context.Context, l postage.Letter) (postage.Shipment, error) {
	f.n++
	f.rigid = l.NonMachinable
	rate := "0.78"
	if l.NonMachinable {
		rate = "1.27"
	}
	return postage.Shipment{ID: fmt.Sprintf("shp_%d", f.n), Rates: []postage.Rate{
		{ID: fmt.Sprintf("rate_ga_%d", f.n), Carrier: "USPS", Service: "GroundAdvantage", Rate: "5.65"},
		{ID: fmt.Sprintf("rate_%d", f.n), Carrier: "USPS", Service: "First", Rate: rate},
	}}, nil
}

func (f *fakePostage) Buy(_ context.Context, shipmentID, rateID string) (postage.Label, error) {
	return postage.Label{ShipmentID: shipmentID, Tracking: "9400100000000000000" + shipmentID[4:], ImageURL: "https://easypost-files.test/" + shipmentID + ".png"}, nil
}

func (f *fakePostage) Refund(_ context.Context, id string) error {
	f.refunded = append(f.refunded, id)
	return nil
}

func (f *fakePostage) TestMode() bool { return f.test }

func (f *fakePostage) Track(_ context.Context, _ string) (postage.Tracking, error) {
	return postage.Tracking{Events: f.track}, nil
}

func TestEnvelopePostage(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "post_seller", true)
	if _, err := e.db.Exec(`UPDATE users SET staff_role = $1 WHERE username = 'post_seller'`, admin.SuperAdmin); err != nil {
		t.Fatal(err)
	}
	enrollMFA(t, e.ts.URL, sc)
	if code, body := do(t, sc, "PUT", e.ts.URL+"/v1/seller/shipping/from", map[string]string{"name": "Sam", "line1": "1 Market St", "city": "San Francisco", "state": "CA", "zip": "94105"}); code != 200 {
		t.Fatalf("ship from: %d %v", code, body)
	}
	newSale := func() string {
		code, body := do(t, sc, "POST", e.ts.URL+"/v1/admin/test-sales", map[string]any{})
		if code != 201 {
			t.Fatalf("test sale: %d %v", code, body)
		}
		return body["test_sale"].(map[string]any)["id"].(string)
	}
	id := newSale()

	// Off until a key is set.
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/postage/quote", map[string]any{}); code != 503 {
		t.Fatalf("postage off: %d", code)
	}
	fp := &fakePostage{}
	e.co.Postage = fp

	// The listing form shows postage beside the stamp.
	_, body := do(t, sc, "GET", e.ts.URL+"/v1/seller/shipping/estimate?preset=envelope&weight_oz=1", nil)
	est := body["estimate"].(map[string]any)
	if est["postage_cents"] != float64(78) || est["postage_card_cents"] != float64(112) {
		t.Fatalf("estimate: %v", est)
	}

	code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/postage/quote", map[string]any{})
	if code != 200 {
		t.Fatalf("quote: %d %v", code, body)
	}
	q := body["quote"].(map[string]any)
	if q["cost_cents"] != float64(78) || q["card_charge_cents"] != float64(112) || q["balance_cents"] != float64(0) || q["rate_id"] != "rate_1" {
		t.Fatalf("quote: %v", q)
	}
	// No balance yet, and no saved card.
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/postage", map[string]string{"rate_id": "rate_1", "pay": "balance"}); code != 422 {
		t.Fatalf("empty balance: %d", code)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/postage/topup", map[string]int{"amount_cents": 1000}); code != 409 {
		t.Fatalf("top-up without card: %d", code)
	}
	e.db.Exec(`UPDATE seller_profiles sp SET stripe_customer_id = 'cus_p', label_payment_method = 'pm_p', label_card = 'Visa ending 4242'
		FROM users u WHERE u.id = sp.user_id AND u.username = 'post_seller'`)
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/postage/topup", map[string]int{"amount_cents": 100}); code != 422 {
		t.Fatalf("tiny top-up: %d", code)
	}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/seller/postage/topup", map[string]int{"amount_cents": 1000})
	if code != 200 || body["balance_cents"] != float64(1000) {
		t.Fatalf("top-up: %d %v", code, body)
	}
	charged := 0
	for _, c := range e.plat.Charges {
		charged += c
	}
	if charged != 1061 {
		t.Fatalf("top-up charged %d", charged)
	}

	// Pay from the balance: the order ships and the balance drops by the cost.
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/postage", map[string]string{"rate_id": "rate_1", "pay": "balance"})
	if code != 201 {
		t.Fatalf("buy: %d %v", code, body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/seller/postage", nil)
	if body["balance_cents"] != float64(922) || len(body["history"].([]any)) != 2 {
		t.Fatalf("account: %v", body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders/"+id, nil)
	o := body["order"].(map[string]any)
	sh := o["shipment"].(map[string]any)
	if o["status"] != "shipped" || sh["postage"] != true || sh["tracking_number"] != "94001000000000000001" {
		t.Fatalf("order: %v", o)
	}
	// The envelope page prints the postage.
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/envelope", map[string]any{})
	if code != 200 || body["envelope"].(map[string]any)["postage_url"] != "https://easypost-files.test/shp_1.png" {
		t.Fatalf("envelope: %d %v", code, body)
	}
	// Void: the money goes back to the balance and the order reopens.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/label/void", map[string]any{}); code != 200 && code != 204 {
		t.Fatalf("void: %d %v", code, body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/seller/postage", nil)
	if body["balance_cents"] != float64(1000) || len(fp.refunded) != 1 {
		t.Fatalf("after void: %v %v", body, fp.refunded)
	}

	// Pay by card, rigid envelope.
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/postage/quote", map[string]any{"rigid": true})
	if code != 200 || body["quote"].(map[string]any)["cost_cents"] != float64(127) || !fp.rigid {
		t.Fatalf("rigid quote: %d %v", code, body)
	}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/postage", map[string]string{"rate_id": "rate_2", "pay": "card"})
	if code != 201 || body["postage"].(map[string]any)["charge_cents"] != float64(162) {
		t.Fatalf("card buy: %d %v", code, body)
	}

	// With a Test key nothing is charged.
	fp.test = true
	id2 := newSale()
	_, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id2+"/postage/quote", map[string]any{})
	rate := body["quote"].(map[string]any)["rate_id"].(string)
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id2+"/postage", map[string]string{"rate_id": rate, "pay": "balance"})
	if code != 201 || body["postage"].(map[string]any)["paid_by"] != "test" {
		t.Fatalf("test buy: %d %v", code, body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/seller/postage", nil)
	if body["balance_cents"] != float64(1000) {
		t.Fatalf("test postage spent the balance: %v", body)
	}

	// Tracking: each milestone reaches the buyer's bell once.
	var buyer string
	var number int64
	if err := e.db.QueryRow(`SELECT buyer_id, number FROM orders WHERE id = $1`, id2).Scan(&buyer, &number); err != nil {
		t.Fatal(err)
	}
	titles := func() []string {
		rows, err := e.db.Query(`SELECT title FROM notifications WHERE user_id = $1 AND title LIKE $2 ORDER BY id`, buyer, fmt.Sprintf("Order #%d %%", number))
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			rows.Scan(&s)
			out = append(out, s)
		}
		return out
	}
	base := len(titles()) // "shipped"
	now := time.Now().Add(time.Minute)
	step := func(evs ...shipping.TrackingEvent) {
		fp.track = append(fp.track, evs...)
		e.db.Exec(`UPDATE shipments SET scans_checked_at = NULL`)
		if _, err := e.co.RefreshPostageTracking(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	step(shipping.TrackingEvent{Status: shipping.InTransit, Description: "Accepted at USPS Origin Facility", Location: "Austin TX", At: now})
	step(shipping.TrackingEvent{Status: shipping.InTransit, Description: "Arrived at USPS Regional Facility", Location: "Dallas TX", At: now.Add(time.Hour)})
	step(shipping.TrackingEvent{Status: shipping.InTransit, Description: "Out for Delivery", Location: "Plano TX", At: now.Add(2 * time.Hour)})
	step(shipping.TrackingEvent{Status: shipping.Delivered, Description: "Delivered, In/At Mailbox", Location: "Plano TX", At: now.Add(3 * time.Hour)})
	step() // nothing new
	got := titles()[base:]
	want := []string{
		fmt.Sprintf("Order #%d is on its way", number),
		fmt.Sprintf("Order #%d is out for delivery", number),
		fmt.Sprintf("Order #%d was delivered", number),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("tracking notices: %v", got)
	}
	var status string
	e.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, id2).Scan(&status)
	if status != "delivered" {
		t.Fatalf("order status after delivery: %s", status)
	}

	// A paid order past its ship-by date: the buyer and seller hear once.
	id3 := newSale()
	e.db.Exec(`UPDATE orders SET ship_by = now() - interval '1 hour' WHERE id = $1`, id3)
	for i := 0; i < 2; i++ {
		if _, err := e.co.NotifyLateShipments(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var lateBuyer, lateSeller int
	e.db.QueryRow(`SELECT count(*) FILTER (WHERE title LIKE '%hasn''t shipped yet'), count(*) FILTER (WHERE title LIKE '%is late')
		FROM notifications n JOIN orders o ON o.id = $1 WHERE n.link IN ('/purchases/' || o.id, '/sell/orders/' || o.id)`, id3).Scan(&lateBuyer, &lateSeller)
	if lateBuyer != 1 || lateSeller != 1 {
		t.Fatalf("late notices: buyer %d seller %d", lateBuyer, lateSeller)
	}
}
