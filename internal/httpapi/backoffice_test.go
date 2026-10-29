package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

func TestAdminIPAllowlist(t *testing.T) {
	e := setupEnv(t)
	boss := staff(t, e, "ip_boss", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	_, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/ip-rules", nil)
	if body["your_ip"] != "127.0.0.1" || body["enforced"] != false {
		t.Fatalf("rules: %v", body)
	}
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/ip-rules", map[string]string{"cidr": "198.51.100.0/24"}); code != 409 || errCode(body) != "lockout" {
		t.Fatalf("lockout guard: %d %v", code, body)
	}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/ip-rules", map[string]string{"cidr": "not an ip"}); code != 422 {
		t.Fatalf("bad cidr: %d", code)
	}
	code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/ip-rules", map[string]string{"cidr": "127.0.0.1", "label": "Test box"})
	if code != 201 || body["rule"].(map[string]any)["cidr"] != "127.0.0.1/32" {
		t.Fatalf("add own ip: %d %v", code, body)
	}
	own := int(body["rule"].(map[string]any)["id"].(float64))
	_, body = do(t, boss.c, "POST", e.ts.URL+"/v1/admin/ip-rules", map[string]string{"cidr": "198.51.100.0/24", "label": "Office"})
	office := int(body["rule"].(map[string]any)["id"].(float64))
	if code, _ := do(t, boss.c, "DELETE", fmt.Sprintf("%s/v1/admin/ip-rules/%d", e.ts.URL, own), nil); code != 409 {
		t.Fatalf("removing own network should be refused: %d", code)
	}
	if code, _ := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/dashboard", nil); code != 200 {
		t.Fatalf("allowed ip: %d", code)
	}

	// From an address outside the list: refused, with a fraud alert.
	e.db.Exec(`DELETE FROM admin_ip_rules WHERE id = $1`, own)
	e.srv.ipRules.Forget()
	if code, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/dashboard", nil); code != 403 || errCode(body) != "ip_not_allowed" {
		t.Fatalf("blocked ip: %d %v", code, body)
	}
	var alerts int
	e.db.QueryRow(`SELECT count(*) FROM fraud_alerts WHERE kind = 'admin_ip_blocked'`).Scan(&alerts)
	if alerts != 1 {
		t.Fatalf("alerts: %d", alerts)
	}
	// Break-glass switch.
	e.srv.ipRules.Disabled = true
	if code, _ := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/dashboard", nil); code != 200 {
		t.Fatalf("switched off: %d", code)
	}
	if code, _ := do(t, boss.c, "DELETE", fmt.Sprintf("%s/v1/admin/ip-rules/%d", e.ts.URL, office), nil); code != 204 {
		t.Fatalf("remove: %d", code)
	}
	var n int
	e.db.QueryRow(`SELECT count(*) FROM audit_log WHERE action LIKE 'security.ip_rule_%' AND actor_id = $1`, memberID(t, e, "ip_boss")).Scan(&n)
	if n != 3 {
		t.Fatalf("audit rows: %d", n)
	}
}

func webhook(t *testing.T, e *env, id, typ, account string, obj any) {
	t.Helper()
	o, _ := json.Marshal(obj)
	payload, _ := json.Marshal(map[string]any{"id": id, "type": typ, "account": account, "data": map[string]any{"object": json.RawMessage(o)}})
	req, _ := http.NewRequest("POST", e.ts.URL+"/v1/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", stripe.SignForTest(payload, testWebhookSecret, time.Now()))
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("webhook %s: %v %v", typ, err, res)
	}
	res.Body.Close()
}

