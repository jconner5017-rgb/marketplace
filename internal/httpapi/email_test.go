package httpapi

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/email"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []email.Message
}

func (f *fakeSender) Name() string { return "fake" }
func (f *fakeSender) Send(ctx context.Context, from string, m email.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

// last returns the newest message to an address and the link in it.
func (f *fakeSender) last(t *testing.T, to, path string) (email.Message, string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.sent) - 1; i >= 0; i-- {
		if f.sent[i].To == to {
			m := regexp.MustCompile(`https://shop\.test` + regexp.QuoteMeta(path) + `\S+`).FindString(f.sent[i].Text)
			return f.sent[i], m
		}
	}
	t.Fatalf("no email to %s", to)
	return email.Message{}, ""
}

func TestEmailConfirmResetAndBlast(t *testing.T) {
	e := setupEnv(t)
	fs := &fakeSender{}
	mail := &email.Service{DB: e.db, Sender: fs, From: "S&S <hi@shop.test>", WebURL: "https://shop.test", Postal: "1 Test St, Testville MO", Secret: []byte("k")}
	ctx := context.Background()
	flush := func() {
		t.Helper()
		if _, err := mail.RunOutbox(ctx, 100); err != nil {
			t.Fatal(err)
		}
	}

	// A member from before confirmation existed isn't held up.
	sc, _ := newSeller(t, e, "old_seller", true)
	e.db.Exec(`UPDATE users SET verify_required = false WHERE username = 'old_seller'`)
	l := createListing(t, e, &httpClientT{sc}, rawCard("EM1", 900))["id"].(string)
	e.srv.mail = mail

	// A new member must confirm before buying.
	nb := buyer(t, e, "new_nina")
	_, me := do(t, nb.c, "GET", e.ts.URL+"/v1/me", nil)
	if me["must_confirm_email"] != true {
		t.Fatalf("me: %v", me)
	}
	do(t, nb.c, "PUT", e.ts.URL+"/v1/cart/"+l, map[string]int{"quantity": 1})
	if code, body := do(t, nb.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "old_seller"}); code != 403 ||
		body["error"].(map[string]any)["code"] != "email_unconfirmed" {
		t.Fatalf("unconfirmed checkout: %d %v", code, body)
	}
	// Signup queued a confirmation; asking again sends another.
	if code, _ := do(t, nb.c, "POST", e.ts.URL+"/v1/me/confirm-email", map[string]any{}); code != 200 {
		t.Fatalf("resend: %d", code)
	}
	flush()
	_, link := fs.last(t, "new_nina@example.com", "/verify-email?token=")
	tok := strings.TrimPrefix(link, "https://shop.test/verify-email?token=")
	if code, body := do(t, nb.c, "POST", e.ts.URL+"/v1/auth/confirm-email", map[string]string{"token": tok}); code != 200 {
		t.Fatalf("confirm: %d %v", code, body)
	}
	if code, _ := do(t, nb.c, "POST", e.ts.URL+"/v1/auth/confirm-email", map[string]string{"token": tok}); code != 410 {
		t.Fatal("token reused")
	}
	_, me = do(t, nb.c, "GET", e.ts.URL+"/v1/me", nil)
	if me["must_confirm_email"] != false {
		t.Fatalf("still unconfirmed: %v", me)
	}
	if code, body := do(t, nb.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "old_seller"}); code != 201 {
		t.Fatalf("confirmed checkout: %d %v", code, body)
	}

	// Password reset: unknown emails get the same answer; the link signs
	// the member out everywhere.
	if code, _ := do(t, nb.c, "POST", e.ts.URL+"/v1/auth/forgot", map[string]string{"email": "nobody@example.com"}); code != 200 {
		t.Fatal("forgot unknown")
	}
	do(t, client(t), "POST", e.ts.URL+"/v1/auth/forgot", map[string]string{"email": "NEW_nina@example.com"})
	flush()
	_, link = fs.last(t, "new_nina@example.com", "/reset-password?token=")
	rt := strings.TrimPrefix(link, "https://shop.test/reset-password?token=")
	if code, _ := do(t, client(t), "POST", e.ts.URL+"/v1/auth/reset", map[string]string{"token": rt, "password": "short"}); code != 422 {
		t.Fatal("short password accepted")
	}
	// The failed attempt rolled back, so the token still works.
	if code, body := do(t, client(t), "POST", e.ts.URL+"/v1/auth/reset", map[string]string{"token": rt, "password": "brand-new-pass"}); code != 200 {
		t.Fatalf("reset: %d %v", code, body)
	}
	if code, _ := do(t, nb.c, "GET", e.ts.URL+"/v1/me", nil); code != 401 {
		t.Fatalf("old session kept: %d", code)
	}
	if code, _ := do(t, client(t), "POST", e.ts.URL+"/v1/auth/login", map[string]string{"email": "new_nina@example.com", "password": "brand-new-pass"}); code != 200 {
		t.Fatalf("login with new password: %d", code)
	}

	// Blasts: staff only; reach confirmed members; one-click unsubscribe.
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/admin/email", map[string]any{"subject": "Hi", "body": "x", "audience": "all"}); code != 403 && code != 401 {
		t.Fatalf("member blasted: %d", code)
	}
	boss := staff(t, e, "mail_boss", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	e.db.Exec(`UPDATE users SET email_verified_at = now() WHERE username = 'mail_boss'`)
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/email", map[string]any{"subject": "Launch!", "body": "We're live.", "audience": "all", "test": true}); code != 200 {
		t.Fatalf("test blast: %d %v", code, body)
	}
	code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/email", map[string]any{"subject": "Launch!", "body": "We're live.", "audience": "all"})
	if code != 201 {
		t.Fatalf("blast: %d %v", code, body)
	}
	// old_seller (legacy), new_nina (confirmed) and mail_boss (confirmed).
	if n := body["recipients"].(float64); n != 3 {
		t.Fatalf("recipients %v", n)
	}
	flush()
	m, unsub := fs.last(t, "new_nina@example.com", "/unsubscribe?u=")
	if !m.Broadcast || !strings.Contains(m.Text, "1 Test St") || m.Headers["List-Unsubscribe-Post"] == "" {
		t.Fatalf("blast email: %+v", m)
	}
	u, _ := url.Parse(unsub)
	if code, _ := do(t, client(t), "POST", e.ts.URL+"/v1/email/unsubscribe?u="+u.Query().Get("u")+"&t=bad"+u.Query().Get("t")[3:], nil); code != 400 {
		t.Fatal("forged unsubscribe accepted")
	}
	if code, _ := do(t, client(t), "POST", e.ts.URL+"/v1/email/unsubscribe?"+u.RawQuery, nil); code != 200 {
		t.Fatal("unsubscribe")
	}
	if n, _ := mail.CountRecipients(ctx, "all"); n != 2 {
		t.Fatalf("after unsubscribe %d", n)
	}
	_, list := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/email", nil)
	if b := list["broadcasts"].([]any)[0].(map[string]any); b["sent"].(float64) != 3 {
		t.Fatalf("broadcast stats %v", b)
	}
}
