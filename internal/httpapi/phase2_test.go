package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

// newSeller signs up a user, runs onboarding and (optionally) marks Stripe
// onboarding finished. Returns the logged-in client and Stripe account ID.
func newSeller(t *testing.T, e *env, name string, finish bool) (*http.Client, string) {
	t.Helper()
	c := client(t)
	code, body := do(t, c, "POST", e.ts.URL+"/v1/auth/signup", map[string]string{
		"email": name + "@example.com", "username": name, "password": "longenough1",
	})
	if code != 201 {
		t.Fatalf("signup %s: %d %v", name, code, body)
	}
	code, body = do(t, c, "POST", e.ts.URL+"/v1/seller/onboarding", map[string]string{})
	if code != 200 {
		t.Fatalf("onboarding %s: %d %v", name, code, body)
	}
	url, _ := body["url"].(string)
	if !strings.HasPrefix(url, "https://connect.stripe.com/setup/acct_") ||
		!strings.Contains(url, "return=https://market.test/seller/onboarding/return") {
		t.Fatalf("onboarding url: %s", url)
	}
	acct := strings.TrimPrefix(strings.SplitN(url, "?", 2)[0], "https://connect.stripe.com/setup/")
	if finish {
		e.stripe.finish(acct)
		code, body = do(t, c, "GET", e.ts.URL+"/v1/seller", nil)
		seller, _ := body["seller"].(map[string]any)
		if code != 200 || seller["charges_enabled"] != true {
			t.Fatalf("seller after finish: %d %v", code, body)
		}
	}
	return c, acct
}

func rawCard(ext string, cents int) map[string]any {
	return map[string]any{
		"external_id": ext, "category": "tcg", "game_or_sport": "Pokemon", "set_name": "Base Set",
		"year": 1999, "card_number": "4/102", "title": "Charizard Holo " + ext, "condition": "lp",
		"attributes": []string{"holo"}, "photo_urls": []string{"https://img.example.com/" + ext + ".jpg"},
		"price_cents": cents, "quantity": 1,
	}
}

func TestOnboardingFlow(t *testing.T) {
	e := setupEnv(t)
	c, acct := newSeller(t, e, "seller_one", false)

	// Clicking twice reuses the same Stripe account.
	do(t, c, "POST", e.ts.URL+"/v1/seller/onboarding", map[string]string{})
	if e.stripe.created != 1 {
		t.Fatalf("want 1 Stripe account, got %d", e.stripe.created)
	}

	// Not finished: cannot list.
	code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", rawCard("A1", 500))
	if code != 403 || errCode(body) != "onboarding_incomplete" {
		t.Fatalf("list before onboarding: %d %v", code, body)
	}

	// Stripe return page refreshes status.
	e.stripe.finish(acct)
	res, err := c.Get(e.ts.URL + "/seller/onboarding/return")
	if err != nil {
		t.Fatal(err)
	}
	var page bytes.Buffer
	page.ReadFrom(res.Body)
	res.Body.Close()
	if !strings.Contains(page.String(), "ready to sell") {
		t.Fatalf("return page: %s", page.String())
	}
	code, body = do(t, c, "GET", e.ts.URL+"/v1/seller", nil)
	seller, _ := body["seller"].(map[string]any)
	if seller["tier"].(float64) != 1 || seller["tier_cap_cents"].(float64) != 2000 {
		t.Fatalf("seller: %v", body)
	}

	// Non-sellers get a clear error.
	other := client(t)
	do(t, other, "POST", e.ts.URL+"/v1/auth/signup", map[string]string{
		"email": "buyer@example.com", "username": "just_buyer", "password": "longenough1"})
	if code, body := do(t, other, "GET", e.ts.URL+"/v1/seller", nil); code != 404 || errCode(body) != "not_a_seller" {
		t.Fatalf("non-seller: %d %v", code, body)
	}
}

