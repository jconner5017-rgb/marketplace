package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

// paidOrder makes a shipped order from seller to buyer through checkout.
func paidOrder(t *testing.T, e *env, sc *httpClientT, b *httpClientT, sellerName, sku string, cents int) string {
	t.Helper()
	id := createListing(t, e, sc, rawCard(sku, cents))["id"].(string)
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+id, map[string]int{"quantity": 1})
	code, body := do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": sellerName})
	if code != 201 {
		t.Fatalf("checkout: %d %v", code, body)
	}
	order := body["order_id"].(string)
	e.pay.Complete(sessionOf(t, e, order))
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/confirm", map[string]any{})
	return order
}

func rate(r int, comment string) map[string]any {
	return map[string]any{"rating": r, "comment": comment, "item_rating": 5, "shipping_rating": 4, "communication_rating": 5}
}

func TestFeedbackModel(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	scc, _ := newSeller(t, e, "fb_shop", true)
	sc := &httpClientT{scc}
	e.db.Exec(`UPDATE seller_profiles SET tier_locked = true`)
	bea := buyer(t, e, "bea")
	o1 := paidOrder(t, e, sc, bea, "fb_shop", "F1", 1500)
	fb := func(c *httpClientT, order string, body map[string]any) (int, map[string]any) {
		return do(t, c.c, "POST", e.ts.URL+"/v1/orders/"+order+"/feedback", body)
	}

	// Buyers rate only after shipping; sellers can't leave negatives.
	if code, b := fb(bea, o1, rate(1, "")); code != 409 {
		t.Fatalf("before shipping: %d %v", code, b)
	}
	if code, _ := fb(sc, o1, map[string]any{"rating": -1, "comment": "slow to pay"}); code != 422 {
		t.Fatalf("seller negative: %d", code)
	}
	if code, b := fb(sc, o1, map[string]any{"rating": 1, "comment": "Paid fast"}); code != 200 || b["result"].(map[string]any)["visible"] != false {
		t.Fatalf("seller positive: %d %v", code, b)
	}
	do(t, scc, "POST", e.ts.URL+"/v1/orders/"+o1+"/ship", map[string]any{"carrier": "USPS", "tracking_number": "9400111899223344556677"})
	_, b := do(t, bea.c, "GET", e.ts.URL+"/v1/orders/"+o1, nil)
	if o := b["order"].(map[string]any); o["their_feedback"] != nil || o["their_feedback_hidden"] != true {
		t.Fatalf("buyer sees seller's feedback early: %v", o)
	}
	if code, _ := fb(bea, o1, map[string]any{"rating": 1}); code != 422 {
		t.Fatalf("missing sub-ratings: %d", code)
	}
	code, b := fb(bea, o1, rate(1, "Perfect, well packed"))
	if code != 200 || b["result"].(map[string]any)["visible"] != true || b["order"].(map[string]any)["their_feedback"] == nil {
		t.Fatalf("both rated: %d %v", code, b)
	}
	if code, _ := fb(bea, o1, rate(1, "again")); code != 409 {
		t.Fatalf("rate twice: %d", code)
	}

	// A negative needs contact first; a new seller's first one goes to mediation.
	cy := buyer(t, e, "cyd")
	o2 := paidOrder(t, e, sc, cy, "fb_shop", "F2", 1500)
	do(t, scc, "POST", e.ts.URL+"/v1/orders/"+o2+"/ship", map[string]any{"carrier": "USPS", "tracking_number": "9400111899223344556678"})
	if code, b := fb(cy, o2, rate(-1, "Card arrived creased")); code != 409 || errCode(b) != "resolve_first" {
		t.Fatalf("negative without contact: %d %v", code, b)
	}
	if code, b := do(t, cy.c, "POST", e.ts.URL+"/v1/orders/"+o2+"/messages", map[string]string{"body": "The card arrived creased, can you help?"}); code != 201 {
		t.Fatalf("order message: %d %v", code, b)
	}
	code, b = fb(cy, o2, rate(-1, "Card arrived creased"))
	if code != 200 || b["result"].(map[string]any)["state"] != "mediation" {
		t.Fatalf("new seller grace: %d %v", code, b)
	}
	_, b = do(t, client(t), "GET", e.ts.URL+"/v1/members/fb_shop/feedback", nil)
	sum := b["summary"].(map[string]any)
	if sum["score"].(float64) != 1 || sum["under_review"].(float64) != 1 || sum["positive_percent"].(float64) != 100 {
		t.Fatalf("summary during mediation: %v", sum)
	}
	mod := staff(t, e, "fb_mod", admin.Moderator)
	enrollMFA(t, e.ts.URL, mod.c)
	_, b = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/feedback?filter=mediation", nil)
	fid := b["feedback"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/feedback/"+fid+"/moderate", map[string]any{"action": "publish", "reason": "Photos show a crease"}); code != 204 {
		t.Fatalf("publish: %d", code)
	}
	// Still hidden until the seller rates or the window closes.
	e.db.Exec(`UPDATE orders SET shipped_at = now() - interval '30 days' WHERE id = $1`, o2)
	if err := e.fb.RunJobs(ctx); err != nil {
		t.Fatal(err)
	}
	_, b = do(t, client(t), "GET", e.ts.URL+"/v1/members/fb_shop/feedback", nil)
	sum = b["summary"].(map[string]any)
	if sum["score"].(float64) != 0 || sum["positive_percent"].(float64) != 50 || sum["negative_12m"].(float64) != 1 {
		t.Fatalf("summary after publish and reveal: %v", sum)
	}
	entries := b["feedback"].([]any)
	if len(entries) != 2 || entries[0].(map[string]any)["from"] != "c***d" {
		t.Fatalf("entries: %v", entries)
	}
	neg := entries[0].(map[string]any)["id"].(string)

	// Seller's one public reply, 300 characters at most.
	if code, _ := do(t, scc, "POST", e.ts.URL+"/v1/feedback/"+neg+"/reply", map[string]string{"reply": strings.Repeat("x", 301)}); code != 422 {
		t.Fatalf("long reply: %d", code)
	}
	if code, _ := do(t, scc, "POST", e.ts.URL+"/v1/feedback/"+neg+"/reply", map[string]string{"reply": "Sorry! I offered a full refund."}); code != 200 {
		t.Fatalf("reply: %d", code)
	}
	if code, _ := do(t, scc, "POST", e.ts.URL+"/v1/feedback/"+neg+"/reply", map[string]string{"reply": "Again"}); code != 409 {
		t.Fatalf("second reply: %d", code)
	}

	// An established seller's negative is held 3 days; the buyer can improve it.
	e.db.Exec(`INSERT INTO orders (buyer_id, seller_id, status, subtotal_cents, total_cents, paid_at)
		SELECT (SELECT id FROM users WHERE username = 'bea'), (SELECT id FROM users WHERE username = 'fb_shop'), 'delivered', 500, 500,
		       now() - interval '20 days' FROM generate_series(1, 10)`)
	dee := buyer(t, e, "dee")
	o3 := paidOrder(t, e, sc, dee, "fb_shop", "F3", 1500)
	do(t, scc, "POST", e.ts.URL+"/v1/orders/"+o3+"/ship", map[string]any{"carrier": "USPS", "tracking_number": "9400111899223344556679"})
	do(t, dee.c, "POST", e.ts.URL+"/v1/orders/"+o3+"/messages", map[string]string{"body": "Wrong card in the envelope"})
	if code, b := fb(dee, o3, rate(-1, "Wrong card sent")); code != 200 || b["result"].(map[string]any)["state"] != "held" {
		t.Fatalf("held: %d %v", code, b)
	}
	if code, b := fb(dee, o3, rate(1, "Seller fixed it quickly, thanks")); code != 200 || b["result"].(map[string]any)["revised"] != true {
		t.Fatalf("revise: %d %v", code, b)
	}
	var st string
	var rating int
	e.db.QueryRow(`SELECT state, rating FROM feedback WHERE order_id = $1`, o3).Scan(&st, &rating)
	if st != "published" || rating != 1 {
		t.Fatalf("after revise: %s %d", st, rating)
	}

	// Extortion is scrubbed and flagged; the rating stands.
	eli := buyer(t, e, "eli")
	o4 := paidOrder(t, e, sc, eli, "fb_shop", "F4", 1500)
	do(t, scc, "POST", e.ts.URL+"/v1/orders/"+o4+"/ship", map[string]any{"carrier": "USPS", "tracking_number": "9400111899223344556680"})
	fb(eli, o4, rate(0, "Send me $20 back or I leave a negative"))
	var comment, scrubbed string
	e.db.QueryRow(`SELECT comment, coalesce(scrubbed_comment, '') FROM feedback WHERE order_id = $1`, o4).Scan(&comment, &scrubbed)
	var alerts int
	e.db.QueryRow(`SELECT count(*) FROM fraud_alerts WHERE kind = 'feedback_extortion'`).Scan(&alerts)
	if comment != "" || !strings.Contains(scrubbed, "$20") || alerts != 1 {
		t.Fatalf("extortion: %q %q %d", comment, scrubbed, alerts)
	}

	// Automatic positive for buyers when payment clears.
	do(t, scc, "PUT", e.ts.URL+"/v1/seller/feedback", map[string]string{"auto_feedback": "paid", "auto_feedback_comment": "Smooth transaction, thanks!"})
	fay := buyer(t, e, "fay")
	o5 := paidOrder(t, e, sc, fay, "fb_shop", "F5", 1500)
	e.fb.RunJobs(ctx)
	var auto bool
	e.db.QueryRow(`SELECT auto FROM feedback WHERE order_id = $1 AND role = 'seller_to_buyer'`, o5).Scan(&auto)
	if !auto {
		t.Fatal("no automatic feedback")
	}

	// Nudge buyers who haven't rated, once.
	e.db.Exec(`UPDATE orders SET status = 'shipped', shipped_at = now() - interval '11 days' WHERE id = $1`, o5)
	e.fb.RunJobs(ctx)
	e.fb.RunJobs(ctx)
	var nudges int
	e.db.QueryRow(`SELECT count(*) FROM notifications WHERE title = 'Rate your purchase' AND user_id = (SELECT id FROM users WHERE username = 'fay')`).Scan(&nudges)
	if nudges != 1 {
		t.Fatalf("nudges: %d", nudges)
	}

	// Buyer record: for sellers dealing with the buyer only.
	_, b = do(t, scc, "GET", e.ts.URL+"/v1/members/fay/buyer-record", nil)
	if rec := b["buyer"].(map[string]any); rec["orders_paid"].(float64) != 1 {
		t.Fatalf("buyer record: %v", b)
	}
	other, _ := newSeller(t, e, "other_shop", true)
	if code, _ := do(t, other, "GET", e.ts.URL+"/v1/members/fay/buyer-record", nil); code != 403 {
		t.Fatalf("unrelated seller: %d", code)
	}

	// Outside feedback, verified by staff, shown separately.
	if code, _ := do(t, scc, "POST", e.ts.URL+"/v1/seller/outside-feedback", map[string]any{"platform": "ebay", "profile_url": "https://evil.example.com/usr/x", "feedback_count": 900, "positive_percent": 99.5}); code != 422 {
		t.Fatalf("wrong host: %d", code)
	}
	code, b = do(t, scc, "POST", e.ts.URL+"/v1/seller/outside-feedback", map[string]any{"platform": "ebay", "profile_url": "https://www.ebay.com/usr/fbshop", "feedback_count": 900, "positive_percent": 99.5})
	if code != 201 || !strings.HasPrefix(b["claim"].(map[string]any)["verify_code"].(string), "SLABS-") {
		t.Fatalf("claim: %d %v", code, b)
	}
	cid := b["claim"].(map[string]any)["id"].(string)
	boss := staff(t, e, "fb_boss", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/outside-feedback/"+cid, map[string]string{"decision": "verified", "note": "Code found in bio"}); code != 200 {
		t.Fatalf("verify: %d", code)
	}
	_, b = do(t, client(t), "GET", e.ts.URL+"/v1/members/fb_shop/feedback", nil)
	out := b["summary"].(map[string]any)["outside"].([]any)
	if len(out) != 1 || out[0].(map[string]any)["feedback_count"].(float64) != 900 {
		t.Fatalf("outside: %v", b["summary"])
	}
}
