package httpapi

import (
	"context"
	"testing"
	"time"
)

func auctionCard(ext string, start int, extra map[string]any) map[string]any {
	c := rawCard(ext, start)
	a := map[string]any{"start_cents": start, "duration_days": 7}
	for k, v := range extra {
		a[k] = v
	}
	c["format"] = "auction"
	c["auction"] = a
	return c
}

func createListing(t *testing.T, e *env, c *httpClientT, card map[string]any) map[string]any {
	t.Helper()
	code, body := do(t, c.c, "POST", e.ts.URL+"/v1/seller/listings", card)
	if code != 201 {
		t.Fatalf("create listing: %d %v", code, body)
	}
	return body["listing"].(map[string]any)
}

func bid(t *testing.T, e *env, c *httpClientT, id string, cents int) (int, map[string]any) {
	t.Helper()
	return do(t, c.c, "POST", e.ts.URL+"/v1/listings/"+id+"/bids", map[string]int{"max_cents": cents})
}

func wantBid(t *testing.T, e *env, c *httpClientT, id string, cents int, leading bool, price float64) map[string]any {
	t.Helper()
	code, body := bid(t, e, c, id, cents)
	if code != 200 || body["leading"] != leading || body["current_cents"].(float64) != price {
		t.Fatalf("bid %d: want leading=%v price=%v, got %d %v", cents, leading, price, code, body)
	}
	return body
}

func publicAuction(t *testing.T, e *env, id string) map[string]any {
	t.Helper()
	code, body := do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+id, nil)
	if code != 200 {
		t.Fatalf("get listing: %d %v", code, body)
	}
	return body["listing"].(map[string]any)
}

func TestAuctionProxyBidding(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "auction_house", true) // new seller: $20 limit
	seller := &httpClientT{sc}

	// Validation: lengths, reserve and Buy It Now ordering, price limit.
	for _, bad := range []map[string]any{
		{"duration_days": 3},
		{"reserve_cents": 50},
		{"buy_now_cents": 400, "reserve_cents": 500},
		{"buy_now_cents": 2500},
	} {
		if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", auctionCard("BAD", 100, bad)); code != 422 {
			t.Fatalf("bad auction %v: %d %v", bad, code, body)
		}
	}

	l := createListing(t, e, seller, auctionCard("A1", 100, map[string]any{"reserve_cents": 500, "buy_now_cents": 1500}))
	id := l["id"].(string)
	a := l["auction"].(map[string]any)
	if l["format"] != "auction" || a["state"] != "live" || a["reserve_cents"].(float64) != 500 || l["quantity"].(float64) != 1 {
		t.Fatalf("created auction: %v", l)
	}
	pub := publicAuction(t, e, id)["auction"].(map[string]any)
	if _, leaked := pub["reserve_cents"]; leaked || pub["has_reserve"] != true || pub["buy_now_cents"].(float64) != 1500 {
		t.Fatalf("public auction view: %v", pub)
	}

	alice, bob, carl := buyer(t, e, "alice"), buyer(t, e, "bobby"), buyer(t, e, "carl")
	if code, body := bid(t, e, seller, id, 500); code != 422 || errCode(body) != "own_listing" {
		t.Fatalf("seller bid: %d %v", code, body)
	}
	if code, body := bid(t, e, alice, id, 50); code != 422 || errCode(body) != "bid_too_low" {
		t.Fatalf("below start: %d %v", code, body)
	}
	if code, _ := do(t, e.ts.Client(), "GET", e.ts.URL+"/v1/listings/"+id+"/bids", nil); code != 200 {
		t.Fatalf("anon history: %d", code)
	}

	wantBid(t, e, alice, id, 300, true, 100) // first bid opens at the starting bid
	if code, body := do(t, carl.c, "POST", e.ts.URL+"/v1/listings/"+id+"/buy-now", map[string]any{}); code != 409 {
		t.Fatalf("buy now after a bid: %d %v", code, body)
	}
	wantBid(t, e, bob, id, 200, false, 225) // alice's max covers it: 200 + 25
	wantBid(t, e, bob, id, 400, true, 325)  // bob passes alice: 300 + 25
	if code, body := bid(t, e, bob, id, 350); code != 409 {
		t.Fatalf("leader lowering max: %d %v", code, body)
	}
	res := wantBid(t, e, alice, id, 700, true, 500) // 425, then up to the reserve
	if res["reserve_met"] != true {
		t.Fatalf("reserve should be met: %v", res)
	}

	_, hist := do(t, bob.c, "GET", e.ts.URL+"/v1/listings/"+id+"/bids", nil)
	if len(hist["bids"].([]any)) != 4 || hist["bidders"].(float64) != 2 || hist["outbid"] != true || hist["leading"] != false {
		t.Fatalf("history for bob: %v", hist)
	}
	if first := hist["bids"].([]any)[0].(map[string]any); first["bidder"] != "a***e" {
		t.Fatalf("masked name: %v", first)
	}
	_, n := do(t, bob.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if !hasTitle(n, "You've been outbid on Charizard Holo A1") {
		t.Fatalf("bob outbid notice: %v", n)
	}

	// After bids the seller can only add to the description and photos, and can't end it.
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+id, map[string]any{"title": "New"}); code != 409 || errCode(body) != "has_bids" {
		t.Fatalf("edit title with bids: %d %v", code, body)
	}
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+id, map[string]any{"description": "Centered nicely"}); code != 200 {
		t.Fatalf("edit description with bids: %d %v", code, body)
	}
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/seller/listings/"+id, nil); code != 409 {
		t.Fatalf("end with bids: want 409, got %d", code)
	}
	if code, body := do(t, alice.c, "PUT", e.ts.URL+"/v1/cart/"+id, map[string]int{"quantity": 1}); code != 422 || errCode(body) != "auction" {
		t.Fatalf("auction in cart: %d %v", code, body)
	}

	// Soft close: a bid in the last two minutes pushes the end out.
	e.db.Exec(`UPDATE listings SET ends_at = now() + interval '60 seconds' WHERE id = $1`, id)
	wantBid(t, e, bob, id, 800, true, 750) // 700 + 50
	ends, _ := time.Parse(time.RFC3339Nano, publicAuction(t, e, id)["auction"].(map[string]any)["ends_at"].(string))
	if time.Until(ends) < 100*time.Second {
		t.Fatalf("soft close didn't extend: ends in %v", time.Until(ends))
	}

	// The $20 limit: bids are capped, and reaching it ends the auction.
	wantBid(t, e, alice, id, 5000, true, 850)
	res = wantBid(t, e, bob, id, 2000, false, 2000) // tie at the cap: alice was first
	if res["won"] != false {
		t.Fatalf("bob shouldn't win: %v", res)
	}
	done := publicAuction(t, e, id)
	if done["status"] != "ended" || done["auction"].(map[string]any)["state"] != "won" {
		t.Fatalf("auction should be won: %v", done)
	}
	if code, _ := bid(t, e, carl, id, 1000); code != 409 {
		t.Fatalf("bid after end: want 409, got %d", code)
	}
	_, holds := do(t, alice.c, "GET", e.ts.URL+"/v1/holds", nil)
	hs := holds["holds"].([]any)
	if len(hs) != 1 || hs[0].(map[string]any)["unit_cents"].(float64) != 2000 || hs[0].(map[string]any)["source"] != "auction" {
		t.Fatalf("winner hold: %v", holds)
	}
	_, n = do(t, alice.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if !hasTitle(n, "You won Charizard Holo A1") {
		t.Fatalf("winner notice: %v", n)
	}
	_, h := do(t, alice.c, "GET", e.ts.URL+"/v1/listings/"+id+"/bids", nil)
	if h["won"] != true {
		t.Fatalf("history should show alice won: %v", h)
	}
}

