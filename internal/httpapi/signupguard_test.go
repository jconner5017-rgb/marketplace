package httpapi

import (
	"testing"
	"time"
)

func TestDottedGmail(t *testing.T) {
	for email, want := range map[string]bool{
		"c.s.e.j.h.i.l.g@gmail.com":     true,
		"C.S.E.J.H.I.L.G@GMail.com":     true,
		"a.b.c.d.e@googlemail.com":      true,
		"j.o.h.n.smith+promo@gmail.com": true,
		"john.a.smith@gmail.com":        false,
		"mary.k.jo.ann.lee@gmail.com":   false, // 5 pieces, only 1 single letter
		"c.s.e.j.h.i.l.g@yahoo.com":     false, // other providers treat dots as real
		"cardguy@gmail.com":             false,
		"not-an-email":                  false,
	} {
		if got := dottedGmail(email); got != want {
			t.Errorf("dottedGmail(%q) = %v, want %v", email, got, want)
		}
	}
	for email, want := range map[string]string{
		"J.Conner+ebay@Gmail.com": "jconner",
		"jconner@googlemail.com":  "jconner",
		"j.conner@outlook.com":    "",
	} {
		if got := gmailInbox(email); got != want {
			t.Errorf("gmailInbox(%q) = %q, want %q", email, got, want)
		}
	}
}

func TestSignupToken(t *testing.T) {
	s := &Server{signupKey: []byte("test key")}
	now := time.Now()
	tok := s.signupToken(now.Add(-10 * time.Second))
	if age, ok := s.signupTokenAge(tok, now); !ok || age < 9*time.Second {
		t.Fatalf("age %v ok %v", age, ok)
	}
	other := &Server{signupKey: []byte("another key")}
	if _, ok := other.signupTokenAge(tok, now); ok {
		t.Fatal("a token signed with another key must fail")
	}
	if _, ok := s.signupTokenAge(tok[:len(tok)-2]+"AA", now); ok {
		t.Fatal("an edited token must fail")
	}
	if why := s.signupBlocked(signupRequest{FormToken: tok}); why != "" {
		t.Fatalf("a 10-second-old token should pass: %s", why)
	}
	if why := s.signupBlocked(signupRequest{FormToken: s.signupToken(now)}); why != "too_fast" {
		t.Fatalf("instant submit: %q", why)
	}
	if why := s.signupBlocked(signupRequest{FormToken: s.signupToken(now.Add(-4 * time.Hour))}); why != "stale_token" {
		t.Fatalf("old token: %q", why)
	}
	if why := s.signupBlocked(signupRequest{}); why != "no_token" {
		t.Fatalf("no token: %q", why)
	}
	if why := s.signupBlocked(signupRequest{FormToken: tok, Website: "http://spam"}); why != "honeypot" {
		t.Fatalf("honeypot: %q", why)
	}
	off := &Server{}
	if why := off.signupBlocked(signupRequest{}); why != "" {
		t.Fatalf("with no key the token check is off: %q", why)
	}
}

func TestSignupGuardOverHTTP(t *testing.T) {
	e := setupEnv(t)
	e.srv.signupKey = []byte("http test key")
	c := client(t)

	body := func(email, user, tok string) map[string]string {
		return map[string]string{"email": email, "username": user, "password": "long enough pw", "form_token": tok}
	}
	if code, out := do(t, c, "POST", e.ts.URL+"/v1/auth/signup", body("a@example.com", "aaa_user", "")); code != 400 || errCode(out) != "signup_check" {
		t.Fatalf("no token: %d %v", code, out)
	}
	_, check := do(t, c, "GET", e.ts.URL+"/v1/auth/signup-check", nil)
	fresh := check["token"].(string)
	if code, _ := do(t, c, "POST", e.ts.URL+"/v1/auth/signup", body("a@example.com", "aaa_user", fresh)); code != 400 {
		t.Fatalf("instant submit should be refused: %d", code)
	}
	aged := e.srv.signupToken(time.Now().Add(-30 * time.Second))
	if code, out := do(t, c, "POST", e.ts.URL+"/v1/auth/signup", body("c.s.e.j.h.i.l.g@gmail.com", "bot_user", aged)); code != 422 {
		t.Fatalf("dotted gmail: %d %v", code, out)
	}
	if code, out := do(t, c, "POST", e.ts.URL+"/v1/auth/signup", body("Card.Seller@gmail.com", "real_user", aged)); code != 201 {
		t.Fatalf("real sign-up: %d %v", code, out)
	}
	c2 := client(t)
	if code, out := do(t, c2, "POST", e.ts.URL+"/v1/auth/signup", body("cardseller+2@gmail.com", "same_inbox", aged)); code != 409 || errCode(out) != "email_taken" {
		t.Fatalf("same Gmail inbox: %d %v", code, out)
	}
	hp := body("bot2@example.com", "bot_two", aged)
	hp["website"] = "http://cheap-pills.example"
	if code, _ := do(t, c2, "POST", e.ts.URL+"/v1/auth/signup", hp); code != 400 {
		t.Fatalf("honeypot: %d", code)
	}
}
