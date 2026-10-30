package httpapi

import (
	"testing"
)

func TestBellMarkOneReadAndOrderView(t *testing.T) {
	e := setupEnv(t)
	u := buyer(t, e, "bell_user")
	var uid string
	e.db.QueryRow(`SELECT id FROM users WHERE username = 'bell_user'`).Scan(&uid)
	e.db.Exec(`INSERT INTO notifications (user_id, kind, title, link) VALUES ($1, 'offer', 'A', '/offers'), ($1, 'order', 'B', '/purchases/00000000-0000-0000-0000-000000000001')`, uid)

	_, b := do(t, u.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if b["unread"].(float64) != 2 {
		t.Fatalf("unread: %v", b)
	}
	var first string
	for _, n := range b["notices"].([]any) {
		if m := n.(map[string]any); m["title"] == "A" {
			first = m["id"].(string)
		}
	}
	if code, _ := do(t, u.c, "POST", e.ts.URL+"/v1/notifications/"+first+"/read", map[string]any{}); code != 204 {
		t.Fatalf("read one: %d", code)
	}
	if code, _ := do(t, u.c, "POST", e.ts.URL+"/v1/notifications/zz/read", map[string]any{}); code != 404 {
		t.Fatalf("bad id: %d", code)
	}
	_, b = do(t, u.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if b["unread"].(float64) != 1 {
		t.Fatalf("after read one: %v", b)
	}
	// Someone else can't mark it.
	other := buyer(t, e, "bell_other")
	do(t, other.c, "POST", e.ts.URL+"/v1/notifications/"+first+"/read", map[string]any{})
}