func hasTitle(body map[string]any, title string) bool {
	items, _ := body["notices"].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["title"] == title {
			return true
		}
	}
	return false
}

func TestAuctionEndingRelistAndBuyNow(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "relist_shop", true)
	seller := &httpClientT{sc}
	ctx := context.Background()
	dana := buyer(t, e, "dana")

	relist := createListing(t, e, seller, auctionCard("R1", 200, map[string]any{"auto_relist": true}))["id"].(string)
	noReserve := createListing(t, e, seller, auctionCard("R2", 200, map[string]any{"reserve_cents": 1000}))["id"].(string)
	bin := createListing(t, e, seller, auctionCard("R3", 200, map[string]any{"buy_now_cents": 900}))["id"].(string)
	fixed := createListing(t, e, seller, rawCard("F1", 500))["id"].(string)

	// Search: format filter and ending-soonest sort.
	e.db.Exec(`UPDATE listings SET ends_at = now() + interval '1 hour' WHERE id = $1`, bin)
	_, s := do(t, client(t), "GET", e.ts.URL+"/v1/listings?format=auction&sort=ending", nil)
	got := s["listings"].([]any)
	if s["total"].(float64) != 3 || got[0].(map[string]any)["id"] != bin {
		t.Fatalf("auction search: %v", s)
	}
	_, s = do(t, client(t), "GET", e.ts.URL+"/v1/listings?format=fixed", nil)
	if s["total"].(float64) != 1 || s["listings"].([]any)[0].(map[string]any)["id"] != fixed {
		t.Fatalf("fixed search: %v", s)
	}

	wantBid(t, e, dana, noReserve, 500, true, 200) // below the reserve
	e.db.Exec(`UPDATE listings SET ends_at = now() - interval '1 second' WHERE id = ANY($1::uuid[])`,
		"{"+relist+","+noReserve+"}")
	if n, err := e.sales.CloseDue(ctx); err != nil || n != 2 {
		t.Fatalf("close due: %d %v", n, err)
	}
	r := publicAuction(t, e, relist)
	if r["status"] != "active" || r["auction"].(map[string]any)["round"].(float64) != 1 {
		t.Fatalf("should relist once: %v", r)
	}
	if code, body := do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+noReserve, nil); code != 200 ||
		body["listing"].(map[string]any)["auction"].(map[string]any)["state"] != "unsold" {
		t.Fatalf("reserve not met should be unsold: %d %v", code, body)
	}
	e.db.Exec(`UPDATE listings SET ends_at = now() - interval '1 second' WHERE id = $1`, relist)
	e.sales.CloseDue(ctx)
	if st := publicAuction(t, e, relist)["auction"].(map[string]any)["state"]; st != "unsold" {
		t.Fatalf("second time should be unsold, got %v", st)
	}

	// Buy It Now on an auction with no bids.
	if code, body := do(t, dana.c, "POST", e.ts.URL+"/v1/listings/"+bin+"/buy-now", map[string]any{}); code != 200 {
		t.Fatalf("buy now: %d %v", code, body)
	}
	if st := publicAuction(t, e, bin)["auction"].(map[string]any)["state"]; st != "won" {
		t.Fatalf("buy now should win: %v", st)
	}

	// Ending-soon notices go to watchers once.
	watched := createListing(t, e, seller, auctionCard("R4", 200, nil))["id"].(string)
	do(t, dana.c, "PUT", e.ts.URL+"/v1/watchlist/"+watched, map[string]any{})
	e.db.Exec(`UPDATE listings SET ends_at = now() + interval '30 minutes' WHERE id = $1`, watched)
	for i := 0; i < 2; i++ {
		e.sales.NotifyEndingSoon(ctx)
	}
	var notices int
	e.db.QueryRow(`SELECT count(*) FROM notifications WHERE title LIKE '%ends within the hour'`).Scan(&notices)
	if notices != 1 {
		t.Fatalf("ending notices: %d", notices)
	}

	// No-bid auctions can be ended early; settings can change before bids.
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+watched,
		map[string]any{"auction": map[string]any{"start_cents": 300, "duration_days": 10}}); code != 200 ||
		body["listing"].(map[string]any)["price_cents"].(float64) != 300 {
		t.Fatalf("change before bids: %d %v", code, body)
	}
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+watched, map[string]any{"format": "fixed"}); code != 409 {
		t.Fatalf("format change: %d %v", code, body)
	}
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/seller/listings/"+watched, nil); code != 204 {
		t.Fatalf("end without bids: %d", code)
	}
}

