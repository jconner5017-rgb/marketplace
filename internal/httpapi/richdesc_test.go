package httpapi

import (
	"strings"
	"testing"
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
