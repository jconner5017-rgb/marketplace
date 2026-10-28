package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/demo"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

func TestDemoSeedFacetsAndImages(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	svc := &listings.Service{DB: e.db, Sellers: &sellers.Service{DB: e.db}}

	n, err := demo.Seed(ctx, e.db, svc, "https://images.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if n < 40 {
		t.Fatalf("seeded %d, want 40+", n)
	}
	// Running again updates instead of duplicating.
	if _, err := demo.Seed(ctx, e.db, svc, "https://images.example.test"); err != nil {
		t.Fatal(err)
	}
	anon := client(t)
	_, body := do(t, anon, "GET", e.ts.URL+"/v1/listings?limit=1", nil)
	if int(body["total"].(float64)) != n {
		t.Fatalf("total after reseed: %v, want %d", body["total"], n)
	}

	// Facets: picking a grader still shows the other graders' counts.
	code, body := do(t, anon, "GET", e.ts.URL+"/v1/listings/facets?category=tcg&grader=PSA", nil)
	if code != 200 {
		t.Fatalf("facets: %d %v", code, body)
	}
	graders := map[string]float64{}
	for _, f := range body["grader"].([]any) {
		m := f.(map[string]any)
		graders[m["value"].(string)] = m["count"].(float64)
	}
	if graders["RAW"] == 0 || graders["PSA"] == 0 {
		t.Fatalf("grader facets: %v", graders)
	}
	games := body["game"].([]any)
	for _, g := range games {
		if g.(map[string]any)["value"] == "Baseball" {
			t.Fatal("category=tcg facet includes Baseball")
		}
	}
	if len(body["attribute"].([]any)) == 0 {
		t.Fatal("no attribute facets")
	}

	// Card image renders and escapes text.
	res, err := http.Get(e.ts.URL + "/demo/card/X1.svg?t=%3Cscript%3E&s=Set&k=tcg")
	if err != nil {
		t.Fatal(err)
	}
	var svg bytes.Buffer
	svg.ReadFrom(res.Body)
	res.Body.Close()
	if res.Header.Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(svg.String(), "<svg") ||
		strings.Contains(svg.String(), "<script>") {
		t.Fatalf("svg: %s %s", res.Header.Get("Content-Type"), svg.String()[:80])
	}

	// Nobody can log in as the sample seller.
	code, _ = do(t, client(t), "POST", e.ts.URL+"/v1/auth/login", map[string]string{
		"email": demo.SellerEmail, "password": "anything-at-all"})
	if code != 401 {
		t.Fatalf("demo seller login: want 401, got %d", code)
	}

	if err := demo.Remove(ctx, e.db); err != nil {
		t.Fatal(err)
	}
	_, body = do(t, anon, "GET", e.ts.URL+"/v1/listings", nil)
	if body["total"].(float64) != 0 {
		t.Fatalf("after remove: %v", body["total"])
	}
}
