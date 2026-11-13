package httpapi

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFormattedDescription(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "html_shop", true)
	card := rawCard("HT1", 900)
	card["description_html"] = `<div style="color:red"><h1>My Shop</h1><script>alert(1)</script>` +
		`<table><tr><td><img src="x" onerror="y">Front</td></tr><tr><td>Sport</td><td>Football</td></tr></table>` +
		`<p onclick="z">Thanks <a href="javascript:x">here</a></p></div>`
	l := createListing(t, e, &httpClientT{sc}, card)
	id := l["id"].(string)

	_, body := do(t, sc, "GET", e.ts.URL+"/v1/listings/"+id, nil)
	got := body["listing"].(map[string]any)
	h, _ := got["description_html"].(string)
	for _, bad := range []string{"script", "onerror", "onclick", "style", "javascript", "<img"} {
		if strings.Contains(h, bad) {
			t.Fatalf("kept %q: %s", bad, h)
		}
	}
	if !strings.Contains(h, "<h2>My Shop</h2>") || !strings.Contains(h, "<td>Sport</td><td>Football</td>") {
		t.Fatalf("html: %s", h)
	}
	if d := got["description"].(string); !strings.Contains(d, "My Shop") || strings.Contains(d, "<") {
		t.Fatalf("plain description: %q", d)
	}

	// Search results leave the formatted copy out.
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/listings?q=HT1", nil)
	for _, x := range body["listings"].([]any) {
		if _, ok := x.(map[string]any)["description_html"]; ok {
			t.Fatal("search returned description_html")
		}
	}

	// A plain-text edit replaces the formatted description (FireCardManager API).
	code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/listings/HT1", map[string]any{"description": "Just text"})
	if code != 200 {
		t.Fatalf("patch: %d %v", code, body)
	}
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/listings/"+id, nil)
	got = body["listing"].(map[string]any)
	if _, ok := got["description_html"]; ok || got["description"] != "Just text" {
		t.Fatalf("after plain edit: %v", got)
	}
}

func TestListingSync(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "sync_shop", true)
	a := createListing(t, e, &httpClientT{sc}, rawCard("SY1", 900))["id"].(string)
	createListing(t, e, &httpClientT{sc}, rawCard("SY2", 900))
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/seller/listings/"+a, nil); code != 204 && code != 200 {
		t.Fatalf("remove: %d", code)
	}
	code, body := do(t, sc, "GET", e.ts.URL+"/v1/seller/listings/sync", nil)
	if code != 200 {
		t.Fatalf("sync: %d %v", code, body)
	}
	got := map[string]string{}
	for _, x := range body["listings"].([]any) {
		m := x.(map[string]any)
		got[m["external_id"].(string)] = m["status"].(string)
		if m["price_cents"].(float64) != 900 {
			t.Fatalf("price: %v", m)
		}
	}
	if got["SY1"] != "removed" || got["SY2"] != "active" {
		t.Fatalf("statuses: %v", got)
	}
	// Only what changed since a time.
	st, _ := time.Parse(time.RFC3339Nano, body["server_time"].(string))
	code, body = do(t, sc, "GET", e.ts.URL+"/v1/seller/listings/sync?since="+url.QueryEscape(st.Format(time.RFC3339Nano)), nil)
	if code != 200 || len(body["listings"].([]any)) != 0 {
		t.Fatalf("since (nothing changed): %d %v", code, body)
	}
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/seller/listings/"+createListing(t, e, &httpClientT{sc}, rawCard("SY3", 900))["id"].(string), nil); code != 204 && code != 200 {
		t.Fatalf("remove SY3: %d", code)
	}
	code, body = do(t, sc, "GET", e.ts.URL+"/v1/seller/listings/sync?since="+url.QueryEscape(st.Format(time.RFC3339Nano)), nil)
	if code != 200 || len(body["listings"].([]any)) != 1 || body["listings"].([]any)[0].(map[string]any)["status"] != "removed" {
		t.Fatalf("since (one removed): %v", body)
	}
	if code, _ := do(t, sc, "GET", e.ts.URL+"/v1/seller/listings/sync?since=yesterday", nil); code != 400 {
		t.Fatalf("bad since: %d", code)
	}
}

func TestReturnsPolicy(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "ret_shop", true)
	id := createListing(t, e, &httpClientT{sc}, rawCard("RT1", 900))["id"].(string)
	policyOf := func() string {
		_, body := do(t, sc, "GET", e.ts.URL+"/v1/listings/"+id, nil)
		return body["seller"].(map[string]any)["returns_policy"].(string)
	}
	if p := policyOf(); p != "damaged_only" {
		t.Fatalf("default: %q", p)
	}
	if code, _ := do(t, sc, "PUT", e.ts.URL+"/v1/seller/returns", map[string]string{"returns_policy": "60_days"}); code != 422 {
		t.Fatalf("bad policy: %d", code)
	}
	if code, body := do(t, sc, "PUT", e.ts.URL+"/v1/seller/returns", map[string]string{"returns_policy": "30_days"}); code != 200 {
		t.Fatalf("set: %d %v", code, body)
	}
	if p := policyOf(); p != "30_days" {
		t.Fatalf("after set: %q", p)
	}
	_, body := do(t, sc, "GET", e.ts.URL+"/v1/seller/shipping", nil)
	if body["returns_policy"] != "30_days" {
		t.Fatalf("settings: %v", body)
	}

	// Per listing (seller hub bulk edit): overrides the seller default and
	// survives an edit that doesn't mention it.
	do(t, sc, "PUT", e.ts.URL+"/v1/seller/returns", map[string]string{"returns_policy": "damaged_only"})
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+id, map[string]any{"returns_policy": "weekly"}); code != 422 {
		t.Fatalf("bad listing policy: %d %v", code, body)
	}
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+id, map[string]any{"returns_policy": "30_days"}); code != 200 {
		t.Fatalf("listing policy: %d %v", code, body)
	}
	if p := policyOf(); p != "30_days" {
		t.Fatalf("listing override: %q", p)
	}
	if code, _ := do(t, sc, "PATCH", e.ts.URL+"/v1/listings/RT1", map[string]any{"price_cents": 950}); code != 200 {
		t.Fatalf("api patch: %d", code)
	}
	if p := policyOf(); p != "30_days" {
		t.Fatalf("after unrelated edit: %q", p)
	}
}
