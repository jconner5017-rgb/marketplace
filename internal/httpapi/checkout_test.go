package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

func listingQty(t *testing.T, e *env, id string) (int, string) {
	t.Helper()
	var q int
	var st string
	if err := e.db.QueryRow(`SELECT quantity, status FROM listings WHERE id = $1`, id).Scan(&q, &st); err != nil {
		t.Fatal(err)
	}
	return q, st
}

func sessionOf(t *testing.T, e *env, orderID string) string {
	t.Helper()
	var cs string
	if err := e.db.QueryRow(`SELECT stripe_checkout_id FROM orders WHERE id = $1`, orderID).Scan(&cs); err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestCheckoutFromCart(t *testing.T) {
	e := setupEnv(t)
	sc, acct := newSeller(t, e, "pay_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3`)
	do(t, sc, "PATCH", e.ts.URL+"/v1/seller/settings", map[string]any{"bundle_discount_type": "per_extra_item", "bundle_discount_val": 50})
	a := createListing(t, e, &httpClientT{sc}, rawCard("P1", 1200))["id"].(string)
	card := rawCard("P2", 20000)
	card["quantity"] = 3
	b2 := createListing(t, e, &httpClientT{sc}, card)["id"].(string)

	pat := buyer(t, e, "pat")
	do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/"+a, map[string]int{"quantity": 1})
	do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/"+b2, map[string]int{"quantity": 2})

	// Switched off: status says so and starting fails.
	e.co.Enabled = false
	if _, body := do(t, client(t), "GET", e.ts.URL+"/v1/checkout/status", nil); body["enabled"] != false {
		t.Fatalf("status off: %v", body)
	}
	if code, _ := do(t, pat.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "pay_shop"}); code != 503 {
		t.Fatalf("disabled checkout: %d", code)
	}
	e.co.Enabled = true
	if _, body := do(t, client(t), "GET", e.ts.URL+"/v1/checkout/status", nil); body["enabled"] != true {
		t.Fatalf("status on: %v", body)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "nobody"}); code != 409 {
		t.Fatalf("empty seller: %d", code)
	}

	code, body := do(t, pat.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "PAY_SHOP"})
	if code != 201 || !strings.HasPrefix(body["url"].(string), "https://checkout.stripe.test/") {
		t.Fatalf("start: %d %v", code, body)
	}
	orderID := body["order_id"].(string)
	cs := sessionOf(t, e, orderID)
	p := e.pay.Params(cs)
	// $12 + 2 x $200 = $412, less $0.50 bundle discount for 2 extra cards = $411.
	// Fee: $1.50 + 2 x 3% of $200 ($6) = $13.50.
	if p.FeeCents != 1350 || len(p.Lines) != 1 || p.Lines[0].UnitCents != 41100 ||
		!strings.Contains(p.SuccessURL, "https://shop.test/checkout/done?order="+orderID) {
		t.Fatalf("stripe params: %+v", p)
	}
	if q, st := listingQty(t, e, a); q != 0 || st != "sold_out" {
		t.Fatalf("reserved: %d %s", q, st)
	}
	if q, _ := listingQty(t, e, b2); q != 1 {
		t.Fatalf("reserved qty: %d", q)
	}
	// Another buyer can't take the reserved card.
	other := buyer(t, e, "other")
	if code, _ := do(t, other.c, "PUT", e.ts.URL+"/v1/cart/"+a, map[string]int{"quantity": 1}); code == 200 {
		t.Fatal("reserved card still buyable")
	}

	// Coming back before paying changes nothing.
	if _, body := do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/confirm?session_id="+cs, map[string]any{}); body["status"] != "pending_payment" {
		t.Fatalf("confirm unpaid: %v", body)
	}
	if code, _ := do(t, other.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/confirm?session_id="+cs, map[string]any{}); code != 404 {
		t.Fatalf("someone else's order: %d", code)
	}
	if code, _ := do(t, pat.c, "GET", e.ts.URL+"/v1/orders/"+orderID, nil); code != 404 {
		t.Fatalf("unpaid order shown: %d", code)
	}

	e.pay.Complete(cs)
	for i := 0; i < 2; i++ {
		if _, body := do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/confirm?session_id="+cs, map[string]any{}); body["status"] != "paid" {
			t.Fatalf("confirm paid: %v", body)
		}
	}
	var shipTo string
	var shipBy time.Time
	e.db.QueryRow(`SELECT ship_to::text, ship_by FROM orders WHERE id = $1`, orderID).Scan(&shipTo, &shipBy)
	if !strings.Contains(shipTo, "Austin") || shipBy.Before(time.Now().Add(72*time.Hour)) {
		t.Fatalf("ship to / by: %s %v", shipTo, shipBy)
	}
	_, cart := do(t, pat.c, "GET", e.ts.URL+"/v1/cart", nil)
	if len(cart["groups"].([]any)) != 0 {
		t.Fatalf("cart should be empty: %v", cart)
	}
	var notes int
	e.db.QueryRow(`SELECT count(*) FROM notifications WHERE title LIKE 'New order #%'`).Scan(&notes)
	if notes != 1 {
		t.Fatalf("seller notices: %d", notes)
	}

	// Seller sees it to ship, with the fee; buyer sees it without.
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders?side=selling&status=to_ship", nil)
	os := body["orders"].([]any)
	if len(os) != 1 || os[0].(map[string]any)["fee_cents"].(float64) != 1350 || len(os[0].(map[string]any)["lines"].([]any)) != 2 {
		t.Fatalf("seller orders: %v", body)
	}
	_, body = do(t, pat.c, "GET", e.ts.URL+"/v1/orders", nil)
	if o := body["orders"].([]any)[0].(map[string]any); o["fee_cents"] != nil || o["side"] != "buying" {
		t.Fatalf("buyer orders: %v", o)
	}

	// Feedback waits for shipping; label orders need tracking.
	if code, _ := do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/feedback", map[string]any{"rating": 1, "comment": "great"}); code != 409 {
		t.Fatalf("feedback before shipping: %d", code)
	}
	if code, _ := do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/ship", map[string]any{"carrier": "USPS", "tracking_number": "9400111899223344556677"}); code != 404 {
		t.Fatalf("buyer can't ship: %d", code)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+orderID+"/ship", map[string]any{"carrier": "USPS"}); code != 422 {
		t.Fatalf("no tracking: %d", code)
	}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+orderID+"/ship", map[string]any{"carrier": "USPS", "tracking_number": "9400 1118 9922 3344 5566 77"})
	if code != 200 || body["order"].(map[string]any)["status"] != "shipped" {
		t.Fatalf("ship: %d %v", code, body)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+orderID+"/ship", map[string]any{"carrier": "USPS", "tracking_number": "9400111899223344556677"}); code != 409 {
		t.Fatalf("ship twice: %d", code)
	}
	if code, _ := do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/feedback", map[string]any{"rating": -1, "comment": ""}); code != 422 {
		t.Fatalf("negative needs a reason: %d", code)
	}
	code, body = do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/feedback", map[string]any{"rating": 1, "comment": "Fast and well packed"})
	if code != 200 || body["order"].(map[string]any)["my_feedback"] == nil {
		t.Fatalf("feedback: %d %v", code, body)
	}
	code, body = do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/delivered", map[string]any{})
	if code != 200 || body["order"].(map[string]any)["status"] != "delivered" {
		t.Fatalf("delivered: %d %v", code, body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders/"+orderID, nil)
	if body["order"].(map[string]any)["their_feedback"] == nil {
		t.Fatalf("seller sees feedback: %v", body)
	}

	// Report a problem: opens one case for staff.
	code, body = do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/problem", map[string]any{"kind": "snad", "summary": "The corner is creased, not LP as listed."})
	if code != 201 || body["case_number"].(float64) < 5001 {
		t.Fatalf("problem: %d %v", code, body)
	}
	if code, _ := do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/problem", map[string]any{"kind": "other", "summary": "Another problem with it"}); code != 422 {
		t.Fatalf("second open case: %d", code)
	}

	// A second order, paid but confirmed only by the webhook.
	do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/"+b2, map[string]int{"quantity": 1})
	_, body = do(t, pat.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "pay_shop"})
	o2 := body["order_id"].(string)
	cs2 := sessionOf(t, e, o2)
	if p := e.pay.Params(cs2); len(p.Lines) != 1 || p.Lines[0].UnitCents != 20000 || p.FeeCents != 600 {
		t.Fatalf("single line params: %+v", p)
	}
	sess := e.pay.Complete(cs2)
	obj, _ := json.Marshal(sess)
	payload, _ := json.Marshal(map[string]any{"id": "evt_cs2", "type": "checkout.session.completed", "account": acct,
		"data": map[string]any{"object": json.RawMessage(obj)}})
	req, _ := http.NewRequest("POST", e.ts.URL+"/v1/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", stripe.SignForTest(payload, testWebhookSecret, time.Now()))
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("webhook: %v %v", err, res)
	}
	res.Body.Close()
	var st string
	e.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, o2).Scan(&st)
	if st != "paid" {
		t.Fatalf("webhook should mark paid: %s", st)
	}
}

