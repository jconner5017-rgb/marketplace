package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

func TestReputationJob(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	saved := sellers.TierRules
	sellers.TierRules = []struct {
		Tier       int
		Sales      int
		MinPercent float64
		MinAge     time.Duration
	}{{2, 3, 97, 24 * time.Hour}}
	t.Cleanup(func() { sellers.TierRules = saved })

	sc, _ := newSeller(t, e, "rep_shop", true)
	cheap := createListing(t, e, &httpClientT{sc}, rawCard("RP1", 1500))["id"].(string)
	e.db.Exec(`UPDATE users SET created_at = now() - interval '10 days' WHERE username = 'rep_shop'`)
	rep := &sellers.Reputation{DB: e.db}
	sid := memberID(t, e, "rep_shop")
	tier := func() int {
		var n int
		e.db.QueryRow(`SELECT tier FROM seller_profiles WHERE user_id = $1`, sid).Scan(&n)
		return n
	}
	var orders []string
	for _, b := range []string{"rb1", "rb2", "rb3", "rb4"} {
		buyer(t, e, b)
		id, _ := seedOrder(t, e, b, "rep_shop", cheap, 1500, false)
		orders = append(orders, id)
	}
	// Paid but not shipped: nothing qualifies yet.
	if n, err := rep.RunNightly(ctx); err != nil || n != 0 || tier() != 1 {
		t.Fatalf("before shipping: %d %v tier %d", n, err, tier())
	}
	e.db.Exec(`UPDATE orders SET status = 'shipped' WHERE seller_id = $1`, sid)
	e.db.Exec(`DELETE FROM login_events`) // every test account signed up from 127.0.0.1
	// One buyer shares a sign-in address with the seller, so only 3 count... which is enough.
	e.db.Exec(`INSERT INTO login_events (user_id, email, ip, success) VALUES
		((SELECT id FROM users WHERE username = 'rb4'), 'rb4@example.com', '198.51.100.9', true),
		($1, 'rep_shop@example.com', '198.51.100.9', true)`, sid)
	st, _ := sellers.SellerStats(ctx, e.db, sid)
	if st.QualifiedSales != 3 {
		t.Fatalf("qualified sales: %+v", st)
	}
	e.db.Exec(`UPDATE seller_profiles SET tier_reviewed_at = NULL`)
	if n, _ := rep.RunNightly(ctx); n != 1 || tier() != 2 {
		t.Fatalf("promotion: %d tier %d", n, tier())
	}
	pricey := createListing(t, e, &httpClientT{sc}, rawCard("RP2", 9000))["id"].(string)

	// Negative feedback drops the seller below 97%: demoted, pricey card hidden.
	e.db.Exec(`INSERT INTO feedback (order_id, from_user, to_user, role, rating, comment, visible_at)
		SELECT id, buyer_id, seller_id, 'buyer_to_seller', -1, 'Never arrived', now() FROM orders WHERE id = $1`, orders[0])
	if from, to, err := rep.Review(ctx, sid); err != nil || from != 2 || to != 1 {
		t.Fatalf("demotion: %d→%d %v", from, to, err)
	}
	if _, s := listingQty(t, e, pricey); s != "hidden" {
		t.Fatalf("pricey listing: %s", s)
	}
	// Held by staff: the job leaves it alone.
	e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true WHERE user_id = $1`, sid)
	if _, to, _ := rep.Review(ctx, sid); to != 3 {
		t.Fatalf("locked tier changed to %d", to)
	}
}

func TestSellerReportsEvidenceAndTax(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "rec_shop", true)
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/tax", map[string]bool{"collect_tax": true}); code != 200 ||
		body["seller"].(map[string]any)["collect_tax"] != true {
		t.Fatalf("tax setting: %d %v", code, body)
	}
	id := createListing(t, e, &httpClientT{sc}, rawCard("RC1", 2000))["id"].(string)
	b := buyer(t, e, "rita")
	do(t, b.c, "POST", e.ts.URL+"/v1/listings/"+id+"/messages", map[string]string{"body": "Is the corner sharp?"})
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+id, map[string]int{"quantity": 1})
	_, body := do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "rec_shop"})
	order := body["order_id"].(string)
	cs := sessionOf(t, e, order)
	if !e.pay.Params(cs).AutomaticTax {
		t.Fatal("automatic tax not requested")
	}
	e.pay.Complete(cs)
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/confirm", map[string]any{})
	_, body = do(t, sc, "GET", e.ts.URL+"/v1/orders/"+order, nil)
	o := body["order"].(map[string]any)
	if o["tax_cents"].(float64) != 165 || !strings.HasPrefix(o["stripe_payment_url"].(string), "https://dashboard.stripe.com/payments/pi_") {
		t.Fatalf("seller order: %v", o)
	}
	_, body = do(t, b.c, "GET", e.ts.URL+"/v1/orders/"+order, nil)
	if _, leaked := body["order"].(map[string]any)["stripe_payment_url"]; leaked {
		t.Fatal("buyer sees the Stripe link")
	}

	get := func(c *http.Client, path string) (int, string, http.Header) {
		res, err := c.Get(e.ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}
	today := time.Now().Format("2006-01-02")
	code, csv, h := get(sc, "/v1/seller/reports/sales.csv?from=2026-01-01&to="+today)
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if code != 200 || !strings.Contains(h.Get("Content-Disposition"), "attachment") || len(lines) != 2 ||
		!strings.Contains(lines[1], ",20.00,0.00,1.65,1.50,0.00,0.00,0.00,18.50,TX,78701") {
		t.Fatalf("csv: %d %q", code, csv)
	}
	if code, _, _ := get(sc, "/v1/seller/reports/sales.csv?from=2026-13-01"); code != 422 {
		t.Fatalf("bad date: %d", code)
	}
	code, js, _ := get(sc, "/v1/seller/reports/summary")
	var sum struct {
		Total struct {
			Orders   int `json:"orders"`
			NetCents int `json:"net_cents"`
		} `json:"total"`
		Months []any `json:"months"`
	}
	json.Unmarshal([]byte(js), &sum)
	if code != 200 || sum.Total.Orders != 1 || sum.Total.NetCents != 1850 || len(sum.Months) != 12 {
		t.Fatalf("summary: %d %s", code, js)
	}

	code, html, _ := get(sc, "/v1/orders/"+order+"/evidence")
	if code != 200 || !strings.Contains(html, "Is the corner sharp?") || !strings.Contains(html, "Charizard Holo RC1") ||
		!strings.Contains(html, "Listing version") {
		t.Fatalf("evidence: %d %.400s", code, html)
	}
	if code, _, _ := get(b.c, "/v1/orders/"+order+"/evidence"); code != 404 {
		t.Fatalf("buyer evidence: %d", code)
	}
}