func TestListingLifecycleAndTierCap(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "lister", true)

	// Tier 1 cap is $20.
	code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", rawCard("A1", 2001))
	if code != 422 || !strings.Contains(fmt.Sprint(body), "tier allows up to $20.00") {
		t.Fatalf("over tier cap: %d %v", code, body)
	}
	code, body = do(t, c, "POST", e.ts.URL+"/v1/seller/listings", rawCard("A1", 1850))
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	l := body["listing"].(map[string]any)
	id := l["id"].(string)
	if l["condition"] != "LP" || l["status"] != "active" || l["version"].(float64) != 1 {
		t.Fatalf("created listing: %v", l)
	}

	// Validation: raw card without condition, bad photo, graded without grade.
	bad := rawCard("A2", 500)
	delete(bad, "condition")
	bad["photo_urls"] = []string{"http://insecure.example.com/x.jpg"}
	code, body = do(t, c, "POST", e.ts.URL+"/v1/seller/listings", bad)
	fields, _ := body["error"].(map[string]any)["fields"].(map[string]any)
	if code != 422 || fields["condition"] == nil || fields["photo_urls"] == nil {
		t.Fatalf("validation: %d %v", code, body)
	}

	// Update price and quantity; version bumps; qty 0 = sold out.
	code, body = do(t, c, "PATCH", e.ts.URL+"/v1/seller/listings/"+id, map[string]any{"price_cents": 1500, "quantity": 0})
	l = body["listing"].(map[string]any)
	if code != 200 || l["price_cents"].(float64) != 1500 || l["status"] != "sold_out" || l["version"].(float64) != 2 {
		t.Fatalf("patch: %d %v", code, body)
	}
	var versions int
	e.db.QueryRow(`SELECT count(*) FROM listing_versions WHERE listing_id = $1`, id).Scan(&versions)
	if versions != 2 {
		t.Fatalf("want 2 snapshots, got %d", versions)
	}

	// Another seller can't touch it.
	c2, _ := newSeller(t, e, "intruder", true)
	if code, _ := do(t, c2, "PATCH", e.ts.URL+"/v1/seller/listings/"+id, map[string]any{"price_cents": 1}); code != 404 {
		t.Fatalf("other seller patch: want 404, got %d", code)
	}
	if code, _ := do(t, c2, "DELETE", e.ts.URL+"/v1/seller/listings/"+id, nil); code != 404 {
		t.Fatalf("other seller delete: want 404, got %d", code)
	}

	// Remove.
	if code, _ := do(t, c, "DELETE", e.ts.URL+"/v1/seller/listings/"+id, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := do(t, c, "GET", e.ts.URL+"/v1/seller/listings/"+id, nil); code != 404 {
		t.Fatalf("get removed: want 404, got %d", code)
	}
}

