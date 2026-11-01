package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/demo"
	"github.com/jconner5017-rgb/marketplace/internal/usps"
)

type fakeAddr struct{}

func (fakeAddr) Standardize(ctx context.Context, street, secondary, city, state, zip string) (usps.Standard, error) {
	return usps.Standard{ZIP5: zip[:5], ZIP4: "4911", DeliveryPoint: "17"}, nil
}

type fakeTracker struct{ scans []usps.Scan }

func (f *fakeTracker) Piece(ctx context.Context, imb string) (usps.Piece, error) {
	return usps.Piece{Scans: f.scans}, nil
}

func TestTrackedEnvelope(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "env_shop", true)
	a := createListing(t, e, &httpClientT{sc}, rawCard("EN1", 900))["id"].(string)
	b := buyer(t, e, "elle")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+a, map[string]int{"quantity": 1})
	_, body := do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "env_shop"})
	order := body["order_id"].(string)
	e.pay.Complete(sessionOf(t, e, order))
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/confirm", map[string]any{})
	e.db.Exec(`UPDATE orders SET ship_to = '{"name":"Elle","line1":"4317 SE Furlong Dr","city":"Lees Summit","state":"MO","zip":"64082"}' WHERE id = $1`, order)

	// Off until a mailer ID is configured.
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/envelope", map[string]any{}); code != 503 {
		t.Fatalf("envelope while off: %d", code)
	}
	tr := &fakeTracker{}
	e.co.Letters.MailerID, e.co.Letters.ServiceType = "904287724", 40
	e.co.Letters.Addresses, e.co.Letters.Tracker = fakeAddr{}, tr

	// The buyer can't print it.
	if code, _ := do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/envelope", map[string]any{}); code != 403 && code != 404 {
		t.Fatalf("buyer printed: %d", code)
	}
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/envelope", map[string]any{})
	if code != 200 {
		t.Fatalf("envelope: %d %v", code, body)
	}
	env := body["envelope"].(map[string]any)
	bc := env["barcode"].(string)
	if len(bc) != 31 || !strings.HasPrefix(bc, "00040904287724") || !strings.HasSuffix(bc, "64082491117") || len(env["bars"].(string)) != 65 || env["full_zip"] != true {
		t.Fatalf("barcode %v", env)
	}
	// Printing again gives the same barcode.
	_, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/envelope", map[string]any{})
	if body["envelope"].(map[string]any)["barcode"] != bc {
		t.Fatal("barcode changed on reprint")
	}

	// Marking shipped with no tracking uses the barcode.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/ship", map[string]any{}); code != 200 {
		t.Fatalf("ship: %d %v", code, body)
	}
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/orders/"+order, nil)
	sh := body["order"].(map[string]any)["shipment"].(map[string]any)
	if sh["tracking_number"] != bc || sh["service"] != "First-Class letter (Intelligent Mail)" {
		t.Fatalf("shipment %v", sh)
	}

	// Scans arrive; the buyer sees them and gets one notice.
	tr.scans = []usps.Scan{
		{DateTime: "2026-11-02 21:10:00", MailPhase: "Phase 1 - Origin Processing", City: "KANSAS CITY", State: "MO"},
		{DateTime: "2026-11-03 04:31:12", MailPhase: "Phase 4c - Destination Sequenced Carrier Sortation", City: "SAINT LOUIS", State: "MO"},
	}
	n, err := e.co.RefreshLetterScans(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("refresh: %d %v", n, err)
	}
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/orders/"+order, nil)
	sh = body["order"].(map[string]any)["shipment"].(map[string]any)
	evs := sh["events"].([]any)
	if len(evs) != 2 || sh["last_event"] != "Sorted for delivery in SAINT LOUIS MO" ||
		evs[0].(map[string]any)["description"] != "Processed at origin facility in KANSAS CITY MO" {
		t.Fatalf("events %v / %v", sh["last_event"], evs)
	}
	// Checked recently: nothing new to do.
	if n, _ := e.co.RefreshLetterScans(context.Background()); n != 0 {
		t.Fatalf("refreshed again: %d", n)
	}
	var notices int
	e.db.QueryRow(`SELECT count(*) FROM notifications WHERE title LIKE '%moving through USPS%'`).Scan(&notices)
	if notices != 1 {
		t.Fatalf("notices %d", notices)
	}
}

// With real checkout on, the sample shop can't be paid for; admins get a
// test order instead.
func TestSampleShopCheckout(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "sample_like", true)
	a := createListing(t, e, &httpClientT{sc}, rawCard("SS1", 900))["id"].(string)
	e.db.Exec(`UPDATE seller_profiles sp SET stripe_account_id = 'acct_demo_sample_like' FROM users u WHERE u.id = sp.user_id AND u.username = 'sample_like'`)
	b := buyer(t, e, "regular_rita")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+a, map[string]int{"quantity": 1})
	if code, body := do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "sample_like"}); code != 409 ||
		!strings.Contains(body["error"].(map[string]any)["message"].(string), "sample listing") {
		t.Fatalf("buyer: %d %v", code, body)
	}
	boss := buyer(t, e, "sample_boss")
	e.db.Exec(`UPDATE users SET role = 'admin' WHERE username = 'sample_boss'`)
	do(t, boss.c, "PUT", e.ts.URL+"/v1/cart/"+a, map[string]int{"quantity": 1})
	code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "sample_like"})
	if code != 201 {
		t.Fatalf("admin: %d %v", code, body)
	}
	var test bool
	e.db.QueryRow(`SELECT test FROM orders WHERE id = $1`, body["order_id"]).Scan(&test)
	if !test {
		t.Fatal("admin sample order not flagged as test")
	}

	// Removing the sample shop (SEED_DEMO=remove) takes its test orders too.
	e.db.Exec(`UPDATE users SET email = $1 WHERE username = 'sample_like'`, demo.SellerEmail)
	if err := demo.Remove(context.Background(), e.db); err != nil {
		t.Fatalf("remove with test orders: %v", err)
	}
	var left int
	e.db.QueryRow(`SELECT count(*) FROM orders WHERE id = $1`, body["order_id"]).Scan(&left)
	e.db.QueryRow(`SELECT $1::int + count(*) FROM users WHERE username = 'sample_like'`, left).Scan(&left)
	if left != 0 {
		t.Fatalf("sample shop left behind %d rows", left)
	}
}
