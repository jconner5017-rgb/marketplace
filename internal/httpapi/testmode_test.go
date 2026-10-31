package httpapi

import (
	"context"
	"github.com/jconner5017-rgb/marketplace/internal/demo"
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
	// Opening the order cleared its "confirmed" notification.
	var unread int
	e.db.QueryRow(`SELECT count(*) FROM notifications WHERE link = $1 AND read_at IS NULL`, "/purchases/"+oid).Scan(&unread)
	if unread != 0 {
		t.Fatalf("order notice still unread after viewing")
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

	// The admin can't ship a normal seller's order, but can play the sample
	// shop on their own test order.
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/orders/"+oid+"/ship", map[string]string{"carrier": "", "tracking_number": ""}); code != 404 {
		t.Fatalf("admin shipping someone else's order: %d", code)
	}
	e.db.Exec(`DELETE FROM users WHERE lower(email) = lower($1)`, demo.SellerEmail)
	e.db.Exec(`UPDATE users SET email = $1 WHERE username = 'tm_shop'`, demo.SellerEmail)
	defer e.db.Exec(`UPDATE users SET email = 'tm_shop@example.com' WHERE username = 'tm_shop'`)
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/orders/"+oid+"/ship", map[string]string{"carrier": "", "tracking_number": ""}); code != 200 {
		t.Fatalf("admin ships as sample shop: %d %v", code, body)
	}
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/orders/"+oid+"/delivered", map[string]string{}); code != 200 {
		t.Fatalf("delivered: %d %v", code, body)
	}

	// Once real checkout is on, test mode ends.
	e.co.Enabled = true
	if _, b := do(t, boss.c, "GET", e.ts.URL+"/v1/checkout/status", nil); b["enabled"] != true || b["test"] != false {
		t.Fatalf("status when live: %v", b)
	}
	// The admin's test seller goes back to "set up payments", and real
	// onboarding replaces the test account.
	if n, err := e.srv.sellers.RetireTestSellers(context.Background()); err != nil || n < 1 {
		t.Fatalf("retire: %d %v", n, err)
	}
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/seller", nil)
	if sel := body["seller"].(map[string]any); sel["charges_enabled"] != false {
		t.Fatalf("retired seller: %v", sel)
	}
	code, body = do(t, boss.c, "POST", e.ts.URL+"/v1/seller/onboarding", map[string]string{})
	if code != 200 || !strings.HasPrefix(body["url"].(string), "https://connect.stripe.com/") {
		t.Fatalf("real onboarding: %d %v", code, body)
	}
	var acct string
	e.db.QueryRow(`SELECT stripe_account_id FROM seller_profiles sp JOIN users u ON u.id = sp.user_id WHERE u.username = 'tm_boss'`).Scan(&acct)
	if strings.HasPrefix(acct, "acct_test_") {
		t.Fatalf("test account not replaced: %s", acct)
	}
}
