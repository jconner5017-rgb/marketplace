package httpapi

// Integration tests. They need a real, disposable Postgres database:
//
//	TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/marketplace_test?sslmode=disable go test ./...
//
// Without TEST_DATABASE_URL these tests are skipped. They wipe the users
// and sessions tables, so never point them at a real database.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/checkout"
	"github.com/jconner5017-rgb/marketplace/internal/db"
	"github.com/jconner5017-rgb/marketplace/internal/fcm"
	"github.com/jconner5017-rgb/marketplace/internal/feedback"
	"github.com/jconner5017-rgb/marketplace/internal/fees"
	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/photos"
	"github.com/jconner5017-rgb/marketplace/internal/sales"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
	"github.com/jconner5017-rgb/marketplace/internal/shopping"
	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

// fakeStripe stands in for Stripe Connect in tests.
type fakeStripe struct {
	mu       sync.Mutex
	accounts map[string]*stripe.Account
	created  int
}

func newFakeStripe() *fakeStripe { return &fakeStripe{accounts: map[string]*stripe.Account{}} }

func (f *fakeStripe) CreateStandardAccount(ctx context.Context, email string) (stripe.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	a := &stripe.Account{ID: fmt.Sprintf("acct_test%d", f.created)}
	f.accounts[a.ID] = a
	return *a, nil
}

func (f *fakeStripe) CreateOnboardingLink(ctx context.Context, id, refreshURL, returnURL string) (string, error) {
	return "https://connect.stripe.com/setup/" + id + "?return=" + returnURL, nil
}

func (f *fakeStripe) GetAccount(ctx context.Context, id string) (stripe.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.accounts[id]
	if !ok {
		return stripe.Account{}, &stripe.Error{Status: 404, Message: "No such account"}
	}
	return *a, nil
}

// finish simulates the seller completing Stripe onboarding.
func (f *fakeStripe) finish(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[id].ChargesEnabled = true
	f.accounts[id].DetailsSubmitted = true
}

type env struct {
	ts     *httptest.Server
	auth   *auth.Service
	stripe *fakeStripe
	db     *sql.DB
	sales  *sales.Service
	pay    *stripe.FakePayments
	co     *checkout.Service
	srv    *Server
	fcm    *fcm.Service
	ship   *shipping.Fake
	plat   *stripe.FakePlatform
	fb     *feedback.Service
}

const testWebhookSecret = "whsec_test_secret"

func setupEnv(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	auth.PasswordIterations = 1000 // keep tests fast

	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`TRUNCATE users, stripe_events, announcements, admin_ip_rules, blocked_identifiers CASCADE`); err != nil {
		t.Fatal(err)
	}

	fs := newFakeStripe()
	authSvc := &auth.Service{DB: d, SessionTTL: time.Hour}
	sellerSvc := &sellers.Service{DB: d, Stripe: fs}
	salesSvc := &sales.Service{DB: d}
	if _, err := d.Exec(`DELETE FROM catalog_categories; DELETE FROM catalog_attributes;
		DELETE FROM fee_schedules WHERE id > 1; UPDATE fee_upgrades SET active = false`); err != nil {
		t.Fatal(err)
	}
	catalog := &listings.Catalog{DB: d}
	if err := catalog.Load(ctx); err != nil {
		t.Fatal(err)
	}
	pay := stripe.NewFakePayments()
	ship, plat := shipping.NewFake(), stripe.NewFakePlatform()
	co := &checkout.Service{DB: d, Sellers: sellerSvc, Fees: &fees.Service{DB: d}, Pay: pay, Enabled: true,
		WebURL: "https://shop.test", Ship: ship, Platform: plat}
	fcmSvc := &fcm.Service{DB: d, HTTP: http.DefaultClient, AllowPrivate: true}
	fb := &feedback.Service{DB: d, Rep: &sellers.Reputation{DB: d}}
	co.Rep = fb.Rep
	s := New(Options{
		DB: d, Auth: authSvc,
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		SessionTTL:    time.Hour,
		Sellers:       sellerSvc,
		Listings:      &listings.Service{DB: d, Sellers: sellerSvc},
		PublicURL:     "https://market.test",
		WebhookSecret: testWebhookSecret,
		Shopping:      &shopping.Service{DB: d, Listings: &listings.Service{DB: d, Sellers: sellerSvc}, Sellers: sellerSvc},
		Inbox:         &inbox.Service{DB: d, Listings: &listings.Service{DB: d, Sellers: sellerSvc}},
		Sales:         salesSvc,
		MFA:           &admin.MFA{DB: d, Key: []byte("0123456789abcdef0123456789abcdef")},
		Audit:         &admin.Audit{DB: d},
		Catalog:       catalog,
		Fees:          &fees.Service{DB: d},
		Checkout:      co,
		FCM:           fcmSvc,
		Feedback:      fb,
		Photos:        &photos.Service{DB: d, BaseURL: "https://api.market.test"},

		ShippingWebhookToken: "shiptok",
	})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &env{ts: ts, auth: authSvc, stripe: fs, db: d, sales: salesSvc, pay: pay, co: co, srv: s, fcm: fcmSvc, ship: ship, plat: plat, fb: fb}
}

