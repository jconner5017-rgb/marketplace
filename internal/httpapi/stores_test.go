package httpapi

import (
	"strconv"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

// A business seller applies for a store, staff approve it, the seller sets
// up a category and a 20% sale, buyers see and pay the sale price, follow
// the store and hear about new listings and updates.
func TestBusinessStore(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "card_shop", true)
	code, body := do(t, c, "PUT", e.ts.URL+"/v1/seller/listings/external/SHOP-1", rawCard("SHOP-1", 1000))
	if code != 200 && code != 201 {
		t.Fatalf("list: %d %v", code, body)
	}
	listingID := body["listing"].(map[string]any)["id"].(string)

	// Not open yet: no store, sales refused.
	if _, body := do(t, c, "GET", e.ts.URL+"/v1/seller/store", nil); body["store"] != nil {
		t.Fatalf("store before applying: %v", body)
	}
	app := map[string]any{"slug": "Card-Shop", "name": "The Card Shop", "tagline": "Vintage and graded",
		"legal_name": "Card Shop LLC", "business_type": "llc", "website": "cardshop.example.com"}
	if code, body := do(t, c, "PUT", e.ts.URL+"/v1/seller/store", map[string]any{"slug": "x", "name": "The Card Shop",
		"legal_name": "Card Shop LLC", "business_type": "llc"}); code != 422 {
		t.Fatalf("bad slug: %d %v", code, body)
	}
	code, body = do(t, c, "PUT", e.ts.URL+"/v1/seller/store", app)
	st, _ := body["store"].(map[string]any)
	if code != 200 || st["status"] != "pending" || st["slug"] != "card-shop" || st["website"] != "https://cardshop.example.com" {
		t.Fatalf("apply: %d %v", code, body)
	}
	if code, _ := do(t, client(t), "GET", e.ts.URL+"/v1/stores/card-shop", nil); code != 404 {
		t.Fatalf("pending store is public: %d", code)
	}
	if code, _ := do(t, c, "POST", e.ts.URL+"/v1/seller/store/sales", map[string]any{"name": "Grand opening", "percent": 20,
		"ends_at": time.Now().Add(48 * time.Hour)}); code != 403 {
		t.Fatalf("sale before approval: %d", code)
	}

	// Staff approve it.
	boss := staff(t, e, "store_boss", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/admin/stores?status=pending", nil)
	if list := body["stores"].([]any); len(list) != 1 || list[0].(map[string]any)["legal_name"] != "Card Shop LLC" {
		t.Fatalf("admin list: %v", body)
	}
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/stores/card_shop/review", map[string]any{"status": "active"}); code != 200 {
		t.Fatalf("approve: %d %v", code, body)
	}

	// Public page hides the business details.
	code, body = do(t, client(t), "GET", e.ts.URL+"/v1/stores/CARD-SHOP", nil)
	st, _ = body["store"].(map[string]any)
	if code != 200 || st["name"] != "The Card Shop" || st["legal_name"] != nil || st["listings"] != float64(1) {
		t.Fatalf("public store: %d %v", code, body)
	}

	// Category, then a 20% sale on it.
	code, body = do(t, c, "PUT", e.ts.URL+"/v1/seller/store/categories", map[string]any{"categories": []map[string]any{{"name": "Pokemon"}, {"name": "Baseball"}}})
	cats := body["store"].(map[string]any)["categories"].([]any)
	if code != 200 || len(cats) != 2 {
		t.Fatalf("categories: %d %v", code, body)
	}
	pokemon := int(cats[0].(map[string]any)["id"].(float64))
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/store/categorize", map[string]any{"category_id": pokemon, "listing_ids": []string{listingID}}); code != 200 || body["updated"] != float64(1) {
		t.Fatalf("categorize: %d %v", code, body)
	}
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/store/sales", map[string]any{"name": "Grand opening", "percent": 60,
		"ends_at": time.Now().Add(48 * time.Hour)}); code != 422 {
		t.Fatalf("60%% sale: %d %v", code, body)
	}
	code, body = do(t, c, "POST", e.ts.URL+"/v1/seller/store/sales", map[string]any{"name": "Grand opening", "percent": 20,
		"category_id": pokemon, "ends_at": time.Now().Add(48 * time.Hour)})
	if code != 201 {
		t.Fatalf("sale: %d %v", code, body)
	}
	saleID := int(body["sale"].(map[string]any)["id"].(float64))
	if code, _ := do(t, c, "POST", e.ts.URL+"/v1/seller/store/sales", map[string]any{"name": "Second", "percent": 10,
		"ends_at": time.Now().Add(24 * time.Hour)}); code != 409 {
		t.Fatalf("overlapping sale: %d", code)
	}

	// Buyers see and pay the sale price.
	_, body = do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+listingID, nil)
	l := body["listing"].(map[string]any)
	if l["sale_percent"] != float64(20) || l["sale_price_cents"] != float64(800) || l["price_cents"] != float64(1000) {
		t.Fatalf("listing on sale: %v", l)
	}
	b := buyer(t, e, "shop_fan")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+listingID, map[string]int{"quantity": 1})
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/cart", nil)
	if body["total_cents"] != float64(800) {
		t.Fatalf("cart total with sale: %v", body)
	}
	_, body = do(t, client(t), "GET", e.ts.URL+"/v1/listings?seller=card_shop&store_category="+itoaT(pokemon)+"&on_sale=1", nil)
	if body["total"] != float64(1) {
		t.Fatalf("store category search: %v", body)
	}

	// Follow, then new listings and an update reach the follower.
	if code, body := do(t, b.c, "POST", e.ts.URL+"/v1/stores/card-shop/follow", nil); code != 200 || body["following"] != true || body["followers"] != float64(1) {
		t.Fatalf("follow: %d %v", code, body)
	}
	if code, _ := do(t, c, "POST", e.ts.URL+"/v1/stores/card-shop/follow", nil); code != 409 {
		t.Fatalf("follow own store: %d", code)
	}
	do(t, c, "PUT", e.ts.URL+"/v1/seller/listings/external/SHOP-2", rawCard("SHOP-2", 1500))
	svc := NewStores(e.db, nil)
	if n, err := svc.NotifyNewListings(t.Context()); err != nil || n != 1 {
		t.Fatalf("new listing notices: %d %v", n, err)
	}
	if n, _ := svc.NotifyNewListings(t.Context()); n != 0 {
		t.Fatalf("notified twice: %d", n)
	}
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/store/updates", map[string]string{"subject": "New stock", "body": "Fresh vintage cards this week."}); code != 201 || body["recipients"] != float64(1) {
		t.Fatalf("update: %d %v", code, body)
	}
	if code, _ := do(t, c, "POST", e.ts.URL+"/v1/seller/store/updates", map[string]string{"subject": "Again", "body": "Another update too soon."}); code != 409 {
		t.Fatalf("second update: %d", code)
	}
	var notices int
	if err := e.db.QueryRow(`SELECT count(*) FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE u.username = 'shop_fan' AND n.kind IN ('store_new_listings', 'store_update')`).Scan(&notices); err != nil || notices != 2 {
		t.Fatalf("follower notices: %d %v", notices, err)
	}

	// Ending the sale restores the price.
	if code, _ := do(t, c, "DELETE", e.ts.URL+"/v1/seller/store/sales/"+itoaT(saleID), nil); code != 200 {
		t.Fatalf("end sale: %d", code)
	}
	_, body = do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+listingID, nil)
	if l := body["listing"].(map[string]any); l["sale_percent"] != nil {
		t.Fatalf("sale still on: %v", l)
	}
}

func itoaT(n int) string { return strconv.Itoa(n) }
