package httpapi

import (
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

// Staff add, publish, edit and delete posts.
func TestBlog(t *testing.T) {
	e := setupEnv(t)
	anon := client(t)
	// (The test setup truncates users, which empties blog_posts too, so the
	// four seeded guides are checked against a fresh database separately.)
	var code int
	var body map[string]any

	// Members can't write posts.
	m := buyer(t, e, "blog_reader")
	if code, _ := do(t, m.c, "POST", e.ts.URL+"/v1/admin/blog", map[string]any{"title": "Nope nope"}); code != 401 && code != 403 {
		t.Fatalf("member create: %d", code)
	}

	boss := staff(t, e, "blog_boss", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	draft := map[string]any{"title": "Grading Pokémon Cards: A Starter Guide", "description": "How to grade.",
		"body": []map[string]any{{"h": "Why grade"}, {"p": "Fees are {fee.flat}."}, {"links": []map[string]string{{"label": "Sell", "href": "/sell"}}}}}
	code, body = do(t, boss.c, "POST", e.ts.URL+"/v1/admin/blog", draft)
	p, _ := body["post"].(map[string]any)
	if code != 201 || p["slug"] != "grading-pok-mon-cards-a-starter-guide" || p["status"] != "draft" || p["published_at"] != nil {
		t.Fatalf("create draft: %d %v", code, body)
	}
	id := itoaT(int(p["id"].(float64)))
	if code, _ := do(t, anon, "GET", e.ts.URL+"/v1/blog/grading-pok-mon-cards-a-starter-guide", nil); code != 404 {
		t.Fatalf("draft is public: %d", code)
	}
	bad := map[string]any{"title": "Bad link post", "body": []map[string]any{{"links": []map[string]string{{"label": "x", "href": "javascript:alert(1)"}}}}}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/blog", bad); code != 422 {
		t.Fatalf("bad link: %d", code)
	}
	draft["slug"], draft["status"] = "grading-pokemon-cards", "published"
	code, body = do(t, boss.c, "PATCH", e.ts.URL+"/v1/admin/blog/"+id, draft)
	if code != 200 || body["post"].(map[string]any)["published_at"] == nil {
		t.Fatalf("publish: %d %v", code, body)
	}
	if code, _ := do(t, anon, "GET", e.ts.URL+"/v1/blog/grading-pokemon-cards", nil); code != 200 {
		t.Fatalf("published post: %d", code)
	}
	dupe := map[string]any{"title": "Another post here", "slug": "grading-pokemon-cards"}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/blog", dupe); code != 422 {
		t.Fatalf("duplicate slug: %d", code)
	}
	if code, _ := do(t, boss.c, "DELETE", e.ts.URL+"/v1/admin/blog/"+id, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := do(t, anon, "GET", e.ts.URL+"/v1/blog/grading-pokemon-cards", nil); code != 404 {
		t.Fatalf("deleted post still public: %d", code)
	}
}
