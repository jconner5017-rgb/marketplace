package httpapi

import (
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

// Search speed-ups (2026-11-20): tiles get the photo's small copy, counts
// stop at a cap unless exact=1, random shelves draw from a pool.
func TestSearchSpeedups(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "speed_seller", true)
	for _, ext := range []string{"SP-1", "SP-2", "SP-3"} {
		if code, body := do(t, c, "PUT", e.ts.URL+"/v1/seller/listings/external/"+ext, rawCard(ext, 500)); code != 200 && code != 201 {
			t.Fatalf("list %s: %d %v", ext, code, body)
		}
	}
	// SP-1's photo has a small copy.
	var uid string
	e.db.QueryRow(`SELECT id FROM users WHERE username = 'speed_seller'`).Scan(&uid)
	if _, err := e.db.Exec(`INSERT INTO photos (owner_id, store, object_key, url, content_type, width, height, size_bytes, thumb_url)
		VALUES ($1, 's3', 'p/x/SP-1.jpg', 'https://img.example.com/SP-1.jpg', 'image/jpeg', 1, 1, 1, 'https://img.example.com/t/SP-1.jpg')`, uid); err != nil {
		t.Fatal(err)
	}
	_, body := do(t, c, "GET", e.ts.URL+"/v1/listings?seller=speed_seller", nil)
	thumbs := 0
	for _, x := range body["listings"].([]any) {
		l := x.(map[string]any)
		if l["thumb_url"] == "https://img.example.com/t/SP-1.jpg" {
			thumbs++
		} else if l["thumb_url"] != nil {
			t.Fatalf("thumb on %v: %v", l["title"], l["thumb_url"])
		}
	}
	if thumbs != 1 || body["total"] != float64(3) || body["total_capped"] != nil {
		t.Fatalf("thumbs %d, body %v", thumbs, body["total"])
	}

	// Counting stops at the cap; exact=1 (sitemap, feeds) counts them all.
	old := listings.TotalCap
	listings.TotalCap = 2
	defer func() { listings.TotalCap = old }()
	_, body = do(t, c, "GET", e.ts.URL+"/v1/listings?seller=speed_seller", nil)
	if body["total"] != float64(2) || body["total_capped"] != true || len(body["listings"].([]any)) != 3 {
		t.Fatalf("capped: %v %v", body["total"], body["total_capped"])
	}
	_, body = do(t, c, "GET", e.ts.URL+"/v1/listings?seller=speed_seller&exact=1", nil)
	if body["total"] != float64(3) || body["total_capped"] != nil {
		t.Fatalf("exact: %v", body["total"])
	}
	listings.TotalCap = old

	// Random shelves: the asked-for number, drawn from the matching cards.
	seen := map[string]bool{}
	for range 20 {
		_, body = do(t, c, "GET", e.ts.URL+"/v1/listings?seller=speed_seller&sort=random&limit=2", nil)
		ls := body["listings"].([]any)
		if len(ls) != 2 || body["total"] != float64(3) {
			t.Fatalf("random: %v", body)
		}
		for _, x := range ls {
			seen[x.(map[string]any)["title"].(string)] = true
		}
	}
	if len(seen) != 3 {
		t.Fatalf("random picks only ever showed %v", seen)
	}
}
