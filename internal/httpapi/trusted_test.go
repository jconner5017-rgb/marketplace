package httpapi

import "testing"

func TestTrustedBuyerEnvelopes(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "trust_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true`)
	seller := &httpClientT{sc}
	card := rawCard("TR1", 5000)
	card["package_preset"] = "bubble"
	card["quantity"] = 5
	l := createListing(t, e, seller, card)["id"].(string)
	pal := buyer(t, e, "trust_pal")

	pay := func(b *httpClientT, envelope bool) (int, map[string]any) {
		t.Helper()
		do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+l, map[string]int{"quantity": 1})
		return do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "trust_shop", "envelope": envelope})
	}
	finish := func(b *httpClientT, o string) {
		t.Helper()
		e.pay.Complete(sessionOf(t, e, o))
		do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+o+"/confirm", map[string]any{})
	}

	// Not trusted yet: no envelope choice, and asking for one is refused.
	_, cart := do(t, pal.c, "GET", e.ts.URL+"/v1/cart", nil)
	_ = cart
	if code, body := pay(pal, true); code != 409 {
		t.Fatalf("untrusted envelope: %d %v", code, body)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/trusted", map[string]any{"username": "nobody_here"}); code != 422 {
		t.Fatalf("unknown buyer: %d", code)
	}
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/trusted", map[string]any{"username": "@Trust_Pal"}); code != 200 || len(body["buyers"].([]any)) != 1 {
		t.Fatalf("add trusted: %d %v", code, body)
	}
	_, cart = do(t, pal.c, "GET", e.ts.URL+"/v1/cart", nil)
	if g := cart["groups"].([]any)[0].(map[string]any); g["envelope_choice"] != true || g["ship_method"] != "label" {
		t.Fatalf("cart choice: %v", g)
	}
	code, body := pay(pal, true)
	if code != 201 {
		t.Fatalf("trusted envelope checkout: %d %v", code, body)
	}
	o := body["order_id"].(string)
	finish(pal, o)
	getOrder := func(c *httpClientT, id string) map[string]any {
		t.Helper()
		_, b := do(t, c.c, "GET", e.ts.URL+"/v1/orders/"+id, nil)
		return b["order"].(map[string]any)
	}
	ord := getOrder(seller, o)
	if ord["ship_method"] != "envelope" || ord["envelope_trusted"] != true || ord["can_offer_envelope"] != false {
		t.Fatalf("order: %v", ord)
	}
	// Over $20 by envelope: the tracked envelope has to be printed first.
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+o+"/ship", map[string]any{}); code != 422 {
		t.Fatalf("shipped without barcode: %d %v", code, body)
	}
	e.db.Exec(`INSERT INTO envelope_barcodes (order_id, tracking, routing) VALUES ($1, '00040901234567000001', '94105')`, o)
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+o+"/ship", map[string]any{}); code != 200 {
		t.Fatalf("ship envelope: %d %v", code, body)
	}
	// A missing envelope isn't covered; other problems still are.
	if code, _ := do(t, pal.c, "POST", e.ts.URL+"/v1/orders/"+o+"/problem", map[string]any{"kind": "inr", "summary": "It never arrived at all"}); code != 422 {
		t.Fatalf("inr on trusted envelope: %d", code)
	}
	if code, body := do(t, pal.c, "POST", e.ts.URL+"/v1/orders/"+o+"/problem", map[string]any{"kind": "snad", "summary": "Card is creased badly"}); code != 201 && code != 200 {
		t.Fatalf("snad: %d %v", code, body)
	}

	// Invite link: a new account joins the list before buying.
	_, tb := do(t, sc, "GET", e.ts.URL+"/v1/seller/trusted", nil)
	invite := tb["invite"].(string)
	if len(invite) < 10 {
		t.Fatalf("invite: %v", tb)
	}
	newbie := buyer(t, e, "trust_newbie")
	if code, body := do(t, newbie.c, "GET", e.ts.URL+"/v1/trust/"+invite, nil); code != 200 || body["seller"] != "trust_shop" {
		t.Fatalf("invite info: %d %v", code, body)
	}
	if code, _ := do(t, newbie.c, "POST", e.ts.URL+"/v1/trust/"+invite, map[string]any{}); code != 200 {
		t.Fatalf("accept invite: %d", code)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/trust/"+invite, map[string]any{}); code != 409 {
		t.Fatalf("seller trusting self: %d", code)
	}
	_, tb = do(t, sc, "POST", e.ts.URL+"/v1/seller/trusted/invite", map[string]any{})
	if tb["invite"] == invite || len(tb["buyers"].([]any)) != 2 {
		t.Fatalf("reset invite: %v", tb)
	}
	if code, _ := do(t, newbie.c, "GET", e.ts.URL+"/v1/trust/"+invite, nil); code != 404 {
		t.Fatalf("old invite still works: %d", code)
	}

	// After the sale: a buyer who wasn't trusted gets an offer to accept.
	late := buyer(t, e, "trust_late")
	code, body = pay(late, false)
	if code != 201 {
		t.Fatalf("label checkout: %d %v", code, body)
	}
	o2 := body["order_id"].(string)
	finish(late, o2)
	ord = getOrder(seller, o2)
	if ord["ship_method"] != "label" || ord["can_offer_envelope"] != true {
		t.Fatalf("label order: %v", ord)
	}
	if code, _ := do(t, late.c, "POST", e.ts.URL+"/v1/orders/"+o2+"/envelope-offer/accept", map[string]any{}); code != 409 {
		t.Fatalf("accept before offer: %d", code)
	}
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+o2+"/envelope-offer", map[string]any{}); code != 200 || body["order"].(map[string]any)["envelope_offer"] != "offered" {
		t.Fatalf("offer: %d %v", code, body)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+o2+"/envelope-offer", map[string]any{}); code != 409 {
		t.Fatalf("offer twice: %d", code)
	}
	if code, body := do(t, late.c, "POST", e.ts.URL+"/v1/orders/"+o2+"/envelope-offer/accept", map[string]any{}); code != 200 {
		t.Fatalf("accept: %d %v", code, body)
	}
	if ord = getOrder(late, o2); ord["ship_method"] != "envelope" || ord["envelope_trusted"] != true || ord["envelope_offer"] != "accepted" {
		t.Fatalf("accepted order: %v", ord)
	}
	_, tb = do(t, sc, "GET", e.ts.URL+"/v1/seller/trusted", nil)
	if len(tb["buyers"].([]any)) != 3 {
		t.Fatalf("offer should trust buyer: %v", tb)
	}

	// Over $100 never ships by envelope.
	big := rawCard("TR2", 15000)
	big["package_preset"] = "bubble"
	bl := createListing(t, e, seller, big)["id"].(string)
	do(t, pal.c, "PUT", e.ts.URL+"/v1/cart/"+bl, map[string]int{"quantity": 1})
	if code, _ := do(t, pal.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "trust_shop", "envelope": true}); code != 409 {
		t.Fatalf("over $100 envelope: %d", code)
	}
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/seller/trusted/trust_pal", nil); code != 200 {
		t.Fatalf("remove: %d", code)
	}
}