func offerOn(t *testing.T, e *env, c *httpClientT, id string, cents int) (int, map[string]any) {
	t.Helper()
	return do(t, c.c, "POST", e.ts.URL+"/v1/listings/"+id+"/offers", map[string]any{"amount_cents": cents, "quantity": 1})
}

func TestBestOffer(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "offer_shop", true)
	seller := &httpClientT{sc}
	ctx := context.Background()

	card := rawCard("O1", 1000)
	card["quantity"] = 2
	card["offers"] = map[string]any{"enabled": true, "accept_kind": "amount", "accept_value": 500,
		"decline_kind": "percent", "decline_value": 60}
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", card); code != 422 {
		t.Fatalf("decline above accept: %d %v", code, body)
	}
	card["offers"] = map[string]any{"enabled": true, "accept_kind": "percent", "accept_value": 90,
		"decline_kind": "amount", "decline_value": 500}
	l := createListing(t, e, seller, card)
	id := l["id"].(string)
	if l["offer_settings"].(map[string]any)["accept_value"].(float64) != 90 {
		t.Fatalf("seller sees settings: %v", l)
	}
	pub := publicAuction(t, e, id)
	if _, leaked := pub["offer_settings"]; leaked || pub["offers_enabled"] != true {
		t.Fatalf("public listing leaks thresholds: %v", pub)
	}
	ac := auctionCard("O2", 100, nil)
	ac["offers"] = map[string]any{"enabled": true}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", ac); code != 201 {
		// offers are dropped for auctions, not an error
		t.Fatalf("auction with offers flag: %d", code)
	}

	eve, finn, gus := buyer(t, e, "eve"), buyer(t, e, "finn"), buyer(t, e, "gus")
	if code, _ := offerOn(t, e, eve, id, 1000); code != 422 {
		t.Fatalf("offer at full price: %d", code)
	}
	if code, body := do(t, eve.c, "POST", e.ts.URL+"/v1/listings/"+id+"/offers",
		map[string]any{"amount_cents": 800, "message": "venmo me instead"}); code != 422 {
		t.Fatalf("off-platform note: %d %v", code, body)
	}
	_, body := offerOn(t, e, eve, id, 400) // below $5: auto-declined
	if o := body["offer"].(map[string]any); o["status"] != "declined" || o["decided_by"] != "auto" {
		t.Fatalf("auto decline: %v", body)
	}
	_, body = offerOn(t, e, eve, id, 950) // at least 90%: auto-accepted
	if o := body["offer"].(map[string]any); o["status"] != "accepted" || o["offers_left"].(float64) != 1 {
		t.Fatalf("auto accept: %v", body)
	}
	if q := publicAuction(t, e, id)["quantity"].(float64); q != 1 {
		t.Fatalf("quantity after accept: %v", q)
	}

	// In between: the seller counters, the buyer counters back, the seller accepts.
	_, body = offerOn(t, e, finn, id, 700)
	first := body["offer"].(map[string]any)
	if first["status"] != "pending" {
		t.Fatalf("pending offer: %v", body)
	}
	if code, _ := offerOn(t, e, finn, id, 750); code != 409 {
		t.Fatalf("second pending offer: %d", code)
	}
	_, gb := offerOn(t, e, gus, id, 650)
	gusOffer := gb["offer"].(map[string]any)["id"].(string)

	respond := func(c *httpClientT, oid, action string, cents int) (int, map[string]any) {
		return do(t, c.c, "POST", e.ts.URL+"/v1/offers/"+oid+"/respond", map[string]any{"action": action, "amount_cents": cents})
	}
	if code, _ := respond(finn, first["id"].(string), "accept", 0); code != 409 {
		t.Fatalf("buyer accepting own offer: %d", code)
	}
	if code, _ := respond(seller, first["id"].(string), "counter", 1200); code != 422 {
		t.Fatalf("counter above price: %d", code)
	}
	code, body := respond(seller, first["id"].(string), "counter", 880)
	counter := body["offer"].(map[string]any)
	if code != 200 || counter["from_role"] != "seller" || counter["status"] != "pending" {
		t.Fatalf("seller counter: %d %v", code, body)
	}
	_, mine := do(t, finn.c, "GET", e.ts.URL+"/v1/offers", nil)
	if top := mine["offers"].([]any)[0].(map[string]any); top["can_respond"] != true || top["amount_cents"].(float64) != 880 {
		t.Fatalf("buyer sees counter: %v", mine)
	}
	code, body = respond(finn, counter["id"].(string), "counter", 820)
	back := body["offer"].(map[string]any)
	if code != 200 || back["status"] != "pending" || back["offers_left"].(float64) != 1 {
		t.Fatalf("buyer counter back: %d %v", code, body)
	}
	if code, body := respond(seller, back["id"].(string), "accept", 0); code != 200 || body["offer"].(map[string]any)["status"] != "accepted" {
		t.Fatalf("seller accepts: %d %v", code, body)
	}
	// Last card gone: gus's offer is closed and the listing is sold out.
	var gusStatus string
	e.db.QueryRow(`SELECT status FROM offers WHERE id = $1`, gusOffer).Scan(&gusStatus)
	if gusStatus != "declined" {
		t.Fatalf("other offers should close: %s", gusStatus)
	}
	if code, _ := do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+id, nil); code != 404 {
		t.Fatalf("sold-out listing should be gone: %d", code)
	}
	_, sh := do(t, sc, "GET", e.ts.URL+"/v1/holds?side=selling", nil)
	if len(sh["holds"].([]any)) != 2 {
		t.Fatalf("seller holds: %v", sh)
	}

	// Unanswered offers expire after 48 hours.
	l2 := createListing(t, e, seller, func() map[string]any {
		c := rawCard("O3", 1000)
		c["offers"] = map[string]any{"enabled": true}
		return c
	}())
	_, body = offerOn(t, e, gus, l2["id"].(string), 300)
	oid := body["offer"].(map[string]any)["id"].(string)
	e.db.Exec(`UPDATE offers SET expires_at = now() - interval '1 second' WHERE id = $1`, oid)
	if n, err := e.sales.ExpireOffers(ctx); err != nil || n != 1 {
		t.Fatalf("expire: %d %v", n, err)
	}
	if code, _ := respond(seller, oid, "accept", 0); code != 409 {
		t.Fatalf("accept expired: %d", code)
	}
	// Offers off: no offers accepted.
	if code, _ := offerOn(t, e, gus, createListing(t, e, seller, rawCard("O4", 900))["id"].(string), 500); code != 409 {
		t.Fatalf("offers off: %d", code)
	}
}
