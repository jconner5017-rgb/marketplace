package httpapi

import "testing"

// The seller hub overview answers for a seller with one listing and no
// sales: 31 days of sales, four period totals and the listing counted.
func TestSellerOverview(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "hub_seller", true)
	if code, body := do(t, c, "PUT", e.ts.URL+"/v1/seller/listings/external/HUB-1", rawCard("HUB-1", 500)); code != 200 && code != 201 {
		t.Fatalf("list: %d %v", code, body)
	}
	code, body := do(t, c, "GET", e.ts.URL+"/v1/seller/overview", nil)
	if code != 200 {
		t.Fatalf("overview: %d %v", code, body)
	}
	sales := body["sales"].(map[string]any)
	if len(sales["days"].([]any)) != 31 || len(sales["periods"].([]any)) != 4 {
		t.Fatalf("sales: %v", sales)
	}
	if l := body["listings"].(map[string]any); l["active"] != float64(1) || l["fixed"] != float64(1) {
		t.Fatalf("listings: %v", l)
	}
	if o := body["orders"].(map[string]any); o["to_ship"] != float64(0) {
		t.Fatalf("orders: %v", o)
	}
}