func TestGradedCardAndPromotion(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "slab_seller", true)
	if _, err := e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true`); err != nil {
		t.Fatal(err)
	}
	card := map[string]any{
		"category": "sports", "game_or_sport": "Baseball", "set_name": "Topps Chrome", "year": 2018,
		"title": "Shohei Ohtani Rookie", "grader": "psa", "grade": 10, "cert_number": "12345678",
		"attributes": []string{"rookie"}, "photo_urls": []string{"https://img.example.com/o.jpg"},
		"price_cents": 95000, "quantity": 1, "package_preset": "bubble", "weight_oz": 3,
	}
	code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", card)
	if code != 201 {
		t.Fatalf("graded create: %d %v", code, body)
	}
	card["grade"] = 9.3
	if code, _ := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", card); code != 422 {
		t.Fatalf("grade 9.3: want 422, got %d", code)
	}
	card["grade"] = 10
	card["price_cents"] = 100001
	if code, _ := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", card); code != 422 {
		t.Fatalf("over $1,000: want 422, got %d", code)
	}

	// Demotion hides pricier listings from buyers without deleting them.
	if _, err := e.db.Exec(`UPDATE seller_profiles SET tier = 1`); err != nil {
		t.Fatal(err)
	}
	_, body = do(t, client(t), "GET", e.ts.URL+"/v1/listings", nil)
	if body["total"].(float64) != 0 {
		t.Fatalf("demoted seller's $950 card still public: %v", body)
	}
	_, body = do(t, c, "GET", e.ts.URL+"/v1/seller/listings", nil)
	if body["total"].(float64) != 1 {
		t.Fatalf("seller should still see own listing: %v", body)
	}
}

func TestUpsertByExternalID(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "fcm_user", true)
	code, body := do(t, c, "PUT", e.ts.URL+"/v1/seller/listings/external/FCM-77", rawCard("FCM-77", 1000))
	if code != 201 || body["created"] != true {
		t.Fatalf("first upsert: %d %v", code, body)
	}
	id := body["listing"].(map[string]any)["id"]
	card := rawCard("FCM-77", 900)
	code, body = do(t, c, "PUT", e.ts.URL+"/v1/seller/listings/external/FCM-77", card)
	l := body["listing"].(map[string]any)
	if code != 200 || body["created"] != false || l["id"] != id || l["price_cents"].(float64) != 900 {
		t.Fatalf("second upsert: %d %v", code, body)
	}
	if code, _ := do(t, c, "PUT", e.ts.URL+"/v1/seller/listings/external/OTHER", rawCard("FCM-77", 900)); code != 422 {
		t.Fatalf("mismatched external id: want 422, got %d", code)
	}
	// Create with an external_id that already exists.
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", rawCard("FCM-77", 900)); code != 409 {
		t.Fatalf("duplicate external create: want 409, got %d %v", code, body)
	}
}

func postCSV(t *testing.T, c *http.Client, url, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/csv")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestCSVImport(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "bulk_seller", true)

	// The downloadable template must import cleanly.
	res, err := http.Get(e.ts.URL + "/v1/seller/listings/import/template")
	if err != nil {
		t.Fatal(err)
	}
	var tmpl bytes.Buffer
	tmpl.ReadFrom(res.Body)
	res.Body.Close()
	code, body := postCSV(t, c, e.ts.URL+"/v1/seller/listings/import", tmpl.String())
	if code != 200 || body["created"].(float64) != 2 || body["failed"].(float64) != 0 {
		t.Fatalf("template import: %d %v", code, body)
	}

	// Re-upload updates instead of duplicating; bad rows are reported, good rows saved.
	csv := "\uFEFFexternal_id,category,game_or_sport,set_name,title,condition,photo_urls,price,quantity\n" +
		"SKU-0001,tcg,Pokemon,Base Set,Charizard Holo,LP,https://img.example.com/c.jpg,$17.25,2\n" +
		"SKU-0003,tcg,Magic,Alpha,Lightning Bolt,NM,https://img.example.com/b.jpg,12.5,1\n" +
		"SKU-0004,tcg,Magic,Alpha,Black Lotus,NM,https://img.example.com/l.jpg,25000,1\n" +
		"SKU-0005,tcg,Magic,Alpha,Mox,,https://img.example.com/m.jpg,abc,1\n" +
		"SKU-0003,tcg,Magic,Alpha,Dup row,NM,https://img.example.com/d.jpg,1,1\n"
	code, body = postCSV(t, c, e.ts.URL+"/v1/seller/listings/import", csv)
	if code != 200 {
		t.Fatalf("import: %d %v", code, body)
	}
	if body["updated"].(float64) != 1 || body["created"].(float64) != 1 || body["failed"].(float64) != 3 {
		t.Fatalf("import counts: %v", body)
	}
	rows := body["rows"].([]any)
	r4 := rows[2].(map[string]any)["errors"].(map[string]any)
	r5 := rows[3].(map[string]any)["errors"].(map[string]any)
	r6 := rows[4].(map[string]any)["errors"].(map[string]any)
	if r4["price_cents"] == nil || r5["price"] == nil || r5["condition"] == nil || r6["external_id"] == nil {
		t.Fatalf("row errors: %v", rows)
	}
	var price, qty int
	e.db.QueryRow(`SELECT price_cents, quantity FROM listings WHERE external_id = 'SKU-0001'`).Scan(&price, &qty)
	if price != 1725 || qty != 2 {
		t.Fatalf("SKU-0001 not updated: %d %d", price, qty)
	}

	// File-level problems.
	if code, body := postCSV(t, c, e.ts.URL+"/v1/seller/listings/import", "external_id,colour\nA,red\n"); code != 422 || errCode(body) != "invalid_csv" {
		t.Fatalf("unknown column: %d %v", code, body)
	}
	if code, _ := postCSV(t, c, e.ts.URL+"/v1/seller/listings/import", ""); code != 422 {
		t.Fatalf("empty file: want 422, got %d", code)
	}
}

func TestSearch(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "shop_a", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true`)
	add := func(m map[string]any) {
		if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", m); code != 201 {
			t.Fatalf("add: %d %v", code, body)
		}
	}
	add(rawCard("C1", 1200))
	add(rawCard("C2", 300))
	psa := rawCard("C3", 8000)
	delete(psa, "condition")
	psa["grader"], psa["grade"], psa["title"] = "PSA", 9, "Charizard PSA 9"
	add(psa)
	bolt := rawCard("M1", 500)
	bolt["game_or_sport"], bolt["set_name"], bolt["title"], bolt["attributes"] = "Magic", "Alpha", "Lightning Bolt", []string{}
	add(bolt)
	ball := rawCard("S1", 2500)
	ball["category"], ball["game_or_sport"], ball["set_name"], ball["title"] = "sports", "Baseball", "Topps", "Mickey Mantle"
	add(ball)

	// An unfinished seller's listings are never public.
	c2, acct2 := newSeller(t, e, "shop_b", true)
	do(t, c2, "POST", e.ts.URL+"/v1/seller/listings", rawCard("Z1", 100))
	e.db.Exec(`UPDATE seller_profiles SET charges_enabled = false WHERE stripe_account_id = $1`, acct2)

	anon := client(t)
	search := func(q string) map[string]any {
		code, body := do(t, anon, "GET", e.ts.URL+"/v1/listings?"+q, nil)
		if code != 200 {
			t.Fatalf("search %s: %d %v", q, code, body)
		}
		return body
	}
	total := func(q string) int { return int(search(q)["total"].(float64)) }

	if n := total(""); n != 5 {
		t.Fatalf("all: want 5, got %d", n)
	}
	if n := total("q=charizard"); n != 3 {
		t.Fatalf("q=charizard: want 3, got %d", n)
	}
	if n := total("q=bolt&category=tcg"); n != 1 {
		t.Fatalf("bolt: want 1, got %d", n)
	}
	if n := total("category=sports"); n != 1 {
		t.Fatalf("sports: want 1, got %d", n)
	}
	if n := total("game=pokemon&grader=raw"); n != 2 {
		t.Fatalf("raw pokemon: want 2, got %d", n)
	}
	if n := total("grader=PSA&min_grade=9"); n != 1 {
		t.Fatalf("psa 9+: want 1, got %d", n)
	}
	if n := total("min_price=5&max_price=25"); n != 3 {
		t.Fatalf("$5-$25: want 3, got %d", n)
	}
	if n := total("attribute=holo"); n != 4 {
		t.Fatalf("holo: want 4, got %d", n)
	}
	if n := total("seller=SHOP_A"); n != 5 {
		t.Fatalf("seller filter: want 5, got %d", n)
	}
	ls := search("sort=price_asc&limit=2")["listings"].([]any)
	if len(ls) != 2 || ls[0].(map[string]any)["price_cents"].(float64) != 300 {
		t.Fatalf("price_asc: %v", ls)
	}
	if _, has := ls[0].(map[string]any)["external_id"]; has {
		t.Fatal("external_id leaked to public search")
	}
	if n := len(search("sort=price_desc&limit=2&offset=4")["listings"].([]any)); n != 1 {
		t.Fatalf("pagination: want 1, got %d", n)
	}
	if code, _ := do(t, anon, "GET", e.ts.URL+"/v1/listings?sort=bogus", nil); code != 400 {
		t.Fatalf("bad sort: want 400, got %d", code)
	}
	// Odd search syntax must not error.
	search("q=" + "%22unterminated%20OR%20-%20%26%7C!")

	// Public single listing.
	id := search("q=mantle")["listings"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := do(t, anon, "GET", e.ts.URL+"/v1/listings/"+id, nil); code != 200 {
		t.Fatalf("get listing: %d", code)
	}
	if code, _ := do(t, anon, "GET", e.ts.URL+"/v1/listings/not-a-uuid", nil); code != 404 {
		t.Fatalf("bad id: want 404, got %d", code)
	}
}

