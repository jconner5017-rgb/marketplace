package httpapi

import (
	"net/http"
	"testing"
)

// The seller hub overview answers for a seller with one listing and no
// sales: 31 days of sales, five period totals and the listing counted.
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
	if len(sales["days"].([]any)) != 31 || len(sales["periods"].([]any)) != 5 /* today, 7d, 31d, 90d, year */ {
		t.Fatalf("sales: %v", sales)
	}
	if l := body["listings"].(map[string]any); l["active"] != float64(1) || l["fixed"] != float64(1) {
		t.Fatalf("listings: %v", l)
	}
	if o := body["orders"].(map[string]any); o["to_ship"] != float64(0) {
		t.Fatalf("orders: %v", o)
	}

	// Traffic (2026-11-20): a visitor sees the card in 4 searches and opens it
	// once; a crawler's searches don't count.
	browse := func(path, ua string) {
		req, _ := http.NewRequest("GET", e.ts.URL+path, nil)
		req.Header.Set("User-Agent", ua)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	const browser = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/130.0"
	for range 4 {
		browse("/v1/listings?seller=hub_seller&impr=1", browser)
	}
	browse("/v1/listings?seller=hub_seller&impr=1", "Mozilla/5.0 (compatible; Googlebot/2.1)")
	browse("/v1/listings?seller=hub_seller", browser) // not shown to anyone (sitemap, feeds)
	var id string
	e.db.QueryRow(`SELECT id FROM listings WHERE external_id = 'HUB-1'`).Scan(&id)
	browse("/v1/listings/"+id+"?view=1", browser)
	if err := e.srv.metrics.FlushImpressions(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, body = do(t, c, "GET", e.ts.URL+"/v1/seller/overview", nil)
	tr := body["traffic"].(map[string]any)
	if tr["impressions_31d"] != float64(4) || tr["views_31d"] != float64(1) || tr["ctr_31d"] != float64(25) {
		t.Fatalf("traffic: %v", tr)
	}
	// Selling costs and seller level are there (no sales yet).
	if c := body["costs_31d"].(map[string]any); c["total_sales_cents"] != float64(0) || c["net_cents"] != float64(0) {
		t.Fatalf("costs: %v", c)
	}
	if lv := body["level"].(map[string]any); lv["tier"] == nil || lv["tier_today"] == nil || lv["defect_percent"] != float64(0) {
		t.Fatalf("level: %v", lv)
	}
}
