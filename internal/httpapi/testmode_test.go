package httpapi

import (
	"strings"
	"testing"
)

// While real checkout is off, an admin can become a seller without Stripe
// and place orders that are marked paid with no payment, flagged as tests.
// Nobody else can.
func TestAdminTestMode(t *testing.T) {
	e := setupEnv(t)
	e.co.Enabled = false
	defer func() { e.co.Enabled = true }()

	boss := buyer(t, e, "tm_boss")
	e.db.Exec(`UPDATE users SET role = 'admin' WHERE username = 'tm_boss'`)
	pat := buyer(t, e, "tm_pat")

	// Seller setup with no Stripe.
	code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/seller/onboarding", map[string]string{})
	if code != 200 || body["url"] != "/sell?test_seller=1" {
		t.Fatalf("admin onboarding: %d %v", code, body)
	}
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/seller", nil)
	if sel := body["seller"].(map[string]any); sel["charges_enabled"] != true {
		t.Fatalf("test seller: %v", body)
	}
	if code, body := do(t, pat.c, "POST", e.ts.URL+"/v1/seller/onboarding", map[string]string{}); code != 200 || !strings.HasPrefix(body["url"].(string), "https://connect.stripe.com/") {
		t.Fatalf("non-admin onboarding should go to Stripe: %d %v", code, body)
	}

	// The admin buys a card from a normal seller.
	sc, _ := newSeller(t, e, "tm_shop", true)
	id := createListing(t, e, &httpClientT{sc}, rawCard("TM1", 1500))["id"].(string)
	do(t, boss.c, "PUT", e.ts.URL+"/v1/cart/"+id, map[string]int{"quantity": 1})
	do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/"+id, map[string]int{"quantity": 1})

	if _, b := do(t, pat.c, "GET", e.ts.URL+"/v1/checkout/status", nil); b["enabled"] != false || b["test"] != false {
		t.Fatalf("member status: %v", b)
	}
	if _, b := do(t, boss.c, "GET", e.ts.URL+"/v1/checkout/status", nil); b["test"] != true {
		t.Fatalf("admin status: %v", b)
	}
	if code, _ := do(t, pat.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "tm_shop"}); code != 503 {
		t.Fatalf("member checkout while off: %d", code)
	}
	code, body = do(t, boss.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "tm_shop"})
	if code != 201 || !strings.Contains(body["url"].(string), "test=1") {
		t.Fatalf("admin test checkout: %d %v", code, body)
	}
	oid := body["order_id"].(string)
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/orders/"+oid, nil)
	o := body["order"].(map[string]any)
	if o["status"] != "paid" || o["test"] != true {
		t.Fatalf("test order: %v", o)
	}
	// The seller sees it as a test too, with no Stripe refund link.
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders/"+oid, nil)
	if so := body["order"].(map[string]any); so["test"] != true || so["stripe_payment_url"] != nil {
		t.Fatalf("seller view: %v", so)
	}
	var note string
	e.db.QueryRow(`SELECT note FROM ledger_entries WHERE order_id = $1 AND kind = 'charge'`, oid).Scan(&note)
	if !strings.HasPrefix(note, "TEST ORDER") {
		t.Fatalf("ledger note %q", note)
	}

	// Once real checkout is on, test mode ends.
	e.co.Enabled = true
	if _, b := do(t, boss.c, "GET", e.ts.URL+"/v1/checkout/status", nil); b["enabled"] != true || b["test"] != false {
		t.Fatalf("status when live: %v", b)
	}
}
