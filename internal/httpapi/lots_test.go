package httpapi

import (
	"net/http"
	"net/url"
	"testing"
)

func TestLotListingsAndCustomParcel(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "lot_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true`)

	// A lot: card number dropped, ships in a box, weight estimated from the count.
	lot := rawCard("LOT1", 2500)
	lot["title"], lot["lot_count"], lot["card_number"] = "25-card 2024 Prizm base lot", 25, "12"
	delete(lot, "package_preset")
	delete(lot, "weight_oz")
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", lot)
	if code != 201 {
		t.Fatalf("create lot: %d %v", code, body)
	}
	l := body["listing"].(map[string]any)
	if l["lot_count"].(float64) != 25 || l["package_preset"] != "box" || l["card_number"] != nil || l["weight_oz"].(float64) != 2.5 {
		t.Fatalf("lot fields: %v", l)
	}
	lotID := l["id"].(string)

	// Rules: 2 to 5000 cards, never an envelope.
	bad := rawCard("LOT2", 500)
	bad["lot_count"] = 1
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", bad); code != 422 {
		t.Fatalf("lot of 1 accepted: %d", code)
	}
	bad["lot_count"], bad["package_preset"] = 5, "envelope"
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", bad); code != 201 || body["listing"].(map[string]any)["package_preset"] != "box" {
		t.Fatalf("envelope lot should become a box: %d %v", code, body)
	}

	single := rawCard("ONE", 900)
	single["package_preset"] = "bubble"
	singleID := createListing(t, e, &httpClientT{sc}, single)["id"].(string)
	if l := createdListing(t, e, sc, singleID); l["lot_count"] != nil {
		t.Fatalf("single card has a lot count: %v", l)
	}

	// Search filter.
	anon := client(t)
	count := func(v string) int {
		code, body := do(t, anon, "GET", e.ts.URL+"/v1/listings?"+url.Values{"lots": {v}}.Encode(), nil)
		if code != 200 {
			t.Fatalf("search: %d %v", code, body)
		}
		return int(body["total"].(float64))
	}
	if count("only") != 2 || count("none") != 1 || count("") != 3 {
		t.Fatalf("lot filter: only=%d none=%d all=%d", count("only"), count("none"), count(""))
	}

	// Turning a lot back into a single card.
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+lotID, map[string]any{"lot_count": 0}); code != 200 ||
		body["listing"].(map[string]any)["lot_count"] != nil {
		t.Fatalf("clear lot: %d %v", code, body)
	}
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+lotID, map[string]any{"lot_count": 30}); code != 200 ||
		body["listing"].(map[string]any)["lot_count"].(float64) != 30 {
		t.Fatalf("set lot: %d %v", code, body)
	}

	// Buy it, then price a label in the seller's own box.
	b := buyer(t, e, "lena")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+lotID, map[string]int{"quantity": 1})
	_, body = do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "lot_shop"})
	order := body["order_id"].(string)
	e.pay.Complete(sessionOf(t, e, order))
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/confirm", map[string]any{})
	do(t, sc, "PUT", e.ts.URL+"/v1/seller/shipping/from", map[string]string{"name": "Sam", "line1": "1 Market St", "city": "San Francisco", "state": "CA", "zip": "94105"})

	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", map[string]any{})
	if code != 200 {
		t.Fatalf("default rates: %d %v", code, body)
	}
	p := body["parcel"].(map[string]any)
	if p["length"].(float64) != 8 || p["weight_oz"].(float64) <= 0 {
		t.Fatalf("suggested parcel: %v", p)
	}
	custom := map[string]any{"parcel": map[string]any{"length": 12, "width": 9, "height": 6, "weight_oz": 40}}
	code, body = do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", custom)
	if code != 200 || body["parcel"].(map[string]any)["height"].(float64) != 6 || len(body["rates"].([]any)) == 0 {
		t.Fatalf("custom rates: %d %v", code, body)
	}
	huge := map[string]any{"parcel": map[string]any{"length": 200, "width": 9, "height": 6, "weight_oz": 40}}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", huge); code != 422 {
		t.Fatalf("oversize box accepted: %d", code)
	}
	heavy := map[string]any{"parcel": map[string]any{"length": 12, "width": 9, "height": 6, "weight_oz": 2000}}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/orders/"+order+"/label/rates", heavy); code != 422 {
		t.Fatalf("overweight box accepted: %d", code)
	}
}

func createdListing(t *testing.T, e *env, c *http.Client, id string) map[string]any {
	t.Helper()
	code, body := do(t, c, "GET", e.ts.URL+"/v1/seller/listings/"+id, nil)
	if code != 200 {
		t.Fatalf("get listing: %d %v", code, body)
	}
	return body["listing"].(map[string]any)
}
