package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/fcm"
)

// api calls the API with a seller key.
func api(t *testing.T, e *env, key, method, path string, body any, idem string) (int, map[string]any, http.Header) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, rdr)
	if body != nil { // like FireCardManager: no Content-Type on bodyless calls
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out, res.Header
}

func newKey(t *testing.T, e *env, c *http.Client) string {
	t.Helper()
	code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/api-keys", map[string]string{"name": "FireCardManager"})
	if code != 201 || !strings.HasPrefix(body["api_key"].(string), "ss_key_") {
		t.Fatalf("create key: %d %v", code, body)
	}
	return body["api_key"].(string)
}

func TestFCMListingsAPI(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "fcm_shop", true)
	key := newKey(t, e, sc)
	_, body := do(t, sc, "GET", e.ts.URL+"/v1/seller/connect", nil)
	if ks := body["keys"].([]any); len(ks) != 1 || ks[0].(map[string]any)["prefix"] == nil {
		t.Fatalf("keys: %v", body)
	}
	if b, _ := json.Marshal(body); strings.Contains(string(b), key) {
		t.Fatal("key listed in full")
	}
	if code, _, _ := api(t, e, "ss_key_nope", "POST", "/v1/listings", rawCard("X", 500), ""); code != 401 {
		t.Fatalf("bad key: %d", code)
	}

	card := rawCard("FCM-1", 900)
	code, b1, _ := api(t, e, key, "POST", "/v1/listings", card, "idem-1")
	if code != 201 {
		t.Fatalf("create: %d %v", code, b1)
	}
	code, b2, h := api(t, e, key, "POST", "/v1/listings", card, "idem-1")
	if code != 201 || h.Get("Idempotent-Replayed") != "true" ||
		b2["listing"].(map[string]any)["id"] != b1["listing"].(map[string]any)["id"] {
		t.Fatalf("replay: %d %v %v", code, h, b2)
	}
	card["price_cents"] = 950
	if code, _, _ := api(t, e, key, "POST", "/v1/listings", card, "idem-1"); code != 422 {
		t.Fatalf("key reuse: %d", code)
	}
	if code, b, _ := api(t, e, key, "POST", "/v1/listings", card, ""); code != 200 || b["created"] != false {
		t.Fatalf("upsert existing: %d %v", code, b)
	}

	bad := rawCard("FCM-3", 99999)
	code, bulk, _ := api(t, e, key, "POST", "/v1/listings/bulk", map[string]any{"listings": []any{rawCard("FCM-2", 400), bad, map[string]any{"title": "no sku"}}}, "")
	if code != 200 || bulk["created"].(float64) != 1 || bulk["errors"].(float64) != 2 {
		t.Fatalf("bulk: %d %v", code, bulk)
	}

	code, p, _ := api(t, e, key, "PATCH", "/v1/listings/FCM-2", map[string]any{"price_cents": 450, "quantity": 3}, "")
	if code != 200 || p["listing"].(map[string]any)["price_cents"].(float64) != 450 {
		t.Fatalf("patch: %d %v", code, p)
	}
	// Without an API key, a bodyless DELETE is still refused (CSRF guard).
	if req, _ := http.NewRequest("DELETE", e.ts.URL+"/v1/listings/FCM-2", nil); true {
		if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusUnsupportedMediaType {
			t.Fatalf("keyless bodyless DELETE: %v %v", err, res.StatusCode)
		}
	}
	if code, _, _ := api(t, e, key, "DELETE", "/v1/listings/FCM-2", nil, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _, _ := api(t, e, key, "DELETE", "/v1/listings/FCM-2", nil, ""); code != 404 {
		t.Fatalf("delete again: %d", code)
	}

	// Revoked keys stop working.
	kid := body["keys"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := do(t, sc, "DELETE", e.ts.URL+"/v1/seller/api-keys/"+kid, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _, _ := api(t, e, key, "GET", "/v1/orders", nil, ""); code != 401 {
		t.Fatalf("revoked key: %d", code)
	}
}

type hookServer struct {
	mu     sync.Mutex
	status int
	got    []map[string]any
	sigs   []string
	bodies [][]byte
}

func (h *hookServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	defer h.mu.Unlock()
	var m map[string]any
	json.Unmarshal(b, &m)
	h.got = append(h.got, m)
	h.sigs = append(h.sigs, r.Header.Get("SS-Signature"))
	h.bodies = append(h.bodies, b)
	w.WriteHeader(h.status)
}

func TestFCMWebhooksReleaseOversellAndReconcile(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	sc, _ := newSeller(t, e, "hook_shop", true)
	key := newKey(t, e, sc)
	hook := &hookServer{status: 200}
	hs := httptest.NewServer(hook)
	defer hs.Close()
	inv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("SS-Signature") == "" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"items":[{"external_id":"H-GONE","quantity":0},{"external_id":"H-LOW","quantity":1},{"external_id":"NOT-HERE","quantity":0}]}`))
	}))
	defer inv.Close()

	code, body := do(t, sc, "PUT", e.ts.URL+"/v1/seller/connect", map[string]any{"webhook_url": hs.URL + "/hook", "inventory_url": inv.URL})
	secret, _ := body["signing_secret"].(string)
	if code != 200 || !strings.HasPrefix(secret, "whsec_ss_") {
		t.Fatalf("connect: %d %v", code, body)
	}
	e.fcm.AllowPrivate = false
	if code, _ := do(t, sc, "PUT", e.ts.URL+"/v1/seller/connect", map[string]any{"webhook_url": "http://10.0.0.5/hook"}); code != 422 {
		t.Fatalf("private url accepted: %d", code)
	}
	e.fcm.AllowPrivate = true

	// A paid order sends order.paid with the SKU, signed.
	api(t, e, key, "POST", "/v1/listings", rawCard("H-1", 800), "")
	lid := ""
	e.db.QueryRow(`SELECT id FROM listings WHERE external_id = 'H-1'`).Scan(&lid)
	b := buyer(t, e, "hank")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+lid, map[string]int{"quantity": 1})
	_, body = do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "hook_shop"})
	order := body["order_id"].(string)
	cs := sessionOf(t, e, order)
	e.pay.Complete(cs)
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/confirm", map[string]any{})
	if n, err := e.fcm.RunDeliveries(ctx); err != nil || n != 1 {
		t.Fatalf("deliveries: %d %v", n, err)
	}
	ev := hook.got[0]
	items := ev["data"].(map[string]any)["items"].([]any)
	if ev["type"] != "order.paid" || items[0].(map[string]any)["external_id"] != "H-1" {
		t.Fatalf("event: %v", ev)
	}
	ts := strings.TrimPrefix(strings.SplitN(hook.sigs[0], ",", 2)[0], "t=")
	tm := time.Unix(atoi(ts), 0)
	if fcm.Sign(secret, hook.bodies[0], tm) != hook.sigs[0] {
		t.Fatal("bad signature")
	}
	_, ob, _ := api(t, e, key, "GET", "/v1/orders", nil, "")
	if l := ob["orders"].([]any)[0].(map[string]any)["lines"].([]any)[0].(map[string]any); l["external_id"] != "H-1" {
		t.Fatalf("orders for FCM: %v", l)
	}

	// FCM marks a paid order shipped with the key.
	api(t, e, key, "POST", "/v1/listings", rawCard("H-3", 600), "")
	e.db.QueryRow(`SELECT id FROM listings WHERE external_id = 'H-3'`).Scan(&lid)
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+lid, map[string]int{"quantity": 1})
	_, body = do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "hook_shop"})
	shipOrder := body["order_id"].(string)
	e.pay.Complete(sessionOf(t, e, shipOrder))
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+shipOrder+"/confirm", map[string]any{})
	if code, r, _ := api(t, e, key, "POST", "/v1/orders/"+shipOrder+"/ship",
		map[string]any{"carrier": "USPS", "tracking_number": "9400100000000000000001"}, ""); code != 200 ||
		r["order"].(map[string]any)["status"] != "shipped" {
		t.Fatalf("ship with key: %d %v", code, r)
	}

	// The other site sold it first: our order is refunded.
	if code, r, _ := api(t, e, key, "POST", "/v1/orders/"+order+"/oversold",
		map[string]any{"external_id": "H-1", "sold_elsewhere_at": time.Now().Add(-time.Hour)}, ""); code != 200 || r["outcome"] != "refunded" {
		t.Fatalf("oversold: %d %v", code, r)
	}
	var st string
	e.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, order).Scan(&st)
	if st != "refunded" || len(e.pay.Refunds) != 1 {
		t.Fatalf("after oversell: %s %v", st, e.pay.Refunds)
	}
	if _, r, _ := api(t, e, key, "POST", "/v1/orders/"+order+"/oversold",
		map[string]any{"external_id": "H-1", "sold_elsewhere_at": time.Now().Add(-time.Hour)}, ""); r["outcome"] != "refunded" || len(e.pay.Refunds) != 1 {
		t.Fatalf("repeat oversold should not refund twice: %v", e.pay.Refunds)
	}

	// Card deleted by FCM while a buyer is on Stripe's page: checkout closed, no charge.
	api(t, e, key, "POST", "/v1/listings", rawCard("H-2", 700), "")
	e.db.QueryRow(`SELECT id FROM listings WHERE external_id = 'H-2'`).Scan(&lid)
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+lid, map[string]int{"quantity": 1})
	_, body = do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "hook_shop"})
	pending := body["order_id"].(string)
	if code, _, _ := api(t, e, key, "DELETE", "/v1/listings/H-2", nil, ""); code != 204 {
		t.Fatalf("delete during checkout: %d", code)
	}
	e.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, pending).Scan(&st)
	if st != "cancelled" {
		t.Fatalf("pending order: %s", st)
	}

	// Failing endpoint: retried with backoff, alerts after an hour.
	hook.status = 500
	do(t, sc, "POST", e.ts.URL+"/v1/seller/connect/test", map[string]any{})
	e.fcm.RunDeliveries(ctx)
	var attempts int
	var next time.Time
	e.db.QueryRow(`SELECT attempts, next_attempt_at FROM webhook_deliveries WHERE event = 'ping'`).Scan(&attempts, &next)
	if attempts != 1 || next.Before(time.Now().Add(20*time.Second)) {
		t.Fatalf("retry: %d %v", attempts, next)
	}
	e.db.Exec(`UPDATE webhook_deliveries SET next_attempt_at = now(), created_at = now() - interval '2 hours' WHERE event = 'ping'`)
	e.fcm.RunDeliveries(ctx)
	var alerts int
	e.db.QueryRow(`SELECT count(*) FROM fraud_alerts WHERE kind = 'webhook_failing'`).Scan(&alerts)
	if alerts != 1 {
		t.Fatalf("alerts: %d", alerts)
	}

	// Nightly reconciliation against the inventory endpoint.
	api(t, e, key, "POST", "/v1/listings", rawCard("H-GONE", 500), "")
	low := rawCard("H-LOW", 500)
	low["quantity"] = 3
	api(t, e, key, "POST", "/v1/listings", low, "")
	sid := memberID(t, e, "hook_shop")
	r, err := e.fcm.Reconcile(ctx, sid, e.co.ReleaseListing)
	if err != nil || r.Removed != 1 || r.Lowered != 1 {
		t.Fatalf("reconcile: %+v %v", r, err)
	}
	var gone string
	var q int
	e.db.QueryRow(`SELECT status FROM listings WHERE external_id = 'H-GONE'`).Scan(&gone)
	e.db.QueryRow(`SELECT quantity FROM listings WHERE external_id = 'H-LOW'`).Scan(&q)
	if gone != "removed" || q != 1 {
		t.Fatalf("after reconcile: %s %d", gone, q)
	}
}

func atoi(s string) int64 {
	var n int64
	for _, c := range s {
		n = n*10 + int64(c-'0')
	}
	return n
}

// A card that sold on the site isn't put back up by inventory software,
// even if the seller selects it and lists again before FCM sees the sale.
func TestFCMNoRelistAfterSale(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "relist_shop", true)
	key := newKey(t, e, sc)
	card := rawCard("FCM-77", 900)
	code, b, _ := api(t, e, key, "POST", "/v1/listings", card, "")
	if code != 201 {
		t.Fatalf("list: %d %v", code, b)
	}
	id := b["listing"].(map[string]any)["id"].(string)
	// Re-listing while it's still for sale just updates it.
	if code, b, _ := api(t, e, key, "POST", "/v1/listings", card, ""); code != 200 || b["created"] != false {
		t.Fatalf("update: %d %v", code, b)
	}
	buy := buyer(t, e, "relist_buyer")
	do(t, buy.c, "PUT", e.ts.URL+"/v1/cart/"+id, map[string]int{"quantity": 1})
	if code, body := do(t, buy.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "relist_shop"}); code != 201 {
		t.Fatalf("checkout: %d %v", code, body)
	}
	code, b, _ = api(t, e, key, "POST", "/v1/listings", card, "")
	if code != 409 || b["error"].(map[string]any)["code"] != "recently_sold" {
		t.Fatalf("relist after sale: %d %v", code, b)
	}
	var qty int
	e.db.QueryRow(`SELECT quantity FROM listings WHERE id = $1`, id).Scan(&qty)
	if qty != 0 {
		t.Fatalf("quantity reset to %d", qty)
	}
	// The bulk endpoint refuses it too.
	code, b, _ = api(t, e, key, "POST", "/v1/listings/bulk", map[string]any{"listings": []any{card}}, "")
	if code != 200 || !strings.Contains(b["results"].([]any)[0].(map[string]any)["error"].(string), "sold") {
		t.Fatalf("bulk relist: %d %v", code, b)
	}
}

// Listings from inventory software pause after 3 days without a check-in
// and come back on the next sync; the seller's own listings aren't touched.
func TestFCMStaleSyncPause(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "stale_shop", true)
	key := newKey(t, e, sc)
	_, b, _ := api(t, e, key, "POST", "/v1/listings", rawCard("FCM-501", 900), "")
	synced := b["listing"].(map[string]any)["id"].(string)
	own := createListing(t, e, &httpClientT{sc}, rawCard("HAND-1", 800))["id"].(string)

	// Checked in recently: nothing paused.
	if p, err := e.fcm.PauseStale(context.Background()); err != nil || len(p) != 0 {
		t.Fatalf("fresh: %v %v", p, err)
	}
	e.db.Exec(`UPDATE api_keys SET last_used_at = now() - interval '4 days'`)
	p, err := e.fcm.PauseStale(context.Background())
	if err != nil || len(p) != 1 || p[0].Listings != 1 || p[0].Email != "stale_shop@example.com" {
		t.Fatalf("pause: %+v %v", p, err)
	}
	status := func(id string) (s string) {
		e.db.QueryRow(`SELECT status FROM listings WHERE id = $1`, id).Scan(&s)
		return
	}
	if status(synced) != "hidden" || status(own) != "active" {
		t.Fatalf("after pause: synced %s, own %s", status(synced), status(own))
	}
	if code, _ := do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+synced, nil); code != 404 {
		t.Fatalf("paused listing still public: %d", code)
	}
	var notes int
	e.db.QueryRow(`SELECT count(*) FROM notifications WHERE title LIKE '%listings paused%'`).Scan(&notes)
	if notes != 1 {
		t.Fatalf("notices %d", notes)
	}
	// Any API call from FCM brings them back.
	if code, _, _ := api(t, e, key, "GET", "/v1/orders", nil, ""); code != 200 {
		t.Fatalf("orders: %d", code)
	}
	if status(synced) != "active" {
		t.Fatalf("not resumed: %s", status(synced))
	}
	if p, _ := e.fcm.PauseStale(context.Background()); len(p) != 0 {
		t.Fatal("paused again right after a sync")
	}
}

// A FireCardManager Web key doesn't stand in for the desktop app: while the
// PC is away, the website's calls neither prevent the pause nor undo it.
// A seller who only uses the website is judged by its key.
func TestFCMWebKeyDoesNotCountAsDesktopSync(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "web_shop", true)
	desk := newKey(t, e, sc)
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/api-keys", map[string]string{"kind": "web"})
	if code != 201 || body["key"].(map[string]any)["kind"] != "web" || body["key"].(map[string]any)["name"] != "FireCardManager Web" {
		t.Fatalf("web key: %d %v", code, body)
	}
	web := body["api_key"].(string)
	_, b, _ := api(t, e, desk, "POST", "/v1/listings", rawCard("FCM-601", 900), "")
	id := b["listing"].(map[string]any)["id"].(string)
	status := func() (s string) {
		e.db.QueryRow(`SELECT status FROM listings WHERE id = $1`, id).Scan(&s)
		return
	}
	// The PC goes quiet; the website keeps calling.
	e.db.Exec(`UPDATE api_keys SET last_used_at = now() - interval '4 days' WHERE kind = 'desktop'`)
	api(t, e, web, "GET", "/v1/seller/listings/sync", nil, "")
	if p, _ := e.fcm.PauseStale(context.Background()); len(p) != 1 || status() != "hidden" {
		t.Fatalf("web key kept the listings up: %v %s", p, status())
	}
	api(t, e, web, "GET", "/v1/seller/listings/sync", nil, "")
	if status() != "hidden" {
		t.Fatal("web key brought paused listings back")
	}
	api(t, e, desk, "GET", "/v1/orders", nil, "")
	if status() != "active" {
		t.Fatal("desktop check-in didn't resume")
	}
	// Without a desktop key, the website's key is what counts.
	keys := func() []any { _, b := do(t, sc, "GET", e.ts.URL+"/v1/seller/connect", nil); return b["keys"].([]any) }
	for _, k := range keys() {
		if k.(map[string]any)["kind"] == "desktop" {
			do(t, sc, "DELETE", e.ts.URL+"/v1/seller/api-keys/"+k.(map[string]any)["id"].(string), nil)
		}
	}
	e.db.Exec(`UPDATE api_keys SET last_used_at = now() - interval '4 days'`)
	e.fcm.PauseStale(context.Background())
	api(t, e, web, "GET", "/v1/seller/listings/sync", nil, "")
	if status() != "active" {
		t.Fatalf("web-only seller not resumed by the web key: %s", status())
	}
}