func setup(t *testing.T) (*httptest.Server, *auth.Service) {
	e := setupEnv(t)
	return e.ts, e.auth
}

// client is a browser-like client that keeps cookies.
func client(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func do(t *testing.T, c *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rdr)
	if body != nil || method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func TestHealth(t *testing.T) {
	ts, _ := setup(t)
	code, body := do(t, client(t), "GET", ts.URL+"/healthz", nil)
	if code != 200 || body["status"] != "ok" {
		t.Fatalf("health: %d %v", code, body)
	}
}

func TestSignupLoginLogoutFlow(t *testing.T) {
	ts, _ := setup(t)
	c := client(t)

	// Not logged in yet.
	if code, _ := do(t, c, "GET", ts.URL+"/v1/me", nil); code != 401 {
		t.Fatalf("me before signup: want 401, got %d", code)
	}

	// Sign up logs you in.
	code, body := do(t, c, "POST", ts.URL+"/v1/auth/signup", map[string]string{
		"email": "seller@example.com", "username": "card_seller", "password": "pikachu-1999",
	})
	if code != 201 {
		t.Fatalf("signup: %d %v", code, body)
	}
	code, body = do(t, c, "GET", ts.URL+"/v1/me", nil)
	user, _ := body["user"].(map[string]any)
	if code != 200 || user["username"] != "card_seller" || user["role"] != "user" {
		t.Fatalf("me after signup: %d %v", code, body)
	}
	if _, leaked := user["password_hash"]; leaked {
		t.Fatal("password hash leaked in API response")
	}

	// Log out ends the session.
	if code, _ := do(t, c, "POST", ts.URL+"/v1/auth/logout", nil); code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code, _ := do(t, c, "GET", ts.URL+"/v1/me", nil); code != 401 {
		t.Fatalf("me after logout: want 401, got %d", code)
	}

	// Wrong password.
	code, body = do(t, c, "POST", ts.URL+"/v1/auth/login", map[string]string{
		"email": "seller@example.com", "password": "wrong-password",
	})
	if code != 401 || errCode(body) != "invalid_credentials" {
		t.Fatalf("bad login: %d %v", code, body)
	}

	// Unknown email gives the same answer (no account enumeration).
	code, body = do(t, c, "POST", ts.URL+"/v1/auth/login", map[string]string{
		"email": "nobody@example.com", "password": "pikachu-1999",
	})
	if code != 401 || errCode(body) != "invalid_credentials" {
		t.Fatalf("unknown email: %d %v", code, body)
	}

	// Correct login, email case-insensitive.
	code, body = do(t, c, "POST", ts.URL+"/v1/auth/login", map[string]string{
		"email": "SELLER@example.com", "password": "pikachu-1999",
	})
	if code != 200 {
		t.Fatalf("login: %d %v", code, body)
	}
	if code, _ := do(t, c, "GET", ts.URL+"/v1/me", nil); code != 200 {
		t.Fatalf("me after login: %d", code)
	}
}

func TestSignupValidationAndDuplicates(t *testing.T) {
	ts, _ := setup(t)
	c := client(t)

	code, body := do(t, c, "POST", ts.URL+"/v1/auth/signup", map[string]string{
		"email": "bad", "username": "x", "password": "short",
	})
	if code != 422 || errCode(body) != "invalid_input" {
		t.Fatalf("validation: %d %v", code, body)
	}
	fields, _ := body["error"].(map[string]any)["fields"].(map[string]any)
	for _, f := range []string{"email", "username", "password"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("missing field error for %s: %v", f, fields)
		}
	}

	ok := map[string]string{"email": "a@example.com", "username": "first_user", "password": "longenough1"}
	if code, body := do(t, c, "POST", ts.URL+"/v1/auth/signup", ok); code != 201 {
		t.Fatalf("first signup: %d %v", code, body)
	}

	dupEmail := map[string]string{"email": "A@Example.com", "username": "second_user", "password": "longenough1"}
	if code, body := do(t, client(t), "POST", ts.URL+"/v1/auth/signup", dupEmail); code != 409 || errCode(body) != "email_taken" {
		t.Fatalf("dup email: %d %v", code, body)
	}

	dupUser := map[string]string{"email": "b@example.com", "username": "FIRST_USER", "password": "longenough1"}
	if code, body := do(t, client(t), "POST", ts.URL+"/v1/auth/signup", dupUser); code != 409 || errCode(body) != "username_taken" {
		t.Fatalf("dup username: %d %v", code, body)
	}
}

