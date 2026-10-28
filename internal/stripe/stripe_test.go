package stripe

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestVerifyWebhook(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1","type":"account.updated","account":"acct_1","data":{"object":{"id":"acct_1","charges_enabled":true}}}`)
	now := time.Unix(1_800_000_000, 0)

	ev, err := VerifyWebhook(payload, SignForTest(payload, secret, now), secret, 5*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID != "evt_1" || ev.Type != "account.updated" || ev.Account != "acct_1" {
		t.Fatalf("parsed wrong: %+v", ev)
	}

	bad := []struct {
		name, header, secret string
		at                   time.Time
	}{
		{"wrong secret", SignForTest(payload, "other", now), secret, now},
		{"too old", SignForTest(payload, secret, now.Add(-10*time.Minute)), secret, now},
		{"empty header", "", secret, now},
		{"garbage", "t=abc,v1=zz", secret, now},
	}
	for _, b := range bad {
		if _, err := VerifyWebhook(payload, b.header, b.secret, 5*time.Minute, b.at); !errors.Is(err, ErrBadSignature) {
			t.Errorf("%s: want ErrBadSignature, got %v", b.name, err)
		}
	}

	tampered := []byte(`{"id":"evt_1","type":"account.updated","account":"acct_EVIL"}`)
	if _, err := VerifyWebhook(tampered, SignForTest(payload, secret, now), secret, 5*time.Minute, now); !errors.Is(err, ErrBadSignature) {
		t.Errorf("tampered payload accepted")
	}
}

func TestClientRequests(t *testing.T) {
	var gotPath, gotAuth string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		user, _, _ := r.BasicAuth()
		gotAuth = user
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		switch r.URL.Path {
		case "/v1/accounts":
			w.Write([]byte(`{"id":"acct_123","charges_enabled":false,"details_submitted":false}`))
		case "/v1/account_links":
			w.Write([]byte(`{"url":"https://connect.stripe.com/setup/s/abc"}`))
		case "/v1/accounts/acct_123":
			w.Write([]byte(`{"id":"acct_123","charges_enabled":true,"details_submitted":true}`))
		default:
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"No such thing"}}`))
		}
	}))
	defer srv.Close()

	c := NewClient("sk_test_x")
	c.BaseURL = srv.URL
	ctx := context.Background()

	a, err := c.CreateStandardAccount(ctx, "s@example.com")
	if err != nil || a.ID != "acct_123" {
		t.Fatalf("create: %+v %v", a, err)
	}
	if gotPath != "POST /v1/accounts" || gotAuth != "sk_test_x" || gotForm.Get("type") != "standard" || gotForm.Get("country") != "US" {
		t.Fatalf("create request wrong: %s auth=%s form=%v", gotPath, gotAuth, gotForm)
	}

	link, err := c.CreateOnboardingLink(ctx, "acct_123", "https://x/refresh", "https://x/return")
	if err != nil || link != "https://connect.stripe.com/setup/s/abc" {
		t.Fatalf("link: %s %v", link, err)
	}
	if gotForm.Get("type") != "account_onboarding" || gotForm.Get("return_url") != "https://x/return" {
		t.Fatalf("link form wrong: %v", gotForm)
	}

	a, err = c.GetAccount(ctx, "acct_123")
	if err != nil || !a.ChargesEnabled {
		t.Fatalf("get: %+v %v", a, err)
	}

	_, err = c.GetAccount(ctx, "acct_missing")
	var se *Error
	if !errors.As(err, &se) || se.Status != 400 || se.Message != "No such thing" {
		t.Fatalf("want stripe error, got %v", err)
	}
}

func TestCheckoutSessionRequest(t *testing.T) {
	var got url.Values
	var acct, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acct, path = r.Header.Get("Stripe-Account"), r.URL.Path
		r.ParseForm()
		got = r.PostForm
		w.Write([]byte(`{"id":"cs_1","url":"https://checkout.stripe.com/c/pay/cs_1","status":"open","payment_status":"unpaid"}`))
	}))
	defer srv.Close()
	c := NewClient("sk_test_x")
	c.BaseURL = srv.URL
	s, err := c.CreateCheckoutSession(context.Background(), "acct_9", CheckoutParams{
		OrderID: "ord1", FeeCents: 150, SuccessURL: "https://x.test/done?s={CHECKOUT_SESSION_ID}", CancelURL: "https://x.test/c",
		Lines: []LineItem{{Name: "Charizard", UnitCents: 1200, Quantity: 2, ImageURL: "https://img.test/a.jpg"}},
	})
	if err != nil || s.ID != "cs_1" {
		t.Fatal(s, err)
	}
	if acct != "acct_9" || path != "/v1/checkout/sessions" {
		t.Fatalf("header/path: %q %q", acct, path)
	}
	want := map[string]string{
		"mode": "payment", "payment_intent_data[application_fee_amount]": "150",
		"line_items[0][price_data][unit_amount]": "1200", "line_items[0][quantity]": "2",
		"line_items[0][price_data][currency]":               "usd",
		"shipping_address_collection[allowed_countries][0]": "US",
		"success_url": "https://x.test/done?s={CHECKOUT_SESSION_ID}", "metadata[order_id]": "ord1",
	}
	for k, v := range want {
		if got.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, got.Get(k), v)
		}
	}
}
