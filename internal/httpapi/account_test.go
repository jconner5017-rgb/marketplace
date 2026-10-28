package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// buyer signs up a plain account (no Stripe onboarding).
func buyer(t *testing.T, e *env, name string) *httpClientT {
	t.Helper()
	c := client(t)
	code, body := do(t, c, "POST", e.ts.URL+"/v1/auth/signup", map[string]string{
		"email": name + "@example.com", "username": name, "password": "longenough1",
	})
	if code != 201 {
		t.Fatalf("signup %s: %d %v", name, code, body)
	}
	return &httpClientT{c}
}

func TestWatchlistCartAndHeaderSummary(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "shop_owner", true)
	do(t, sc, "PATCH", e.ts.URL+"/v1/seller/settings", map[string]any{"bundle_discount_type": "per_extra_item", "bundle_discount_val": 50})
	ids := []string{}
	for i, cents := range []int{1000, 500, 300} {
		card := rawCard("W"+string(rune('A'+i)), cents)
		if i == 2 {
			card["quantity"] = 2
		}
		code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", card)
		if code != 201 {
			t.Fatalf("create: %d %v", code, body)
		}
		ids = append(ids, body["listing"].(map[string]any)["id"].(string))
	}

	// Signed-out visitors can't watch or add to cart.
	anon := client(t)
	if code, _ := do(t, anon, "PUT", e.ts.URL+"/v1/cart/"+ids[0], map[string]int{"quantity": 1}); code != 401 {
		t.Fatalf("anon cart: want 401, got %d", code)
	}
	if code, _ := do(t, anon, "PUT", e.ts.URL+"/v1/watchlist/"+ids[0], map[string]any{}); code != 401 {
		t.Fatalf("anon watch: want 401, got %d", code)
	}

	b := buyer(t, e, "collector")
	// Watch, twice is fine.
	for i := 0; i < 2; i++ {
		if code, body := do(t, b.c, "PUT", e.ts.URL+"/v1/watchlist/"+ids[0], map[string]any{}); code != 200 {
			t.Fatalf("watch: %d %v", code, body)
		}
	}
	_, body := do(t, b.c, "GET", e.ts.URL+"/v1/watchlist", nil)
	if n := len(body["items"].([]any)); n != 1 {
		t.Fatalf("watchlist: %d items", n)
	}
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/listings/"+ids[0]+"/me", nil)
	if body["watching"] != true || body["own"] != false {
		t.Fatalf("listing/me: %v", body)
	}

	// Cart: three cards from one seller, $0.50 off each extra card.
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+ids[0], map[string]int{"quantity": 1})
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+ids[1], map[string]int{"quantity": 1})
	code, body := do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+ids[2], map[string]int{"quantity": 2})
	if code != 200 {
		t.Fatalf("add to cart: %d %v", code, body)
	}
	g := body["groups"].([]any)[0].(map[string]any)
	// 1000 + 500 + 2*300 = 2100; 4 cards → 3 extra × 50 = 150 off.
	if body["card_count"].(float64) != 4 || g["subtotal_cents"].(float64) != 2100 ||
		g["discount_cents"].(float64) != 150 || body["total_cents"].(float64) != 1950 || g["ship_method"] != "label" {
		t.Fatalf("cart math: %v", body)
	}
	if code, body := do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+ids[2], map[string]int{"quantity": 3}); code != 422 || errCode(body) != "not_enough" {
		t.Fatalf("too many: %d %v", code, body)
	}

	// Seller can't buy their own card.
	if code, body := do(t, sc, "PUT", e.ts.URL+"/v1/cart/"+ids[0], map[string]int{"quantity": 1}); code != 422 || errCode(body) != "own_listing" {
		t.Fatalf("own listing: %d %v", code, body)
	}

	// Header summary.
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/me/summary", nil)
	if body["cart_cards"].(float64) != 4 || body["watching"].(float64) != 1 || body["is_seller"] != false {
		t.Fatalf("summary: %v", body)
	}

	// Seller hides a card: it stays in the cart, flagged, and leaves the total.
	do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+ids[1], map[string]any{"status": "hidden"})
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/cart", nil)
	if body["problems"].(float64) != 1 || body["card_count"].(float64) != 3 {
		t.Fatalf("cart after hide: %v", body)
	}
	_, body = do(t, b.c, "DELETE", e.ts.URL+"/v1/cart/"+ids[1], nil)
	if body["problems"].(float64) != 0 {
		t.Fatalf("cart after remove: %v", body)
	}

	// One raw $3 card in an envelope ships with a stamp.
	b2 := buyer(t, e, "small_buyer")
	_, body = do(t, b2.c, "PUT", e.ts.URL+"/v1/cart/"+ids[2], map[string]int{"quantity": 1})
	if g := body["groups"].([]any)[0].(map[string]any); g["ship_method"] != "envelope" {
		t.Fatalf("single cheap card should ship by envelope: %v", g)
	}
}