func TestStripeWebhook(t *testing.T) {
	e := setupEnv(t)
	_, acct := newSeller(t, e, "hooked", false)

	send := func(payload []byte, sig string) int {
		req, _ := http.NewRequest("POST", e.ts.URL+"/v1/webhooks/stripe", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Stripe-Signature", sig)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	payload := []byte(fmt.Sprintf(`{"id":"evt_1","type":"account.updated","account":%q,"data":{"object":{"id":%q,"charges_enabled":true,"details_submitted":true}}}`, acct, acct))

	if code := send(payload, "t=1,v1=deadbeef"); code != 400 {
		t.Fatalf("bad signature: want 400, got %d", code)
	}
	if code := send(payload, stripe.SignForTest(payload, testWebhookSecret, time.Now())); code != 200 {
		t.Fatalf("good webhook: %d", code)
	}
	var enabled bool
	e.db.QueryRowContext(context.Background(),
		`SELECT charges_enabled FROM seller_profiles WHERE stripe_account_id = $1`, acct).Scan(&enabled)
	if !enabled {
		t.Fatal("webhook did not enable charges")
	}
	// Redelivery is acknowledged without reprocessing.
	if code := send(payload, stripe.SignForTest(payload, testWebhookSecret, time.Now())); code != 200 {
		t.Fatalf("duplicate webhook: %d", code)
	}

	deauth := []byte(fmt.Sprintf(`{"id":"evt_2","type":"account.application.deauthorized","account":%q,"data":{"object":{}}}`, acct))
	if code := send(deauth, stripe.SignForTest(deauth, testWebhookSecret, time.Now())); code != 200 {
		t.Fatalf("deauth webhook: %d", code)
	}
	e.db.QueryRow(`SELECT charges_enabled FROM seller_profiles WHERE stripe_account_id = $1`, acct).Scan(&enabled)
	if enabled {
		t.Fatal("deauthorized seller can still sell")
	}
}

func TestBundleDiscountSettings(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "bundler", true)
	code, body := do(t, c, "PATCH", e.ts.URL+"/v1/seller/settings", map[string]any{
		"bundle_discount_type": "per_extra_item", "bundle_discount_val": 75})
	s := body["seller"].(map[string]any)
	if code != 200 || s["bundle_discount_type"] != "per_extra_item" || s["bundle_discount_val"].(float64) != 75 {
		t.Fatalf("set discount: %d %v", code, body)
	}
	if code, _ := do(t, c, "PATCH", e.ts.URL+"/v1/seller/settings", map[string]any{
		"bundle_discount_type": "per_extra_item", "bundle_discount_val": 9999}); code != 422 {
		t.Fatalf("too big: want 422, got %d", code)
	}
	code, body = do(t, c, "PATCH", e.ts.URL+"/v1/seller/settings", map[string]any{})
	if code != 200 || body["seller"].(map[string]any)["bundle_discount_type"] != nil {
		t.Fatalf("clear discount: %d %v", code, body)
	}
}