func TestCheckoutExpiryAndHolds(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	sc, hacct := newSeller(t, e, "hold_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3`)
	seller := &httpClientT{sc}
	a := createListing(t, e, seller, rawCard("H1", 900))["id"].(string)
	quinn := buyer(t, e, "quinn")

	// Walking away from Stripe's page gives the card back.
	do(t, quinn.c, "PUT", e.ts.URL+"/v1/cart/"+a, map[string]int{"quantity": 1})
	_, body := do(t, quinn.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "hold_shop"})
	o := body["order_id"].(string)
	if q, _ := listingQty(t, e, a); q != 0 {
		t.Fatalf("reserved: %d", q)
	}
	e.pay.Expire(sessionOf(t, e, o))
	e.db.Exec(`UPDATE orders SET created_at = now() - interval '1 hour' WHERE id = $1`, o)
	if err := e.co.RunJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if q, st := listingQty(t, e, a); q != 1 || st != "active" {
		t.Fatalf("restored: %d %s", q, st)
	}
	var st string
	e.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, o).Scan(&st)
	if st != "cancelled" {
		t.Fatalf("order: %s", st)
	}

	// Pressing Pay again closes the first checkout; backing out releases the card.
	_, body = do(t, quinn.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "hold_shop"})
	first := body["order_id"].(string)
	code, body := do(t, quinn.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "hold_shop"})
	if code != 201 {
		t.Fatalf("pay again: %d %v", code, body)
	}
	second := body["order_id"].(string)
	if s, err := e.pay.GetCheckoutSession(ctx, hacct, sessionOf(t, e, first)); err != nil || s.Status != "expired" {
		t.Fatal("first session should be closed")
	}
	e.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, first).Scan(&st)
	if st != "cancelled" {
		t.Fatalf("first order: %s", st)
	}
	if code, body := do(t, quinn.c, "POST", e.ts.URL+"/v1/orders/"+second+"/cancel", map[string]any{}); code != 200 || body["status"] != "cancelled" {
		t.Fatalf("cancel: %d %v", code, body)
	}
	if q, _ := listingQty(t, e, a); q != 1 {
		t.Fatalf("released after cancel: %d", q)
	}

	// Stripe failing to start cancels the order too.
	e.pay.Fail = context.DeadlineExceeded
	if code, _ := do(t, quinn.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "hold_shop"}); code != 500 {
		t.Fatalf("stripe down: %d", code)
	}
	e.pay.Fail = nil
	if q, _ := listingQty(t, e, a); q != 1 {
		t.Fatalf("not restored after stripe failure: %d", q)
	}

	// An auction win is paid through its hold.
	auc := createListing(t, e, seller, auctionCard("H2", 500, map[string]any{"buy_now_cents": 2500}))["id"].(string)
	if code, body := do(t, quinn.c, "POST", e.ts.URL+"/v1/listings/"+auc+"/buy-now", map[string]any{}); code != 200 {
		t.Fatalf("buy now: %d %v", code, body)
	}
	var hold string
	e.db.QueryRow(`SELECT id FROM holds WHERE listing_id = $1`, auc).Scan(&hold)
	rob := buyer(t, e, "rob")
	if code, _ := do(t, rob.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"hold_ids": []string{hold}}); code != 409 {
		t.Fatalf("someone else's hold: %d", code)
	}
	code, body = do(t, quinn.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"hold_ids": []string{hold}})
	if code != 201 {
		t.Fatalf("hold checkout: %d %v", code, body)
	}
	ho := body["order_id"].(string)
	cs := sessionOf(t, e, ho)
	if p := e.pay.Params(cs); p.Lines[0].UnitCents != 2500 || p.FeeCents != 150 {
		t.Fatalf("hold params: %+v", p)
	}
	e.pay.Complete(cs)
	do(t, quinn.c, "POST", e.ts.URL+"/v1/orders/"+ho+"/confirm?session_id="+cs, map[string]any{})
	e.db.QueryRow(`SELECT status FROM holds WHERE id = $1`, hold).Scan(&st)
	if st != "paid" {
		t.Fatalf("hold should be paid: %s", st)
	}

	// An unpaid accepted offer: while checkout is off nobody is late; once
	// it's on, the buyer gets a strike and the card goes back on sale.
	card := rawCard("H3", 1000)
	card["quantity"] = 2
	off := createListing(t, e, seller, card)["id"].(string)
	var uid, sid string
	e.db.QueryRow(`SELECT id FROM users WHERE username = 'rob'`).Scan(&uid)
	e.db.QueryRow(`SELECT id FROM users WHERE username = 'hold_shop'`).Scan(&sid)
	e.db.Exec(`UPDATE listings SET quantity = 1 WHERE id = $1`, off)
	e.db.QueryRow(`INSERT INTO holds (listing_id, buyer_id, seller_id, source, quantity, unit_cents, due_at)
		VALUES ($1, $2, $3, 'offer', 1, 800, now() - interval '1 minute') RETURNING id`, off, uid, sid).Scan(&hold)
	e.co.Enabled = false
	e.co.RunJobs(ctx)
	var due time.Time
	e.db.QueryRow(`SELECT status, due_at FROM holds WHERE id = $1`, hold).Scan(&st, &due)
	if st != "awaiting_payment" || due.Before(time.Now().Add(47*time.Hour)) {
		t.Fatalf("hold should be extended while checkout is off: %s %v", st, due)
	}
	e.co.Enabled = true
	e.db.Exec(`UPDATE holds SET due_at = now() - interval '1 minute' WHERE id = $1`, hold)
	if n, err := e.co.ExpireHolds(ctx); err != nil || n != 1 {
		t.Fatalf("expire holds: %d %v", n, err)
	}
	var strikes int
	e.db.QueryRow(`SELECT buyer_strikes FROM users WHERE id = $1`, uid).Scan(&strikes)
	if q, _ := listingQty(t, e, off); q != 2 || strikes != 1 {
		t.Fatalf("after expiry: qty %d strikes %d", q, strikes)
	}
}