func TestMessagesBellAndAnnouncements(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "seller_x", true)
	_, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", rawCard("M1", 1500))
	lid := body["listing"].(map[string]any)["id"].(string)

	b := buyer(t, e, "asker")
	// Off-platform payment talk is blocked and never reaches the seller.
	code, body := do(t, b.c, "POST", e.ts.URL+"/v1/listings/"+lid+"/messages", map[string]string{"body": "Can I pay you on Venmo instead?"})
	if code != 422 || errCode(body) != "off_platform" {
		t.Fatalf("venmo message: %d %v", code, body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/me/summary", nil)
	if body["unread_notices"].(float64) != 0 {
		t.Fatalf("blocked message notified the seller: %v", body)
	}

	code, body = do(t, b.c, "POST", e.ts.URL+"/v1/listings/"+lid+"/messages", map[string]string{"body": "Any whitening on the back corners?"})
	if code != 201 {
		t.Fatalf("ask seller: %d %v", code, body)
	}
	thread := body["thread_id"].(string)

	// Seller's bell lights up.
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/notifications", nil)
	notices := body["notices"].([]any)
	if body["unread"].(float64) != 1 || len(notices) != 1 ||
		!strings.Contains(notices[0].(map[string]any)["title"].(string), "asker sent you a message") {
		t.Fatalf("seller bell: %v", body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/messages", nil)
	ts := body["threads"].([]any)
	if len(ts) != 1 || ts[0].(map[string]any)["unread"] != true || ts[0].(map[string]any)["role"] != "seller" {
		t.Fatalf("seller threads: %v", body)
	}

	// Opening the conversation reads it and clears the bell notice.
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/messages/"+thread, nil)
	if msgs := body["messages"].([]any); len(msgs) != 1 {
		t.Fatalf("seller sees blocked message or wrong count: %v", msgs)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/me/summary", nil)
	if body["unread_notices"].(float64) != 0 || body["unread_messages"].(float64) != 0 {
		t.Fatalf("after reading: %v", body)
	}

	// Reply reaches the buyer.
	code, _ = do(t, sc, "POST", e.ts.URL+"/v1/messages/"+thread, map[string]string{"body": "Clean corners, sharp edges."})
	if code != 200 {
		t.Fatalf("reply: %d", code)
	}
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/me/summary", nil)
	if body["unread_messages"].(float64) != 1 {
		t.Fatalf("buyer unread: %v", body)
	}
	// Blocked messages are kept for review but shown to no one.
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/messages/"+thread, nil)
	if n := len(body["messages"].([]any)); n != 2 {
		t.Fatalf("buyer messages: %d, want 2", n)
	}
	var blocked int
	e.db.QueryRow(`SELECT count(*) FROM messages WHERE blocked IS NOT NULL`).Scan(&blocked)
	if blocked != 1 {
		t.Fatalf("blocked message not kept for review: %d", blocked)
	}

	// Strangers can't read the thread.
	stranger := buyer(t, e, "nosy")
	if code, _ := do(t, stranger.c, "GET", e.ts.URL+"/v1/messages/"+thread, nil); code != 404 {
		t.Fatalf("stranger read thread: %d", code)
	}

	// Announcements: admin only, shown in every bell.
	if code, _ := do(t, b.c, "POST", e.ts.URL+"/v1/admin/announcements", map[string]string{"title": "Hi"}); code != 403 {
		t.Fatalf("non-admin announce: %d", code)
	}
	admin := buyer(t, e, "site_admin")
	e.auth.SetRole(t.Context(), "site_admin@example.com", "admin")
	code, body = do(t, admin.c, "POST", e.ts.URL+"/v1/admin/announcements", map[string]string{
		"title": "Holiday shipping cutoff Dec 18", "body": "Ship by the 18th for Christmas delivery.", "link": "/help/shipping"})
	if code != 201 {
		t.Fatalf("announce: %d %v", code, body)
	}
	_, body = do(t, stranger.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if body["unread"].(float64) != 1 || body["notices"].([]any)[0].(map[string]any)["kind"] != "announcement" {
		t.Fatalf("announcement in bell: %v", body)
	}
	_, body = do(t, client(t), "GET", e.ts.URL+"/v1/announcements", nil)
	if n := len(body["announcements"].([]any)); n != 1 {
		t.Fatalf("public announcements: %d", n)
	}
	do(t, stranger.c, "POST", e.ts.URL+"/v1/notifications/read-all", map[string]any{})
	_, body = do(t, stranger.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if body["unread"].(float64) != 0 {
		t.Fatalf("after read-all: %v", body)
	}
}

type httpClientT struct{ c *http.Client }
