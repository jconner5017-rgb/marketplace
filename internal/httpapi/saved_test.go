package httpapi

import (
	"context"
	"strings"
	"testing"
)

func TestSavedSearchesAndAlerts(t *testing.T) {
	e := setupEnv(t)
	seller, _ := newSeller(t, e, "savedseller", true)
	buyer := client(t)
	do(t, buyer, "POST", e.ts.URL+"/v1/auth/signup", map[string]string{"email": "savedbuyer@example.com", "username": "savedbuyer", "password": "longenough1"})

	code, body := do(t, buyer, "POST", e.ts.URL+"/v1/saved-searches", map[string]any{"query": "?q=zzcharizard&grader=&offset=48&sort=price_asc"})
	if code != 201 {
		t.Fatalf("save: %d %v", code, body)
	}
	ss := body["search"].(map[string]any)
	if ss["query"] != "q=zzcharizard" || ss["name"] != "zzcharizard" {
		t.Fatalf("normalized: %v", ss)
	}
	id := ss["id"].(string)
	if code, _ := do(t, buyer, "POST", e.ts.URL+"/v1/saved-searches", map[string]any{"query": "q=zzcharizard&page=2"}); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}
	if code, _ := do(t, buyer, "POST", e.ts.URL+"/v1/saved-searches", map[string]any{"query": "sort=newest"}); code != 422 {
		t.Fatalf("empty: %d", code)
	}
	if code, _ := do(t, buyer, "POST", e.ts.URL+"/v1/saved-searches", map[string]any{"query": "min_price=abc"}); code != 422 {
		t.Fatalf("bad filter: %d", code)
	}

	// A matching card is listed; the alert job tells the buyer once.
	card := rawCard("ZZ1", 900)
	card["title"] = "zzcharizard holo"
	if code, body := do(t, seller, "POST", e.ts.URL+"/v1/seller/listings", card); code != 201 {
		t.Fatalf("list: %d %v", code, body)
	}
	_, body = do(t, buyer, "GET", e.ts.URL+"/v1/saved-searches", nil)
	if l := body["searches"].([]any); len(l) != 1 || l[0].(map[string]any)["new_count"].(float64) != 1 {
		t.Fatalf("list: %v", body)
	}
	n, err := e.srv.shopping.RunSavedAlerts(context.Background())
	if err != nil || n < 1 {
		t.Fatalf("alerts: %d %v", n, err)
	}
	var title, link string
	if err := e.db.QueryRow(`SELECT title, link FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE u.username = 'savedbuyer' AND kind = 'saved_search' ORDER BY n.created_at DESC LIMIT 1`).Scan(&title, &link); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(title, "1 new card for") || !strings.Contains(link, "q=zzcharizard") {
		t.Fatalf("notification: %q %q", title, link)
	}
	// Not again within six hours.
	if n, _ := e.srv.shopping.RunSavedAlerts(context.Background()); n != 0 {
		var c int
		e.db.QueryRow(`SELECT count(*) FROM notifications n JOIN users u ON u.id = n.user_id WHERE u.username = 'savedbuyer' AND kind = 'saved_search'`).Scan(&c)
		if c != 1 {
			t.Fatalf("second alert sent")
		}
	}

	// Watch IDs for hearts.
	_, lb := do(t, buyer, "GET", e.ts.URL+"/v1/listings?q=zzcharizard", nil)
	lid := lb["listings"].([]any)[0].(map[string]any)["id"].(string)
	do(t, buyer, "PUT", e.ts.URL+"/v1/watchlist/"+lid, nil)
	_, wb := do(t, buyer, "GET", e.ts.URL+"/v1/watchlist/ids", nil)
	if ids := wb["ids"].([]any); len(ids) != 1 || ids[0] != lid {
		t.Fatalf("watch ids: %v", wb)
	}

	// Turn off, then delete; someone else can't touch it.
	if code, _ := do(t, buyer, "PATCH", e.ts.URL+"/v1/saved-searches/"+id, map[string]any{"notify": false}); code != 204 {
		t.Fatalf("notify off: %d", code)
	}
	if code, _ := do(t, seller, "DELETE", e.ts.URL+"/v1/saved-searches/"+id, nil); code != 404 {
		t.Fatalf("other user delete: %d", code)
	}
	if code, _ := do(t, buyer, "DELETE", e.ts.URL+"/v1/saved-searches/"+id, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
}
