package httpapi

import (
	"net/http"
	"testing"
)

func TestShippingEstimate(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "est_shop", true)
	get := func(q string) map[string]any {
		t.Helper()
		code, body := do(t, sc, "GET", e.ts.URL+"/v1/seller/shipping/estimate?"+q, nil)
		if code != 200 {
			t.Fatalf("estimate %s: %d %v", q, code, body)
		}
		return body["estimate"].(map[string]any)
	}
	if est := get("preset=envelope&weight_oz=1&price_cents=500"); est["method"] != "stamp" || est["cost_cents"].(float64) != 131 {
		t.Fatalf("envelope: %v", est)
	}
	// Before a ship-from address is saved, the quote comes from the middle of the country.
	est := get("preset=bubble&weight_oz=1&price_cents=2500")
	if est["method"] != "label" || est["live"] != true || est["from_set"] != false || est["cost_cents"].(float64) <= est["label_cents"].(float64) {
		t.Fatalf("bubble: %v", est)
	}
	do(t, sc, "PUT", e.ts.URL+"/v1/seller/shipping/from", map[string]string{"name": "Sam", "line1": "1 Market St", "city": "San Francisco", "state": "CA", "zip": "94105"})
	if est := get("preset=box&weight_oz=4&graded=true&price_cents=80000"); est["from_set"] != true || est["signature"] != true || est["carrier"] != "USPS" {
		t.Fatalf("box: %v", est)
	}
	if code, _ := do(t, &http.Client{}, "GET", e.ts.URL+"/v1/seller/shipping/estimate?preset=bubble", nil); code != 401 {
		t.Fatalf("anonymous estimate: %d", code)
	}
}