func TestLedgerRefundsAndChargebacks(t *testing.T) {
	e := setupEnv(t)
	sc, acct := newSeller(t, e, "ledger_shop", true)
	id := createListing(t, e, &httpClientT{sc}, rawCard("L1", 1500))["id"].(string)
	b := buyer(t, e, "lena")
	do(t, b.c, "PUT", e.ts.URL+"/v1/cart/"+id, map[string]int{"quantity": 1})
	_, body := do(t, b.c, "POST", e.ts.URL+"/v1/checkout", map[string]any{"seller": "ledger_shop"})
	order := body["order_id"].(string)
	cs := sessionOf(t, e, order)
	e.pay.Complete(cs)
	do(t, b.c, "POST", e.ts.URL+"/v1/orders/"+order+"/confirm", map[string]any{})
	pi := "pi_" + cs

	kinds := func() map[string]int {
		out := map[string]int{}
		rows, _ := e.db.Query(`SELECT kind, amount_cents FROM ledger_entries WHERE order_id = $1`, order)
		defer rows.Close()
		for rows.Next() {
			var k string
			var c int
			rows.Scan(&k, &c)
			out[k] += c
		}
		return out
	}
	if k := kinds(); k["charge"] != 1500 || k["application_fee"] != 150 {
		t.Fatalf("ledger after payment: %v", k)
	}
	if _, err := e.db.Exec(`UPDATE ledger_entries SET amount_cents = 1 WHERE order_id = $1`, order); err == nil {
		t.Fatal("ledger should be append-only")
	}

	// Partial refund, then the webhook again, then the rest.
	webhook(t, e, "evt_r1", "charge.refunded", acct, map[string]any{"id": "ch_1", "payment_intent": pi, "amount": 1500, "amount_refunded": 500})
	webhook(t, e, "evt_r1b", "charge.refunded", acct, map[string]any{"id": "ch_1", "payment_intent": pi, "amount": 1500, "amount_refunded": 500})
	var st string
	var refunded int
	e.db.QueryRow(`SELECT status, refunded_cents FROM orders WHERE id = $1`, order).Scan(&st, &refunded)
	if st != "partially_refunded" || refunded != 500 || kinds()["refund"] != 500 {
		t.Fatalf("partial refund: %s %d %v", st, refunded, kinds())
	}
	webhook(t, e, "evt_f1", "application_fee.refunded", "", map[string]any{"id": "fee_1", "charge": "ch_1", "amount_refunded": 50})
	webhook(t, e, "evt_r2", "charge.refunded", acct, map[string]any{"id": "ch_1", "payment_intent": pi, "amount": 1500, "amount_refunded": 1500})
	e.db.QueryRow(`SELECT status, refunded_cents FROM orders WHERE id = $1`, order).Scan(&st, &refunded)
	if k := kinds(); st != "refunded" || k["refund"] != 1500 || k["fee_refund"] != 50 {
		t.Fatalf("full refund: %s %v", st, k)
	}
	// A refund for some other account's charge changes nothing.
	webhook(t, e, "evt_r3", "charge.refunded", "acct_other", map[string]any{"id": "ch_1", "payment_intent": pi, "amount": 1500, "amount_refunded": 1500})

	// Chargeback opened, then won: the buyer gets a strike.
	webhook(t, e, "evt_d1", "charge.dispute.created", acct, map[string]any{"id": "dp_1", "charge": "ch_1", "payment_intent": pi, "amount": 1500, "reason": "fraudulent", "status": "needs_response"})
	var open int
	e.db.QueryRow(`SELECT count(*) FROM refund_reports WHERE kind = 'chargeback' AND status = 'open'`).Scan(&open)
	if open != 1 {
		t.Fatalf("open chargebacks: %d", open)
	}
	webhook(t, e, "evt_d2", "charge.dispute.closed", acct, map[string]any{"id": "dp_1", "charge": "ch_1", "payment_intent": pi, "amount": 1500, "reason": "fraudulent", "status": "won"})
	webhook(t, e, "evt_d2b", "charge.dispute.closed", acct, map[string]any{"id": "dp_1", "charge": "ch_1", "payment_intent": pi, "amount": 1500, "reason": "fraudulent", "status": "won"})
	var strikes int
	e.db.QueryRow(`SELECT buyer_strikes FROM users WHERE username = 'lena'`).Scan(&strikes)
	if k := kinds(); k["chargeback_opened"] != 1500 || k["chargeback_won"] != 1500 || strikes != 1 {
		t.Fatalf("dispute: %v strikes %d", k, strikes)
	}

	// Staff see the ledger on the order.
	boss := staff(t, e, "ledger_boss", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/admin/orders/"+order, nil)
	if n := len(body["ledger"].([]any)); n < 6 {
		t.Fatalf("admin ledger: %d %v", n, body["ledger"])
	}
}

func TestWarningsTiersAppealsAndBans(t *testing.T) {
	e := setupEnv(t)
	mod := staff(t, e, "mod_mia", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, mod.c)
	sc, _ := newSeller(t, e, "tier_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 2`)
	pricey := createListing(t, e, &httpClientT{sc}, rawCard("T1", 5000))["id"].(string)
	cheap := createListing(t, e, &httpClientT{sc}, rawCard("T2", 1000))["id"].(string)
	sid := memberID(t, e, "tier_shop")

	// Warning, visible to the member; one appeal; overturned → rescinded.
	code, body := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+sid+"/warn", map[string]string{"reason": "Off-site payment request in messages"})
	if code != 201 {
		t.Fatalf("warn: %d %v", code, body)
	}
	wid := body["warning_id"].(string)
	_, st := do(t, sc, "GET", e.ts.URL+"/v1/me/standing", nil)
	if len(st["warnings"].([]any)) != 1 {
		t.Fatalf("standing: %v", st)
	}
	appeal := map[string]string{"about": "warning", "ref": wid, "message": "That was a buyer's message quoted back, not my request."}
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/me/appeals", appeal); code != 201 {
		t.Fatalf("appeal: %d %v", code, body)
	}
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/me/appeals", appeal); code != 409 {
		t.Fatalf("second appeal: %d", code)
	}
	_, body = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/appeals?status=open", nil)
	aps := body["appeals"].([]any)
	if len(aps) != 1 || aps[0].(map[string]any)["decision"] != "Off-site payment request in messages" {
		t.Fatalf("appeals queue: %v", body)
	}
	aid := aps[0].(map[string]any)["id"].(string)
	if code, body := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/appeals/"+aid, map[string]string{"decision": "overturned", "note": "Quoted text, agreed"}); code != 200 {
		t.Fatalf("decide: %d %v", code, body)
	}
	_, st = do(t, sc, "GET", e.ts.URL+"/v1/me/standing", nil)
	if st["warnings"].([]any)[0].(map[string]any)["rescinded_at"] == nil {
		t.Fatalf("warning should be rescinded: %v", st)
	}

	// Demotion hides listings above the new cap; overturning restores them.
	if code, body := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+sid+"/tier", map[string]any{"tier": 1, "lock": true, "reason": "Several late shipments"}); code != 200 {
		t.Fatalf("demote: %d %v", code, body)
	}
	if _, s := listingQty(t, e, pricey); s != "hidden" {
		t.Fatalf("pricey after demotion: %s", s)
	}
	if _, s := listingQty(t, e, cheap); s != "active" {
		t.Fatalf("cheap after demotion: %s", s)
	}
	_, st = do(t, sc, "GET", e.ts.URL+"/v1/me/standing", nil)
	ref := st["tier_change_id"].(string)
	do(t, sc, "POST", e.ts.URL+"/v1/me/appeals", map[string]string{"about": "demotion", "ref": ref, "message": "The carrier lost the scans; tracking shows on time."})
	_, body = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/appeals?status=open", nil)
	aid = body["appeals"].([]any)[0].(map[string]any)["id"].(string)
	do(t, mod.c, "POST", e.ts.URL+"/v1/admin/appeals/"+aid, map[string]string{"decision": "overturned", "note": "Tracking checks out"})
	var tier int
	e.db.QueryRow(`SELECT tier FROM seller_profiles WHERE user_id = $1`, sid).Scan(&tier)
	if _, s := listingQty(t, e, pricey); s != "active" || tier != 2 {
		t.Fatalf("after overturn: %s tier %d", s, tier)
	}

	// Ban: email variants can't register; the member can appeal from outside.
	b := buyer(t, e, "bad_actor")
	e.db.Exec(`UPDATE users SET email = 'bad.actor@gmail.com' WHERE username = 'bad_actor'`)
	bid := memberID(t, e, "bad_actor")
	if code, body := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+bid+"/status", map[string]any{"status": "banned", "reason": "Repeated shill bidding"}); code != 204 {
		t.Fatalf("ban: %d %v", code, body)
	}
	_ = b
	code, body = do(t, client(t), "POST", e.ts.URL+"/v1/auth/signup", map[string]string{"email": "Bad.Actor+new@googlemail.com", "username": "fresh_start", "password": "longenough1"})
	if code != 403 || errCode(body) != "email_blocked" {
		t.Fatalf("banned email signup: %d %v", code, body)
	}
	if code, _ := do(t, client(t), "POST", e.ts.URL+"/v1/appeals/account", map[string]string{"email": "bad.actor@gmail.com", "password": "wrong-password", "message": "I was not shill bidding at all, that was my brother."}); code != 401 {
		t.Fatalf("appeal wrong password: %d", code)
	}
	code, body = do(t, client(t), "POST", e.ts.URL+"/v1/appeals/account", map[string]string{"email": "bad.actor@gmail.com", "password": "longenough1", "message": "I was not shill bidding at all, that was my brother."})
	if code != 201 {
		t.Fatalf("account appeal: %d %v", code, body)
	}
	_, body = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/appeals?status=open", nil)
	aid = body["appeals"].([]any)[0].(map[string]any)["id"].(string)
	do(t, mod.c, "POST", e.ts.URL+"/v1/admin/appeals/"+aid, map[string]string{"decision": "overturned", "note": "Separate household confirmed"})
	if code, _ := do(t, client(t), "POST", e.ts.URL+"/v1/auth/login", map[string]string{"email": "bad.actor@gmail.com", "password": "longenough1"}); code != 200 {
		t.Fatalf("login after ban overturned: %d", code)
	}
	var blocked int
	e.db.QueryRow(`SELECT count(*) FROM blocked_identifiers WHERE user_id = $1`, bid).Scan(&blocked)
	if blocked != 0 {
		t.Fatalf("blocklist should be cleared: %d", blocked)
	}
}