func TestRejectsNonJSONWrites(t *testing.T) {
	ts, _ := setup(t)
	res, err := http.Post(ts.URL+"/v1/auth/login", "application/x-www-form-urlencoded",
		strings.NewReader("email=a&password=b"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 415 {
		t.Fatalf("form post: want 415, got %d", res.StatusCode)
	}

	req, _ := http.NewRequest("POST", ts.URL+"/v1/auth/login", strings.NewReader(`{"email":"a@b.co","password":"x","extra":1}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("unknown field: want 400, got %d", res.StatusCode)
	}
}

func TestAdminOnly(t *testing.T) {
	ts, svc := setup(t)
	c := client(t)
	do(t, c, "POST", ts.URL+"/v1/auth/signup", map[string]string{
		"email": "boss@example.com", "username": "boss", "password": "longenough1",
	})
	if code, _ := do(t, c, "GET", ts.URL+"/v1/admin/users", nil); code != 403 {
		t.Fatalf("regular user on admin route: want 403, got %d", code)
	}
	if err := svc.SetRole(context.Background(), "boss@example.com", "admin"); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, c, "GET", ts.URL+"/v1/admin/users", nil); code != 403 || errCode(body) != "mfa_required" {
		t.Fatalf("admin without authenticator check: %d %v", code, body)
	}
	enrollMFA(t, ts.URL, c)
	code, body := do(t, c, "GET", ts.URL+"/v1/admin/users", nil)
	if code != 200 {
		t.Fatalf("admin route: %d %v", code, body)
	}
	if users, _ := body["users"].([]any); len(users) != 1 {
		t.Fatalf("want 1 user, got %v", body["users"])
	}
	if code, _ := do(t, client(t), "GET", ts.URL+"/v1/admin/users", nil); code != 401 {
		t.Fatalf("anonymous on admin route: want 401, got %d", code)
	}
}

func TestBannedUserLosesAccess(t *testing.T) {
	ts, svc := setup(t)
	c := client(t)
	do(t, c, "POST", ts.URL+"/v1/auth/signup", map[string]string{
		"email": "scammer@example.com", "username": "scammer", "password": "longenough1",
	})
	if _, err := svc.DB.Exec(`UPDATE users SET status = 'banned' WHERE email = 'scammer@example.com'`); err != nil {
		t.Fatal(err)
	}
	if code, _ := do(t, c, "GET", ts.URL+"/v1/me", nil); code != 401 {
		t.Fatalf("banned user's existing session: want 401, got %d", code)
	}
	code, body := do(t, client(t), "POST", ts.URL+"/v1/auth/login", map[string]string{
		"email": "scammer@example.com", "password": "longenough1",
	})
	if code != 403 || errCode(body) != "account_disabled" {
		t.Fatalf("banned login: %d %v", code, body)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	ts, svc := setup(t)
	c := client(t)
	do(t, c, "POST", ts.URL+"/v1/auth/signup", map[string]string{
		"email": "old@example.com", "username": "old_session", "password": "longenough1",
	})
	if _, err := svc.DB.Exec(`UPDATE sessions SET expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if code, _ := do(t, c, "GET", ts.URL+"/v1/me", nil); code != 401 {
		t.Fatalf("expired session: want 401, got %d", code)
	}
	n, err := svc.PurgeExpiredSessions(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("purge: n=%d err=%v", n, err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts, _ := setup(t)
	c := client(t)
	var last int
	for i := 0; i < 11; i++ {
		last, _ = do(t, c, "POST", ts.URL+"/v1/auth/login", map[string]string{
			"email": "x@example.com", "password": "whatever-123",
		})
	}
	if last != 429 {
		t.Fatalf("11th attempt: want 429, got %d", last)
	}
}

func TestAdminEmailsPromotedOnSignup(t *testing.T) {
	ts, svc := setup(t)
	svc.AdminEmails = []string{"Owner@Example.com"}
	_, body := do(t, client(t), "POST", ts.URL+"/v1/auth/signup", map[string]string{
		"email": "owner@example.com", "username": "the_owner", "password": "longenough1"})
	if body["user"].(map[string]any)["role"] != "admin" {
		t.Fatalf("listed email not admin: %v", body)
	}
	_, body = do(t, client(t), "POST", ts.URL+"/v1/auth/signup", map[string]string{
		"email": "someone@example.com", "username": "someone", "password": "longenough1"})
	if body["user"].(map[string]any)["role"] != "user" {
		t.Fatalf("unlisted email became admin: %v", body)
	}
}

// enrollMFA sets up the staff authenticator for c's account and passes the check.
func enrollMFA(t *testing.T, base string, c *http.Client) string {
	t.Helper()
	code, body := do(t, c, "POST", base+"/v1/admin/mfa/setup", map[string]any{})
	if code != 200 {
		t.Fatalf("mfa setup: %d %v", code, body)
	}
	secret := body["secret"].(string)
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	code, body = do(t, c, "POST", base+"/v1/admin/mfa/confirm", map[string]string{"code": admin.TOTP(raw, time.Now().Unix()/30)})
	if code != 200 || len(body["backup_codes"].([]any)) != 10 {
		t.Fatalf("mfa confirm: %d %v", code, body)
	}
	return secret
}
