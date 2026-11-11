package httpapi

import (
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

func TestTestSales(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "boss_seller", true)
	if _, err := e.db.Exec(`UPDATE users SET staff_role = $1 WHERE username = 'boss_seller'`, admin.SuperAdmin); err != nil {
		t.Fatal(err)
	}
	enrollMFA(t, e.ts.URL, sc)
	mod := staff(t, e, "mod_ts", admin.Moderator)
	enrollMFA(t, e.ts.URL, mod.c)

	// Only Super Admins.
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/test-sales", map[string]any{}); code != 403 {
		t.Fatalf("moderator created a test sale: %d", code)
	}
	// Needs a ship-from address: the envelope goes to the seller.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/admin/test-sales", map[string]any{}); code != 409 {
		t.Fatalf("without ship-from: %d %v", code, body)
	}
	if code, body := do(t, sc, "PUT", e.ts.URL+"/v1/seller/shipping/from", map[string]string{"name": "Sam", "line1": "1 Market St", "city": "San Francisco", "state": "CA", "zip": "94105"}); code != 200 {
		t.Fatalf("ship from: %d %v", code, body)
	}
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/admin/test-sales", map[string]any{})
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	id := body["test_sale"].(map[string]any)["id"].(string)

	// It's in the seller's orders to ship, as an envelope order.
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders/"+id, nil)
	o := body["order"].(map[string]any)
	if o["side"] != "selling" || o["status"] != "paid" || o["ship_method"] != "envelope" || o["test"] != true || o["buyer"] != "sns_test_buyer" {
		t.Fatalf("order: %v", o)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders?side=selling&status=to_ship", nil)
	if len(body["orders"].([]any)) != 1 {
		t.Fatalf("to ship: %v", body)
	}

	// Print the envelope, then ship it.
	e.co.Letters.MailerID, e.co.Letters.ServiceType, e.co.Letters.Addresses = "904287724", 40, fakeAddr{}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/envelope", map[string]any{})
	if code != 200 || body["envelope"].(map[string]any)["to"].(map[string]any)["line1"] != "1 Market St" {
		t.Fatalf("envelope: %d %v", code, body)
	}
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+id+"/ship", map[string]string{"carrier": "", "tracking_number": ""}); code != 200 {
		t.Fatalf("ship: %d %v", code, body)
	}
	var queued int
	e.db.QueryRow(`SELECT count(*) FROM email_outbox WHERE to_email LIKE '%.invalid'`).Scan(&queued)
	if queued != 0 {
		t.Fatalf("%d emails queued to the test buyer", queued)
	}

	// Delete it, and everything with it.
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/admin/test-sales/"+id, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	var left int
	e.db.QueryRow(`SELECT count(*) FROM orders WHERE id = $1`, id).Scan(&left)
	if left != 0 {
		t.Fatal("test sale still there")
	}

	// A real order can't be removed this way.
	lid := createListing(t, e, &httpClientT{sc}, rawCard("TS1", 900))["id"].(string)
	real, _ := seedOrder(t, e, "mod_ts", "boss_seller", lid, 900, false)
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/admin/test-sales/"+real, nil); code != 404 {
		t.Fatalf("deleted a real order: %d", code)
	}
}
