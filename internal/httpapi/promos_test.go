package httpapi

import (
	"strconv"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

func TestPromoCodesWaiveFees(t *testing.T) {
	e := setupEnv(t)
	boss := staff(t, e, "boss_promo", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	sc, _ := newSeller(t, e, "promo_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true`)

	// Staff create a code; sellers can't.
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/admin/promos", map[string]any{"code": "x"}); code != 403 && code != 404 {
		t.Fatalf("seller creating promo: %d", code)
	}
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/promos", map[string]any{"code": "a b", "fee_free_sales": 2}); code != 422 {
		t.Fatalf("bad code accepted: %d %v", code, body)
	}
	code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/promos", map[string]any{
		"code": "shop2", "label": "Test shop", "audience": "business", "fee_free_sales": 2, "max_redemptions": 1,
		"new_sellers_only": true, "active": true})
	if code != 201 {
		t.Fatalf("create promo: %d %v", code, body)
	}
	id := int(body["promo"].(map[string]any)["id"].(float64))
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/promos", map[string]any{"code": "SHOP2", "fee_free_sales": 5, "active": true}); code != 422 {
		t.Fatalf("duplicate code: %d", code)
	}

	// The seller redeems it (case doesn't matter); a second try is refused.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/promos", map[string]any{"code": "nope"}); code != 404 {
		t.Fatalf("unknown code: %d %v", code, body)
	}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/seller/promos", map[string]any{"code": " shop2 "})
	if code != 200 || body["fee_free_sales_left"].(float64) != 2 {
		t.Fatalf("redeem: %d %v", code, body)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/promos", map[string]any{"code": "SHOP2"}); code != 409 {
		t.Fatalf("redeem twice: %d", code)
	}
	// Single use: another seller can't have it.
	other, _ := newSeller(t, e, "promo_other", true)
	if code, _ := do(t, other, "POST", e.ts.URL+"/v1/seller/promos", map[string]any{"code": "SHOP2"}); code != 409 {
		t.Fatalf("over max redemptions: %d", code)
	}

	// Three cards in one order: two are fee-free, the third pays the fee.
	card := rawCard("PR1", 1000)
	card["quantity"] = 3
	l := createListing(t, e, &httpClientT{sc}, card)["id"].(string)
	b := buyer(t, e, "promo_buyer")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+l, map[string]int{"quantity": 3})
	checkout := func() (string, int, int) {
		t.Helper()
		code, body := do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "promo_shop"})
		if code != 201 {
			t.Fatalf("checkout: %d %v", code, body)
		}
		o := body["order_id"].(string)
		var fee, waived int
		e.db.QueryRow(`SELECT fee_cents, fee_waived_cents FROM orders WHERE id = $1`, o).Scan(&fee, &waived)
		return o, fee, waived
	}
	o1, fee, waived := checkout()
	// Every card costs the same here, so the fee left is one card's worth.
	if fee <= 0 || waived != 2*fee {
		t.Fatalf("fee %d waived %d, want two of three cards waived", fee, waived)
	}
	unitFee := fee
	if p := e.pay.Params(sessionOf(t, e, o1)); p.FeeCents != unitFee {
		t.Fatalf("stripe fee %d", p.FeeCents)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/seller/promos", nil)
	if body["fee_free_sales_left"].(float64) != 0 {
		t.Fatalf("left after order: %v", body)
	}
	// Pressing Pay again cancels the first checkout, which gives its sales
	// back before the new one uses them.
	o2, fee, waived := checkout()
	if o2 == o1 || fee != unitFee || waived != 2*unitFee {
		t.Fatalf("second checkout fee %d waived %d", fee, waived)
	}
	var used int
	e.db.QueryRow(`SELECT sales_used FROM promo_redemptions`).Scan(&used)
	if used != 2 {
		t.Fatalf("sales used %d, want 2", used)
	}

	// Staff can pause and edit; the seller keeps what they redeemed.
	if code, body := do(t, boss.c, "PATCH", e.ts.URL+"/v1/admin/promos/"+strconv.Itoa(id), map[string]any{
		"code": "SHOP2", "label": "Paused", "audience": "business", "fee_free_sales": 10, "active": false}); code != 200 {
		t.Fatalf("edit: %d %v", code, body)
	}
	code, body = do(t, boss.c, "GET", e.ts.URL+"/v1/admin/promos/"+strconv.Itoa(id), nil)
	if code != 200 || len(body["redemptions"].([]any)) != 1 || body["promo"].(map[string]any)["active"] != false {
		t.Fatalf("detail: %d %v", code, body)
	}
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/admin/promos", nil)
	if p := body["promos"].([]any)[0].(map[string]any); p["sales_used"].(float64) != 2 || p["redemptions"].(float64) != 1 {
		t.Fatalf("list: %v", p)
	}

	// New sellers only: a seller with a paid sale can't use such a code.
	do(t, boss.c, "POST", e.ts.URL+"/v1/admin/promos", map[string]any{"code": "WELCOME", "fee_free_sales": 5, "new_sellers_only": true, "active": true})
	e.pay.Complete(sessionOf(t, e, o2))
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+o2+"/confirm", map[string]any{})
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/promos", map[string]any{"code": "welcome"}); code != 409 {
		t.Fatalf("not-new seller redeemed: %d %v", code, body)
	}
	if code, _ := do(t, other, "POST", e.ts.URL+"/v1/seller/promos", map[string]any{"code": "welcome"}); code != 200 {
		t.Fatalf("new seller refused: %d", code)
	}
	// A plain end date works through that day; a past one is expired.
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/promos", map[string]any{"code": "OLD", "fee_free_sales": 5, "expires_at": "2020-01-01", "active": true}); code != 201 {
		t.Fatalf("dated code: %d %v", code, body)
	}
	if code, _ := do(t, other, "POST", e.ts.URL+"/v1/seller/promos", map[string]any{"code": "old"}); code != 409 {
		t.Fatalf("expired code redeemed: %d", code)
	}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/promos", map[string]any{"code": "BADDATE", "fee_free_sales": 5, "expires_at": "soon", "active": true}); code != 422 {
		t.Fatalf("bad date: %d", code)
	}
}
