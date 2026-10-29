package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestShippingLabels(t *testing.T) {
	e := setupEnv(t)
	sc, acct := newSeller(t, e, "label_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3`)
	a := createListing(t, e, &httpClientT{sc}, rawCard("LB1", 3000))["id"].(string)
	c := rawCard("LB2", 2000)
	c["package_preset"] = "bubble"
	b2 := createListing(t, e, &httpClientT{sc}, c)["id"].(string)
	b := buyer(t, e, "lou")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+a, map[string]int{"quantity": 1})
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+b2, map[string]int{"quantity": 1})
	_, body := do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "label_shop"})
	order := body["order_id"].(string)
	cs := sessionOf(t, e, order)
	e.pay.Complete(cs)
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/confirm", map[string]any{})
	_ = acct

	// Needs a ship-from address and a card first.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", map[string]any{}); code != 409 {
		t.Fatalf("rates without address: %d %v", code, body)
	}
	if code, _ := do(t, sc, "PUT", e.ts.URL+"/v1/seller/shipping/from", map[string]string{"name": "Sam", "line1": "1 Nowhere Rd", "city": "X", "state": "CA", "zip": "94105"}); code != 422 {
		t.Fatalf("bad address accepted: %d", code)
	}
	if code, body := do(t, sc, "PUT", e.ts.URL+"/v1/seller/shipping/from", map[string]string{"name": "Sam", "line1": "1 Market St", "city": "San Francisco", "state": "ca", "zip": "94105"}); code != 200 ||
		body["ship_from"].(map[string]any)["state"] != "CA" {
		t.Fatalf("ship from: %d %v", code, body)
	}
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", map[string]any{})
	if code != 200 {
		t.Fatalf("rates: %d %v", code, body)
	}
	rate := body["rates"].([]any)[0].(map[string]any)
	if rate["charge_cents"].(float64) <= rate["amount_cents"].(float64) {
		t.Fatalf("processing not added: %v", rate)
	}
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label", map[string]string{"rate_id": rate["id"].(string)}); code != 409 {
		t.Fatalf("buy without card: %d %v", code, body)
	}
	_, body = do(t, sc, "POST", e.ts.URL+"/v1/seller/shipping/card", map[string]any{})
	sess := body["url"].(string)[len("https://checkout.stripe.test/"):]
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/shipping/card/finish", map[string]string{"session_id": sess}); code != 200 || body["card"] != "Visa ending 4242" {
		t.Fatalf("card: %d %v", code, body)
	}

	// Declined card: no label.
	e.plat.Decline = true
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label", map[string]string{"rate_id": rate["id"].(string)}); code != 422 {
		t.Fatalf("declined: %d", code)
	}
	e.plat.Decline = false
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label", map[string]string{"rate_id": "rate_made_up"}); code != 409 {
		t.Fatalf("unknown rate: %d", code)
	}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label", map[string]string{"rate_id": rate["id"].(string)})
	if code != 201 {
		t.Fatalf("buy: %d %v", code, body)
	}
	tracking := body["label"].(map[string]any)["tracking_number"].(string)
	var charged int
	for _, v := range e.plat.Charges {
		charged = v
	}
	if charged != int(rate["charge_cents"].(float64)) {
		t.Fatalf("charged %d", charged)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders/"+order, nil)
	o := body["order"].(map[string]any)
	sh := o["shipment"].(map[string]any)
	if o["status"] != "shipped" || sh["label_url"] == nil || sh["label_bought"] != true {
		t.Fatalf("after label: %v", o)
	}
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/orders/"+order, nil)
	if _, leaked := body["order"].(map[string]any)["shipment"].(map[string]any)["label_url"]; leaked {
		t.Fatal("buyer sees the label file")
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", map[string]any{}); code != 409 {
		t.Fatalf("second label: %d", code)
	}

	// Void before any scan: card refunded, order back to paid.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/void", map[string]any{}); code != 200 || body["order"].(map[string]any)["status"] != "paid" {
		t.Fatalf("void: %d %v", code, body)
	}
	if len(e.plat.Refunded) != 1 || len(e.ship.Voided) != 1 {
		t.Fatalf("void side effects: %v %v", e.plat.Refunded, e.ship.Voided)
	}

	// Buy again, then tracking scans arrive.
	_, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", map[string]any{})
	rid := body["rates"].([]any)[0].(map[string]any)["id"].(string)
	_, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label", map[string]string{"rate_id": rid})
	tracking = body["label"].(map[string]any)["tracking_number"].(string)
	hook := func(token, status string) int {
		b, _ := json.Marshal(map[string]string{"tracking_number": tracking, "status": status, "description": "Scan " + status, "location": "Austin, TX"})
		res, err := http.Post(e.ts.URL+"/v1/webhooks/shipping/fake?token="+token, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := hook("wrong", "in_transit"); code != 404 {
		t.Fatalf("bad token: %d", code)
	}
	hook("shiptok", "in_transit")
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/void", map[string]any{}); code != 409 {
		t.Fatalf("void after scan: %d", code)
	}
	hook("shiptok", "delivered")
	var st string
	e.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, order).Scan(&st)
	var events int
	e.db.QueryRow(`SELECT jsonb_array_length(events) FROM shipments WHERE tracking_number = $1`, tracking).Scan(&events)
	if st != "delivered" || events != 2 {
		t.Fatalf("tracking: %s %d", st, events)
	}
	var labelLedger int
	e.db.QueryRow(`SELECT count(*) FROM ledger_entries WHERE order_id = $1 AND kind = 'label_cost'`, order).Scan(&labelLedger)
	if labelLedger != 2 {
		t.Fatalf("label ledger rows: %d", labelLedger)
	}
}
