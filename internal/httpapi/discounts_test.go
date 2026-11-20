package httpapi

import (
	"strings"
	"testing"
	"time"
)

// Marketing & discounts (2026-11-20): a seller sets up a coupon, an order
// discount and a $ off sale event for some categories; the buyer's cart and
// checkout give the single best deal, the fee is on what the buyer pays, and
// a once-per-buyer coupon can't be used twice.
func TestDiscounts(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "deal_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true`)
	poke := func(ext string, cents, qty int) string {
		c := rawCard(ext, cents)
		c["quantity"] = qty
		return createListing(t, e, &httpClientT{sc}, c)["id"].(string)
	}
	ball := func(ext string, cents int) string {
		c := rawCard(ext, cents)
		c["category"], c["game_or_sport"], c["title"] = "sports", "Baseball", "Ken Griffey Jr. "+ext
		return createListing(t, e, &httpClientT{sc}, c)["id"].(string)
	}
	cheap := poke("D-CHEAP", 300, 25) // $3 Pokemon, 25 of them
	mid := ball("D-MID", 1000)        // $10 Baseball

	// Order discount: at least 20 cards up to $5 get 25% off.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/order-discounts", map[string]any{"min_cards": 1, "percent": 25}); code != 422 {
		t.Fatalf("1 card rule: %d %v", code, body)
	}
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/order-discounts", map[string]any{"min_cards": 20, "price_max_cents": 500, "percent": 25})
	if code != 201 || body["order_discount"].(map[string]any)["label"] != "25% off 20+ cards up to $5" {
		t.Fatalf("rule: %d %v", code, body)
	}
	// Coupon: $4 off Baseball cards, once per buyer.
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/coupons", map[string]any{"code": "x", "percent": 10}); code != 422 {
		t.Fatalf("short code: %d", code)
	}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/seller/coupons", map[string]any{"code": "griffey4", "amount_cents": 400, "games": []string{"Baseball"}})
	if code != 201 || body["coupon"].(map[string]any)["code"] != "GRIFFEY4" || body["coupon"].(map[string]any)["once_per_buyer"] != true {
		t.Fatalf("coupon: %d %v", code, body)
	}
	couponID := body["coupon"].(map[string]any)["id"].(float64)
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/coupons", map[string]any{"code": "GRIFFEY4", "percent": 10}); code != 422 {
		t.Fatalf("duplicate code: %d", code)
	}
	if _, body := do(t, sc, "GET", e.ts.URL+"/v1/seller/discounts", nil); len(body["coupons"].([]any)) != 1 || len(body["order_discounts"].([]any)) != 1 ||
		body["has_store"] != false {
		t.Fatalf("discounts page: %v", body)
	}

	pat := buyer(t, e, "deal_pat")
	group := func(cart map[string]any) map[string]any {
		gs := cart["groups"].([]any)
		if len(gs) != 1 {
			t.Fatalf("cart groups: %v", cart)
		}
		return gs[0].(map[string]any)
	}

	// 20 cheap cards + the $10 card: the order discount ($15) beats the coupon ($4).
	do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/"+cheap, map[string]int{"quantity": 20})
	do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/"+mid, map[string]int{"quantity": 1})
	if code, body := do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/coupon", map[string]string{"seller": "deal_shop", "code": "NOPE"}); code != 422 {
		t.Fatalf("bad coupon: %d %v", code, body)
	}
	code, body = do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/coupon", map[string]string{"seller": "deal_shop", "code": "griffey4"})
	g := group(body)
	if code != 200 || g["deal_kind"] != "order" || g["discount_cents"] != float64(1500) || g["total_cents"] != float64(5500) ||
		!strings.Contains(g["coupon_note"].(string), "better deal") {
		t.Fatalf("order beats coupon: %d %v", code, g)
	}
	// 19 cheap cards: no longer 20, so the coupon wins.
	do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/"+cheap, map[string]int{"quantity": 19})
	_, body = do(t, pat.c, "GET", e.ts.URL+"/v1/cart", nil)
	if g = group(body); g["deal_kind"] != "coupon" || g["discount_cents"] != float64(400) || g["total_cents"] != float64(6300) {
		t.Fatalf("coupon wins: %v", g)
	}

	// Checkout: the coupon is used, the order says so, the fee is on what's paid.
	code, body = do(t, pat.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "deal_shop"})
	if code != 201 {
		t.Fatalf("checkout: %d %v", code, body)
	}
	orderID := body["order_id"].(string)
	var kind, label string
	var disc, fee, total int
	e.db.QueryRow(`SELECT discount_kind, discount_label, discount_cents, fee_cents, total_cents FROM orders WHERE id = $1`, orderID).
		Scan(&kind, &label, &disc, &fee, &total)
	if kind != "coupon" || label != "GRIFFEY4: $4 off Baseball cards" || disc != 400 || total != 6300 {
		t.Fatalf("order: %s %q %d %d", kind, label, disc, total)
	}
	var feeList int
	e.db.QueryRow(`SELECT fee_cents FROM orders WHERE id = $1`, orderID).Scan(&feeList)
	if p := e.pay.Params(sessionOf(t, e, orderID)); !strings.Contains(p.Lines[0].Name, "GRIFFEY4") || p.Lines[0].UnitCents != 6300 {
		t.Fatalf("stripe line: %+v", p.Lines)
	}
	// Used up while that order is open: the cart refuses it.
	if code, body := do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/coupon", map[string]string{"seller": "deal_shop", "code": "GRIFFEY4"}); code != 422 ||
		!strings.Contains(body["error"].(map[string]any)["message"].(string), "already used") {
		t.Fatalf("second use: %d %v", code, body)
	}
	// The buyer leaves checkout: the coupon is free again.
	if code, _ := do(t, pat.c, "POST", e.ts.URL+"/v1/orders/"+orderID+"/cancel", nil); code != 200 && code != 204 {
		t.Fatalf("abandon: %d", code)
	}
	if code, body := do(t, pat.c, "PUT", e.ts.URL+"/v1/cart/coupon", map[string]string{"seller": "deal_shop", "code": "GRIFFEY4"}); code != 200 {
		t.Fatalf("coupon after cancel: %d %v", code, body)
	}
	// The seller switches it off: gone from the buyer's deal.
	do(t, sc, "PATCH", e.ts.URL+"/v1/seller/coupons/"+itoaT(int(couponID)), map[string]bool{"active": false})
	_, body = do(t, pat.c, "GET", e.ts.URL+"/v1/cart", nil)
	if g = group(body); g["deal_kind"] == "coupon" || !strings.Contains(g["coupon_note"].(string), "isn't valid") {
		t.Fatalf("switched-off coupon: %v", g)
	}

	// Sale event: $1 off Baseball cards (stores only). The $10 card shows $9 with
	// a SALE badge; the Pokemon cards don't change.
	e.db.Exec(`INSERT INTO stores (seller_id, slug, name, legal_name, business_type, status) SELECT id, 'deal-shop', 'Deal Shop', 'Deal Shop LLC', 'llc', 'active' FROM users WHERE username = 'deal_shop'`)
	ends := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/store/sales", map[string]any{"name": "Griffey week", "percent": 10, "amount_cents": 100, "ends_at": ends}); code != 422 {
		t.Fatalf("both kinds: %d %v", code, body)
	}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/seller/store/sales", map[string]any{"name": "Griffey week", "amount_cents": 100, "games": []string{"Baseball"}, "ends_at": ends})
	if code != 201 {
		t.Fatalf("sale: %d %v", code, body)
	}
	_, body = do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+mid, nil)
	if l := body["listing"].(map[string]any); l["sale_price_cents"] != float64(900) || l["sale_percent"] != float64(10) {
		t.Fatalf("sale price: %v", l)
	}
	_, body = do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+cheap, nil)
	if l := body["listing"].(map[string]any); l["sale_price_cents"] != nil {
		t.Fatalf("pokemon on sale: %v", l)
	}
	if _, body := do(t, client(t), "GET", e.ts.URL+"/v1/listings?on_sale=1&seller=deal_shop", nil); body["total"] != float64(1) {
		t.Fatalf("on sale filter: %v", body["total"])
	}
	// Cart: just the $10 card - the $1 sale is the only deal.
	do(t, pat.c, "DELETE", e.ts.URL+"/v1/cart/"+cheap, nil)
	_, body = do(t, pat.c, "GET", e.ts.URL+"/v1/cart", nil)
	if g = group(body); g["deal_kind"] != "sale" || g["total_cents"] != float64(900) || g["sale_savings_cents"] != float64(100) {
		t.Fatalf("sale in cart: %v", g)
	}
	// Removing an order discount.
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/seller/discounts", nil)
	rid := body["order_discounts"].([]any)[0].(map[string]any)["id"].(float64)
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/seller/order-discounts/"+itoaT(int(rid)), nil); code != 200 {
		t.Fatalf("remove rule: %d", code)
	}
	if _, body := do(t, sc, "GET", e.ts.URL+"/v1/seller/discounts", nil); len(body["order_discounts"].([]any)) != 0 || body["has_store"] != true ||
		len(body["sales"].([]any)) != 1 {
		t.Fatalf("after removing: %v", body)
	}
	_ = fee
}
