package stripe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A platform that may not create v1 accounts falls back to Accounts v2 for
// creating, onboarding and checking sellers.
func TestAccountsV2Fallback(t *testing.T) {
	var v2Bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/"):
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"Stripe no longer recommends Accounts v1 for new Connect integrations. Use Accounts v2."}}`))
		case r.URL.Path == "/v2/core/accounts" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer sk_live_x" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Stripe-Version") == "" {
				t.Errorf("v2 headers: %v", r.Header)
			}
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			v2Bodies = append(v2Bodies, b)
			w.Write([]byte(`{"id":"acct_v2abc","configuration":{"merchant":{"capabilities":{"card_payments":{"status":"pending"}}}},"requirements":{"entries":[{"description":"identity","minimum_deadline":{"status":"currently_due"}}]}}`))
		case r.URL.Path == "/v2/core/account_links":
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"account_onboarding"`) || !strings.Contains(string(b), `"merchant"`) {
				t.Errorf("link body %s", b)
			}
			w.Write([]byte(`{"url":"https://connect.stripe.com/setup/v2/xyz"}`))
		case r.URL.Path == "/v2/core/accounts/acct_v2abc":
			if !strings.Contains(r.URL.RawQuery, "include=configuration.merchant") {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"id":"acct_v2abc","configuration":{"merchant":{"capabilities":{"card_payments":{"status":"active"}}}},"requirements":{"entries":[]}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewClient("sk_live_x")
	c.BaseURL = srv.URL
	ctx := context.Background()

	a, err := c.CreateStandardAccount(ctx, "seller@example.com")
	if err != nil || a.ID != "acct_v2abc" || a.ChargesEnabled || a.DetailsSubmitted {
		t.Fatalf("create: %+v %v", a, err)
	}
	b := v2Bodies[0]
	if b["dashboard"] != "full" || b["contact_email"] != "seller@example.com" ||
		b["defaults"].(map[string]any)["responsibilities"].(map[string]any)["losses_collector"] != "stripe" {
		t.Fatalf("v2 body %v", b)
	}
	u, err := c.CreateOnboardingLink(ctx, "acct_v2abc", "https://s.test/r", "https://s.test/ok")
	if err != nil || u != "https://connect.stripe.com/setup/v2/xyz" {
		t.Fatalf("link: %s %v", u, err)
	}
	a, err = c.GetAccount(ctx, "acct_v2abc")
	if err != nil || !a.ChargesEnabled || !a.DetailsSubmitted {
		t.Fatalf("get: %+v %v", a, err)
	}
}
