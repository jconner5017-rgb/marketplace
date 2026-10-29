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
	req.Header.Set("Content-Type", "application/json")
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
