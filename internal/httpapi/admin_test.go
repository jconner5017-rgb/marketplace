package httpapi

import (
	"context"
	"encoding/base32"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

func staff(t *testing.T, e *env, name, role string) *httpClientT {
	t.Helper()
	c := buyer(t, e, name)
	if _, err := e.db.Exec(`UPDATE users SET staff_role = $2 WHERE username = $1`, name, role); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStaffAuthenticator(t *testing.T) {
	e := setupEnv(t)
	boss := staff(t, e, "boss_mfa", admin.SuperAdmin)

	_, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/session", nil)
	if body["mfa"] != "setup_required" || body["staff_role"] != "super_admin" {
		t.Fatalf("session before setup: %v", body)
	}
	if code, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/dashboard", nil); code != 403 || errCode(body) != "mfa_required" {
		t.Fatalf("dashboard before setup: %d %v", code, body)
	}
	if code, _ := do(t, buyer(t, e, "not_staff").c, "GET", e.ts.URL+"/v1/admin/session", nil); code != 403 {
		t.Fatalf("non-staff session: %d", code)
	}

	// Setup with a wrong code fails; the right one returns backup codes.
	_, body = do(t, boss.c, "POST", e.ts.URL+"/v1/admin/mfa/setup", map[string]any{})
	raw, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(body["secret"].(string))
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/mfa/confirm", map[string]string{"code": "000000"}); code != 422 {
		t.Fatalf("wrong confirm code: %d", code)
	}
	now := time.Now().Unix() / 30
	code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/mfa/confirm", map[string]string{"code": admin.TOTP(raw, now)})
	if code != 200 {
		t.Fatalf("confirm: %d %v", code, body)
	}
	backups := body["backup_codes"].([]any)
	if code, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/dashboard", nil); code != 200 || len(body["stats"].([]any)) < 10 {
		t.Fatalf("dashboard after setup: %d %v", code, body)
	}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/mfa/setup", map[string]any{}); code != 409 {
		t.Fatalf("second setup: %d", code)
	}

	// A new sign-in needs the check again. The code just used can't be reused.
	fresh := client(t)
	do(t, fresh, "POST", e.ts.URL+"/v1/auth/login", map[string]string{"email": "boss_mfa@example.com", "password": "longenough1"})
	_, body = do(t, fresh, "GET", e.ts.URL+"/v1/admin/session", nil)
	if body["mfa"] != "verify_required" {
		t.Fatalf("new session: %v", body)
	}
	if code, _ := do(t, fresh, "POST", e.ts.URL+"/v1/admin/mfa/verify", map[string]string{"code": admin.TOTP(raw, now)}); code != 422 {
		t.Fatalf("reused code: %d", code)
	}
	// A backup code works once.
	bc := backups[0].(string)
	if code, body := do(t, fresh, "POST", e.ts.URL+"/v1/admin/mfa/verify", map[string]string{"code": bc}); code != 200 || body["used_backup"] != true {
		t.Fatalf("backup code: %d %v", code, body)
	}
	other := client(t)
	do(t, other, "POST", e.ts.URL+"/v1/auth/login", map[string]string{"email": "boss_mfa@example.com", "password": "longenough1"})
	if code, _ := do(t, other, "POST", e.ts.URL+"/v1/admin/mfa/verify", map[string]string{"code": bc}); code != 422 {
		t.Fatalf("backup code reused: %d", code)
	}
	// Four more wrong codes (five in a row) lock staff access.
	for i := 0; i < 3; i++ {
		do(t, other, "POST", e.ts.URL+"/v1/admin/mfa/verify", map[string]string{"code": "111111"})
	}
	if code, body := do(t, other, "POST", e.ts.URL+"/v1/admin/mfa/verify", map[string]string{"code": "111111"}); code != 429 || errCode(body) != "mfa_locked" {
		t.Fatalf("lockout: %d %v", code, body)
	}
	if code, _ := do(t, other, "POST", e.ts.URL+"/v1/admin/mfa/verify", map[string]string{"code": admin.TOTP(raw, now+1)}); code != 429 {
		t.Fatalf("right code while locked: %d", code)
	}

	// Every step is in the audit log, and the chain checks out.
	_, sess := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/session", nil)
	bossID := sess["user"].(map[string]any)["id"].(string)
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/admin/audit?action=staff.&actor="+bossID, nil)
	actions := map[string]int{}
	for _, en := range body["entries"].([]any) {
		actions[en.(map[string]any)["action"].(string)]++
	}
	if actions["staff.mfa_enrolled"] != 1 || actions["staff.sign_in"] != 1 || actions["staff.mfa_locked"] != 1 || actions["staff.mfa_failed"] < 2 {
		t.Fatalf("audit actions: %v", actions)
	}
	if _, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/audit/verify", nil); body["ok"] != true {
		t.Fatalf("chain: %v", body)
	}
}

func TestAuditLogIsTamperEvident(t *testing.T) {
	e := setupEnv(t)
	a := &admin.Audit{DB: e.db}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := a.Log(ctx, nil, admin.Entry{ActorEmail: "x@example.com", Action: "test.step",
			Details: map[string]any{"n": i, "note": "ok"}}); err != nil {
			t.Fatal(err)
		}
	}
	if res, err := a.Verify(ctx); err != nil || !res.OK {
		t.Fatalf("verify: %v %v", res, err)
	}
	if _, err := e.db.Exec(`UPDATE audit_log SET reason = 'edited'`); err == nil {
		t.Fatal("update should be refused")
	}
	if _, err := e.db.Exec(`DELETE FROM audit_log`); err == nil {
		t.Fatal("delete should be refused")
	}
	// Even with the guard switched off directly in the database, an edit breaks the chain.
	var last int64
	e.db.QueryRow(`SELECT max(id) FROM audit_log`).Scan(&last)
	e.db.Exec(`ALTER TABLE audit_log DISABLE TRIGGER audit_log_no_update`)
	e.db.Exec(`UPDATE audit_log SET reason = 'covered up' WHERE id = $1 - 1`, last)
	e.db.Exec(`ALTER TABLE audit_log ENABLE TRIGGER audit_log_no_update`)
	res, _ := a.Verify(ctx)
	if res.OK || res.BrokenAt != last-1 {
		t.Fatalf("tampering not caught: %+v", res)
	}
	// Put it back so later tests start with a valid chain.
	e.db.Exec(`ALTER TABLE audit_log DISABLE TRIGGER audit_log_no_update`)
	e.db.Exec(`UPDATE audit_log SET reason = '' WHERE id = $1 - 1`, last)
	e.db.Exec(`ALTER TABLE audit_log ENABLE TRIGGER audit_log_no_update`)
}

func TestStaffRolesAndAccountStatus(t *testing.T) {
	e := setupEnv(t)
	aud := staff(t, e, "auditor_amy", admin.Auditor)
	enrollMFA(t, e.ts.URL, aud.c)
	if code, _ := do(t, aud.c, "GET", e.ts.URL+"/v1/admin/audit", nil); code != 200 {
		t.Fatalf("auditor reads audit: %d", code)
	}
	if code, body := do(t, aud.c, "POST", e.ts.URL+"/v1/admin/announcements", map[string]string{"title": "Hi"}); code != 403 || errCode(body) != "forbidden" {
		t.Fatalf("auditor announcing: %d %v", code, body)
	}

	// Frozen: can sign in and browse, can't change anything.
	f := buyer(t, e, "frosty")
	e.db.Exec(`UPDATE users SET status = 'frozen' WHERE username = 'frosty'`)
	if code, _ := do(t, f.c, "GET", e.ts.URL+"/v1/me/summary", nil); code != 200 {
		t.Fatalf("frozen browsing: %d", code)
	}
	if code, body := do(t, f.c, "PUT", e.ts.URL+"/v1/watchlist/00000000-0000-0000-0000-000000000000", map[string]any{}); code != 403 || errCode(body) != "account_frozen" {
		t.Fatalf("frozen write: %d %v", code, body)
	}
	if code, _ := do(t, f.c, "POST", e.ts.URL+"/v1/auth/logout", map[string]any{}); code != 204 {
		t.Fatalf("frozen logout: %d", code)
	}

	// Suspended: no sign-in until the date; afterwards the account is active again.
	s := buyer(t, e, "suzy")
	e.db.Exec(`UPDATE users SET status = 'suspended', suspended_until = now() + interval '1 day' WHERE username = 'suzy'`)
	if code, _ := do(t, s.c, "GET", e.ts.URL+"/v1/me", nil); code != 401 {
		t.Fatalf("suspended session: %d", code)
	}
	login := func() int {
		code, _ := do(t, client(t), "POST", e.ts.URL+"/v1/auth/login", map[string]string{"email": "suzy@example.com", "password": "longenough1"})
		return code
	}
	if code := login(); code != 403 {
		t.Fatalf("suspended login: %d", code)
	}
	e.db.Exec(`UPDATE users SET suspended_until = now() - interval '1 minute' WHERE username = 'suzy'`)
	if code := login(); code != 200 {
		t.Fatalf("login after suspension: %d", code)
	}
	var st string
	e.db.QueryRow(`SELECT status FROM users WHERE username = 'suzy'`).Scan(&st)
	if st != "active" {
		t.Fatalf("status after suspension: %s", st)
	}
	var events int
	e.db.QueryRow(`SELECT count(*) FROM login_events WHERE email = 'suzy@example.com' AND NOT success AND reason = 'disabled'`).Scan(&events)
	if events != 1 {
		t.Fatalf("login events: %d", events)
	}
}

func memberID(t *testing.T, e *env, username string) string {
	t.Helper()
	var id string
	if err := e.db.QueryRow(`SELECT id FROM users WHERE username = $1`, username).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAdminMembersAndStaff(t *testing.T) {
	e := setupEnv(t)
	boss := staff(t, e, "boss_users", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	mod := staff(t, e, "mod_mia", admin.Moderator)
	enrollMFA(t, e.ts.URL, mod.c)
	sup := staff(t, e, "support_sam", admin.Support)
	enrollMFA(t, e.ts.URL, sup.c)
	sc, _ := newSeller(t, e, "sketchy_seller", true)
	createListing(t, e, &httpClientT{sc}, rawCard("SK1", 900))
	target := buyer(t, e, "trouble")
	tid, sid := memberID(t, e, "trouble"), memberID(t, e, "sketchy_seller")

	// Search and detail.
	_, body := do(t, sup.c, "GET", e.ts.URL+"/v1/admin/users?q=troub", nil)
	if body["total"].(float64) != 1 {
		t.Fatalf("search: %v", body)
	}
	_, body = do(t, sup.c, "GET", e.ts.URL+"/v1/admin/users/"+sid, nil)
	m := body["member"].(map[string]any)
	if m["seller"].(map[string]any)["review_status"] != "pending" || m["active_listings"].(float64) != 1 {
		t.Fatalf("seller detail: %v", m)
	}

	// Support edits a profile (reason required); can't freeze.
	if code, _ := do(t, sup.c, "PATCH", e.ts.URL+"/v1/admin/users/"+tid, map[string]any{"username": "trouble2"}); code != 422 {
		t.Fatalf("edit without reason: %d", code)
	}
	if code, body := do(t, sup.c, "PATCH", e.ts.URL+"/v1/admin/users/"+tid, map[string]any{"username": "trouble2", "reason": "member asked"}); code != 204 {
		t.Fatalf("edit: %d %v", code, body)
	}
	if code, _ := do(t, sup.c, "POST", e.ts.URL+"/v1/admin/users/"+tid+"/status", map[string]any{"status": "frozen", "reason": "x"}); code != 403 {
		t.Fatalf("support freezing: %d", code)
	}

	// Moderator freezes and suspends; can't ban or touch staff.
	if code, body := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+tid+"/status", map[string]any{"status": "frozen", "reason": "reported for scam"}); code != 204 {
		t.Fatalf("freeze: %d %v", code, body)
	}
	if code, body := do(t, target.c, "PUT", e.ts.URL+"/v1/watchlist/00000000-0000-0000-0000-000000000000", map[string]any{}); code != 403 || errCode(body) != "account_frozen" {
		t.Fatalf("frozen member writes: %d %v", code, body)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+tid+"/status", map[string]any{"status": "suspended", "reason": "repeat"}); code != 422 {
		t.Fatalf("suspend without date: %d", code)
	}
	until := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+tid+"/status", map[string]any{"status": "suspended", "until": until, "reason": "repeat offender"}); code != 204 {
		t.Fatalf("suspend: %d", code)
	}
	if code, _ := do(t, target.c, "GET", e.ts.URL+"/v1/me", nil); code != 401 {
		t.Fatalf("suspended member still signed in: %d", code)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+tid+"/status", map[string]any{"status": "banned", "reason": "fraud"}); code != 403 {
		t.Fatalf("moderator ban: %d", code)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/users/"+memberID(t, e, "support_sam")+"/status", map[string]any{"status": "frozen", "reason": "test"}); code != 403 {
		t.Fatalf("moderator restricting staff: %d", code)
	}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/users/"+tid+"/status", map[string]any{"status": "banned", "reason": "confirmed fraud"}); code != 204 {
		t.Fatalf("ban: %d", code)
	}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/users/"+memberID(t, e, "boss_users")+"/status", map[string]any{"status": "frozen", "reason": "self"}); code != 403 {
		t.Fatalf("self restrict: %d", code)
	}

	// Seller review: rejected sellers' listings are hidden.
	_, body = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/sellers?review=pending", nil)
	if len(body["sellers"].([]any)) != 1 {
		t.Fatalf("pending sellers: %v", body)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/sellers/"+sid+"/review", map[string]any{"decision": "reject"}); code != 422 {
		t.Fatalf("reject without note: %d", code)
	}
	do(t, mod.c, "POST", e.ts.URL+"/v1/admin/sellers/"+sid+"/review", map[string]any{"decision": "reject", "note": "counterfeit slabs"})
	if _, s := do(t, client(t), "GET", e.ts.URL+"/v1/listings?seller=sketchy_seller", nil); s["total"].(float64) != 0 {
		t.Fatalf("rejected seller still listed: %v", s)
	}
	do(t, mod.c, "POST", e.ts.URL+"/v1/admin/sellers/"+sid+"/review", map[string]any{"decision": "approve"})
	if _, s := do(t, client(t), "GET", e.ts.URL+"/v1/listings?seller=sketchy_seller", nil); s["total"].(float64) != 1 {
		t.Fatalf("approved seller hidden: %v", s)
	}

	// Notes.
	if code, _ := do(t, sup.c, "POST", e.ts.URL+"/v1/admin/users/"+sid+"/notes", map[string]any{"body": "Called about slab photos"}); code != 201 {
		t.Fatalf("note: %d", code)
	}

	// Staff management: Super Admin only; at least one Super Admin; no self-changes.
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/staff", map[string]any{"email": "trouble@example.com", "role": "support", "reason": "x"}); code != 403 {
		t.Fatalf("moderator managing staff: %d", code)
	}
	newbie := buyer(t, e, "new_helper")
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/staff", map[string]any{"email": "new_helper@example.com", "role": "support", "reason": "hired"}); code != 204 {
		t.Fatalf("add staff: %d %v", code, body)
	}
	if code, body := do(t, newbie.c, "GET", e.ts.URL+"/v1/admin/session", nil); code != 200 || body["mfa"] != "setup_required" {
		t.Fatalf("new staff session: %d %v", code, body)
	}
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/staff", map[string]any{"user_id": memberID(t, e, "boss_users"), "role": "", "reason": "leaving"}); code != 403 {
		t.Fatalf("self removal: %d", code)
	}
	// Moving mod to Support takes effect at their next authenticator check.
	do(t, boss.c, "POST", e.ts.URL+"/v1/admin/staff", map[string]any{"user_id": memberID(t, e, "mod_mia"), "role": "support", "reason": "role change"})
	if code, body := do(t, mod.c, "GET", e.ts.URL+"/v1/admin/sellers", nil); code != 403 || errCode(body) != "mfa_required" {
		t.Fatalf("after role change: %d %v", code, body)
	}
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/users/"+memberID(t, e, "mod_mia")+"/mfa-reset", map[string]any{"reason": "lost phone"}); code != 204 {
		t.Fatalf("mfa reset: %d %v", code, body)
	}

	// The member page shows the history from the audit log.
	_, body = do(t, boss.c, "GET", e.ts.URL+"/v1/admin/users/"+tid, nil)
	m = body["member"].(map[string]any)
	if m["status"] != "banned" || len(m["history"].([]any)) < 4 {
		t.Fatalf("member history: %v", m)
	}
	if _, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/audit/verify", nil); body["ok"] != true {
		t.Fatalf("chain: %v", body)
	}
}

func TestReportsAndModeration(t *testing.T) {
	e := setupEnv(t)
	mod := staff(t, e, "mod_reports", admin.Moderator)
	enrollMFA(t, e.ts.URL, mod.c)
	sup := staff(t, e, "sup_reports", admin.Support)
	enrollMFA(t, e.ts.URL, sup.c)
	sc, _ := newSeller(t, e, "fake_slabs", true)
	seller := &httpClientT{sc}
	fixed := createListing(t, e, seller, rawCard("FK1", 1500))["id"].(string)
	auc := createListing(t, e, seller, auctionCard("FK2", 100, nil))["id"].(string)
	a, b := buyer(t, e, "reporter_a"), buyer(t, e, "reporter_b")
	wantBid(t, e, a, auc, 500, true, 100)

	report := func(c *httpClientT, typ, id, reason string) int {
		code, _ := do(t, c.c, "POST", e.ts.URL+"/v1/reports", map[string]string{"target_type": typ, "target_id": id, "reason": reason, "details": "slab label looks fake"})
		return code
	}
	if code := report(a, "listing", fixed, "nonsense"); code != 422 {
		t.Fatalf("bad reason: %d", code)
	}
	if code := report(seller, "listing", fixed, "counterfeit"); code != 422 {
		t.Fatalf("own listing: %d", code)
	}
	for _, c := range []*httpClientT{a, a, b} { // a twice is a no-op
		if code := report(c, "listing", fixed, "counterfeit"); code != 201 {
			t.Fatalf("report: %d", code)
		}
	}
	if code := report(a, "user", memberID(t, e, "fake_slabs"), "off_platform"); code != 201 {
		t.Fatalf("report user: %d", code)
	}
	if code := report(a, "message", "999999", "abuse"); code != 404 {
		t.Fatalf("report a message you can't see: %d", code)
	}

	_, body := do(t, sup.c, "GET", e.ts.URL+"/v1/admin/reports", nil)
	rs := body["reports"].([]any)
	if len(rs) != 3 || rs[0].(map[string]any)["open_on_target"].(float64) != 2 {
		t.Fatalf("queue: %v", body)
	}
	first := rs[0].(map[string]any)["id"].(string)
	_, lst := do(t, sup.c, "GET", e.ts.URL+"/v1/admin/listings?reported=1", nil)
	if lst["total"].(float64) != 1 {
		t.Fatalf("reported listings: %v", lst)
	}

	// Support can't take listings down; a moderator can.
	if code, _ := do(t, sup.c, "POST", e.ts.URL+"/v1/admin/listings/"+fixed+"/moderate", map[string]any{"action": "remove", "reason": "fake"}); code != 403 {
		t.Fatalf("support moderating: %d", code)
	}
	if code, body := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/listings/"+fixed+"/moderate", map[string]any{"action": "remove", "reason": "counterfeit PSA label"}); code != 204 {
		t.Fatalf("remove: %d %v", code, body)
	}
	if code, _ := do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+fixed, nil); code != 404 {
		t.Fatalf("removed listing still public: %d", code)
	}
	if code, body := do(t, sc, "PATCH", e.ts.URL+"/v1/seller/listings/"+fixed, map[string]any{"status": "active"}); code != 409 || errCode(body) != "blocked" {
		t.Fatalf("seller re-activating: %d %v", code, body)
	}
	// Taking down a live auction ends it and tells the bidder.
	do(t, mod.c, "POST", e.ts.URL+"/v1/admin/listings/"+auc+"/moderate", map[string]any{"action": "remove", "reason": "counterfeit"})
	_, n := do(t, a.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if !hasTitle(n, "An auction you bid on was removed: Charizard Holo FK2") {
		t.Fatalf("bidder notice: %v", n)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/listings/"+auc+"/moderate", map[string]any{"action": "restore", "reason": "mistake"}); code != 422 {
		t.Fatalf("restore auction: %d", code)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/listings/"+fixed+"/moderate", map[string]any{"action": "restore", "reason": "seller proved it's real"}); code != 204 {
		t.Fatalf("restore: %d", code)
	}
	if code, _ := do(t, client(t), "GET", e.ts.URL+"/v1/listings/"+fixed, nil); code != 200 {
		t.Fatalf("restored listing: %d", code)
	}

	// Resolving one report with all=true closes both on that listing; reporters are thanked.
	if code, body := do(t, sup.c, "POST", e.ts.URL+"/v1/admin/reports/"+first+"/resolve", map[string]any{"status": "actioned", "note": "listing reviewed", "all": true}); code != 200 || body["closed"].(float64) != 2 {
		t.Fatalf("resolve: %d %v", code, body)
	}
	if code, _ := do(t, sup.c, "POST", e.ts.URL+"/v1/admin/reports/"+first+"/resolve", map[string]any{"status": "dismissed", "note": "again"}); code != 409 {
		t.Fatalf("resolve twice: %d", code)
	}
	_, n = do(t, b.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if !hasTitle(n, "Thanks for your report") {
		t.Fatalf("reporter thanked: %v", n)
	}
}

func TestCatalogAndFees(t *testing.T) {
	e := setupEnv(t)
	boss := staff(t, e, "boss_catalog", admin.SuperAdmin)
	enrollMFA(t, e.ts.URL, boss.c)
	mod := staff(t, e, "mod_catalog", admin.Moderator)
	enrollMFA(t, e.ts.URL, mod.c)
	sc, _ := newSeller(t, e, "catalog_seller", true)

	// Add a game with aliases; sellers can use it right away, by alias too.
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/catalog/categories", map[string]any{"kind": "tcg", "value": "Altered"}); code != 403 {
		t.Fatalf("moderator editing catalog: %d", code)
	}
	if code, body := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/catalog/categories",
		map[string]any{"kind": "tcg", "value": "Altered", "label": "Altered TCG", "aliases": []string{"altered tcg", "ALT"}}); code != 204 {
		t.Fatalf("add game: %d %v", code, body)
	}
	card := rawCard("ALT1", 500)
	card["game_or_sport"] = "alt"
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", card)
	if code != 201 || body["listing"].(map[string]any)["game_or_sport"] != "Altered" {
		t.Fatalf("list under new game: %d %v", code, body)
	}
	_, pub := do(t, client(t), "GET", e.ts.URL+"/v1/catalog", nil)
	found := false
	for _, c := range pub["categories"].([]any) {
		if c.(map[string]any)["value"] == "Altered" && c.(map[string]any)["label"] == "Altered TCG" {
			found = true
		}
	}
	if !found {
		t.Fatalf("public catalog: %v", pub["categories"])
	}
	_, cat := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/catalog", nil)
	var altID float64
	for _, c := range cat["categories"].([]any) {
		if m := c.(map[string]any); m["value"] == "Altered" {
			altID = m["id"].(float64)
			if m["listings"].(float64) != 1 {
				t.Fatalf("listing count: %v", m)
			}
		}
	}
	idStr := strconv.Itoa(int(altID))
	if code, _ := do(t, boss.c, "DELETE", e.ts.URL+"/v1/admin/catalog/categories/"+idStr, nil); code != 409 {
		t.Fatalf("delete in use: %d", code)
	}
	// Hiding it stops new listings but keeps the old one.
	do(t, boss.c, "PATCH", e.ts.URL+"/v1/admin/catalog/categories/"+idStr, map[string]any{"active": false})
	card["external_id"] = "ALT2"
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", card); code != 422 {
		t.Fatalf("list under hidden game: %d", code)
	}
	// New feature.
	if code, _ := do(t, boss.c, "PUT", e.ts.URL+"/v1/admin/catalog/features/gold_border", map[string]any{"label": "Gold border", "active": true, "sort": 20}); code != 204 {
		t.Fatalf("add feature: %d", code)
	}
	card = rawCard("GB1", 500)
	card["attributes"] = []string{"gold_border"}
	if code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", card); code != 201 {
		t.Fatalf("list with new feature: %d %v", code, body)
	}

	// Fees: schedule new fees for later; they show as upcoming; current stays.
	_, f := do(t, client(t), "GET", e.ts.URL+"/v1/fees", nil)
	if f["current"].(map[string]any)["flat_cents"].(float64) != 150 {
		t.Fatalf("current fees: %v", f)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if code, _ := do(t, boss.c, "POST", e.ts.URL+"/v1/admin/fees", map[string]any{"flat_cents": 100, "threshold_cents": 10000, "percent_bps": 250, "cap_cents": 2000, "effective_at": past}); code != 422 {
		t.Fatalf("fees in the past: %d", code)
	}
	later := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	code, body = do(t, boss.c, "POST", e.ts.URL+"/v1/admin/fees", map[string]any{"flat_cents": 100, "threshold_cents": 10000, "percent_bps": 250, "cap_cents": 2000, "effective_at": later, "note": "holiday promo"})
	if code != 201 {
		t.Fatalf("schedule fees: %d %v", code, body)
	}
	_, f = do(t, client(t), "GET", e.ts.URL+"/v1/fees", nil)
	if len(f["upcoming"].([]any)) != 1 || f["current"].(map[string]any)["flat_cents"].(float64) != 150 {
		t.Fatalf("upcoming fees: %v", f)
	}
	sid := strconv.Itoa(int(body["schedule"].(map[string]any)["id"].(float64)))
	if code, _ := do(t, boss.c, "DELETE", e.ts.URL+"/v1/admin/fees/"+sid, nil); code != 204 {
		t.Fatalf("cancel fees: %d", code)
	}
	if code, _ := do(t, boss.c, "PATCH", e.ts.URL+"/v1/admin/fees/upgrades/bold_title", map[string]any{"price_cents": 75, "active": true}); code != 204 {
		t.Fatalf("upgrade: %d", code)
	}
	_, af := do(t, mod.c, "GET", e.ts.URL+"/v1/admin/fees", nil)
	for _, u := range af["upgrades"].([]any) {
		if m := u.(map[string]any); m["key"] == "bold_title" && (m["price_cents"].(float64) != 75 || m["active"] != true) {
			t.Fatalf("upgrade saved: %v", m)
		}
	}
	if _, body := do(t, boss.c, "GET", e.ts.URL+"/v1/admin/audit?action=catalog.", nil); len(body["entries"].([]any)) < 3 {
		t.Fatalf("catalog audit: %v", body)
	}
}

// seedOrder inserts a paid order directly (checkout isn't built yet).
func seedOrder(t *testing.T, e *env, buyerName, sellerName, listingID string, cents int, late bool) (string, int64) {
	t.Helper()
	var id string
	var number int64
	shipBy := "now() + interval '3 days'"
	if late {
		shipBy = "now() - interval '1 day'"
	}
	err := e.db.QueryRow(`INSERT INTO orders (buyer_id, seller_id, status, subtotal_cents, total_cents, fee_cents, paid_at, ship_by, ship_to)
		VALUES ((SELECT id FROM users WHERE username = $1), (SELECT id FROM users WHERE username = $2), 'paid', $3, $3, 150, now(), `+shipBy+`,
		'{"name":"Pat Buyer","city":"Austin","state":"TX"}') RETURNING id, number`, buyerName, sellerName, cents).Scan(&id, &number)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO order_items (order_id, listing_id, source, title, unit_cents, quantity) VALUES ($1, $2, 'cart', 'Card', $3, 1)`,
		id, listingID, cents); err != nil {
		t.Fatal(err)
	}
	return id, number
}

func TestOrdersCasesFeedbackAndMessages(t *testing.T) {
	e := setupEnv(t)
	sup := staff(t, e, "sup_cases", admin.Support)
	enrollMFA(t, e.ts.URL, sup.c)
	aud := staff(t, e, "aud_cases", admin.Auditor)
	enrollMFA(t, e.ts.URL, aud.c)
	mod := staff(t, e, "mod_cases", admin.Moderator)
	enrollMFA(t, e.ts.URL, mod.c)
	sc, _ := newSeller(t, e, "order_seller", true)
	lid := createListing(t, e, &httpClientT{sc}, rawCard("OR1", 1800))["id"].(string)
	b := buyer(t, e, "order_buyer")
	oid, num := seedOrder(t, e, "order_buyer", "order_seller", lid, 4000, true)
	seedOrder(t, e, "order_buyer", "order_seller", lid, 1200, false)

	// Ledger, search by number, late shipments.
	_, body := do(t, aud.c, "GET", e.ts.URL+"/v1/admin/orders?party=order_seller", nil)
	if body["total"].(float64) != 2 {
		t.Fatalf("orders: %v", body)
	}
	_, body = do(t, aud.c, "GET", e.ts.URL+"/v1/admin/orders?shipping=late", nil)
	if body["total"].(float64) != 1 || body["orders"].([]any)[0].(map[string]any)["late"] != true {
		t.Fatalf("late: %v", body)
	}
	code, body := do(t, aud.c, "GET", e.ts.URL+"/v1/admin/orders/"+strconv.FormatInt(num, 10), nil)
	if code != 200 || len(body["order"].(map[string]any)["lines"].([]any)) != 1 {
		t.Fatalf("order detail: %d %v", code, body)
	}
	if code, _ := do(t, aud.c, "GET", e.ts.URL+"/v1/admin/cases", nil); code != 403 {
		t.Fatalf("auditor on cases: %d", code)
	}

	// A case: open, shared note reaches both sides, can't close without an outcome.
	code, body = do(t, sup.c, "POST", e.ts.URL+"/v1/admin/cases", map[string]any{"order": strconv.FormatInt(num, 10), "kind": "inr", "summary": "Buyer says the card never arrived"})
	if code != 201 {
		t.Fatalf("open case: %d %v", code, body)
	}
	cid := body["id"].(string)
	do(t, sup.c, "POST", e.ts.URL+"/v1/admin/cases/"+cid+"/notes", map[string]any{"body": "Seller, please share tracking.", "shared": true})
	do(t, sup.c, "POST", e.ts.URL+"/v1/admin/cases/"+cid+"/notes", map[string]any{"body": "Seller has 2 late shipments this month.", "shared": false})
	_, n := do(t, b.c, "GET", e.ts.URL+"/v1/notifications", nil)
	if !hasTitle(n, "Update on case 5001") && !hasTitle(n, "Update on case "+caseNumber(t, e, cid)) {
		t.Fatalf("buyer case update: %v", n)
	}
	if code, _ := do(t, sup.c, "PATCH", e.ts.URL+"/v1/admin/cases/"+cid, map[string]any{"status": "resolved_buyer"}); code != 422 {
		t.Fatalf("close without outcome: %d", code)
	}
	if code, body := do(t, sup.c, "PATCH", e.ts.URL+"/v1/admin/cases/"+cid, map[string]any{"status": "resolved_buyer", "outcome": "Seller refunded in full", "assigned_to": "me"}); code != 204 {
		t.Fatalf("close: %d %v", code, body)
	}
	_, cd := do(t, sup.c, "GET", e.ts.URL+"/v1/admin/cases/"+cid, nil)
	c := cd["case"].(map[string]any)
	if c["status"] != "resolved_buyer" || len(c["notes"].([]any)) != 2 || c["assigned_to"] != "sup_cases@example.com" {
		t.Fatalf("case detail: %v", c)
	}

	// Refund recorded by staff updates the order.
	if code, body := do(t, sup.c, "POST", e.ts.URL+"/v1/admin/refunds", map[string]any{"order": oid, "kind": "refund", "amount_cents": 4000, "stripe_ref": "re_123", "note": "seller refunded in Stripe"}); code != 204 {
		t.Fatalf("record refund: %d %v", code, body)
	}
	_, body = do(t, aud.c, "GET", e.ts.URL+"/v1/admin/orders/"+oid, nil)
	if o := body["order"].(map[string]any); o["status"] != "refunded" || len(o["refunds"].([]any)) != 1 {
		t.Fatalf("after refund: %v", o)
	}

	// Feedback moderation.
	var fid string
	e.db.QueryRow(`INSERT INTO feedback (order_id, from_user, to_user, role, rating, comment)
		VALUES ($1, (SELECT id FROM users WHERE username = 'order_buyer'), (SELECT id FROM users WHERE username = 'order_seller'),
		'buyer_to_seller', -1, 'Seller is a crook!!!') RETURNING id`, oid).Scan(&fid)
	_, body = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/feedback?filter=negative", nil)
	if len(body["feedback"].([]any)) != 1 {
		t.Fatalf("negative feedback: %v", body)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/feedback/"+fid+"/moderate", map[string]any{"action": "remove", "reason": "abusive, and the order was refunded"}); code != 204 {
		t.Fatalf("remove feedback: %d", code)
	}
	_, body = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/feedback?filter=removed", nil)
	if len(body["feedback"].([]any)) != 1 {
		t.Fatalf("removed feedback: %v", body)
	}

	// Message logs: list without text; reading needs a reason and is logged; blocked messages show.
	do(t, b.c, "POST", e.ts.URL+"/v1/listings/"+lid+"/messages", map[string]string{"body": "Is it centered?"})
	do(t, b.c, "POST", e.ts.URL+"/v1/listings/"+lid+"/messages", map[string]string{"body": "Venmo me and I'll pay now"})
	_, body = do(t, sup.c, "GET", e.ts.URL+"/v1/admin/messages?blocked=1&user=order_buyer", nil)
	ths := body["threads"].([]any)
	if len(ths) != 1 || ths[0].(map[string]any)["blocked"].(float64) != 1 {
		t.Fatalf("threads: %v", body)
	}
	tid := ths[0].(map[string]any)["id"].(string)
	if code, _ := do(t, sup.c, "POST", e.ts.URL+"/v1/admin/messages/"+tid+"/read", map[string]any{}); code != 422 {
		t.Fatalf("read without reason: %d", code)
	}
	code, body = do(t, sup.c, "POST", e.ts.URL+"/v1/admin/messages/"+tid+"/read", map[string]any{"reason": "case 5001 review"})
	if code != 200 || len(body["messages"].([]any)) != 2 {
		t.Fatalf("thread log: %d %v", code, body)
	}
	if _, au := do(t, aud.c, "GET", e.ts.URL+"/v1/admin/audit?action=messages.viewed&target_id="+tid, nil); len(au["entries"].([]any)) != 1 {
		t.Fatalf("view not audited: %v", au)
	}
}

func caseNumber(t *testing.T, e *env, id string) string {
	var n int64
	e.db.QueryRow(`SELECT number FROM cases WHERE id = $1`, id).Scan(&n)
	return strconv.FormatInt(n, 10)
}

func TestFraudAlertsAndKPIs(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	mod := staff(t, e, "mod_fraud", admin.Moderator)
	enrollMFA(t, e.ts.URL, mod.c)
	aud := staff(t, e, "aud_fraud", admin.Auditor)
	enrollMFA(t, e.ts.URL, aud.c)
	sc, _ := newSeller(t, e, "shill_seller", true)
	seller := &httpClientT{sc}
	scam := buyer(t, e, "scammer")
	lid := createListing(t, e, seller, rawCard("FR1", 1500))["id"].(string)

	// Off-site payment attempts.
	for i := 0; i < 3; i++ {
		do(t, scam.c, "POST", e.ts.URL+"/v1/listings/"+lid+"/messages", map[string]string{"body": fmt.Sprintf("pay me on venmo %d", i)})
	}
	// Sign-ins from three networks.
	sid := memberID(t, e, "scammer")
	for _, ip := range []string{"203.0.113.1", "198.51.100.7", "192.0.2.44"} {
		e.db.Exec(`INSERT INTO login_events (user_id, email, ip, success) VALUES ($1, 'scammer@example.com', $2, true)`, sid, ip)
	}
	// A bidder on the same network as the seller (everyone is 127.0.0.1 in tests).
	auc := createListing(t, e, seller, auctionCard("FR2", 100, nil))["id"].(string)
	wantBid(t, e, scam, auc, 300, true, 100)

	if code, _ := do(t, aud.c, "POST", e.ts.URL+"/v1/admin/fraud/run", map[string]any{}); code != 403 {
		t.Fatalf("auditor running checks: %d", code)
	}
	code, body := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/fraud/run", map[string]any{})
	if code != 200 {
		t.Fatalf("run: %d %v", code, body)
	}
	if body["new_alerts"].(float64) < 3 {
		t.Fatalf("alerts raised: %v", body)
	}
	_, body = do(t, mod.c, "POST", e.ts.URL+"/v1/admin/fraud/run", map[string]any{})
	if body["new_alerts"].(float64) != 0 {
		t.Fatalf("re-run should not duplicate: %v", body)
	}
	_, body = do(t, mod.c, "GET", e.ts.URL+"/v1/admin/fraud", nil)
	kinds := map[string]string{}
	for _, a := range body["alerts"].([]any) {
		m := a.(map[string]any)
		kinds[m["kind"].(string)] = m["id"].(string)
	}
	for _, k := range []string{"off_platform", "takeover_ips", "shill_shared_ip"} {
		if kinds[k] == "" {
			t.Fatalf("missing %s alert: %v", k, kinds)
		}
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/fraud/"+kinds["off_platform"]+"/review", map[string]any{"status": "reviewed", "note": "froze the account"}); code != 204 {
		t.Fatalf("review alert: %d", code)
	}
	if code, _ := do(t, mod.c, "POST", e.ts.URL+"/v1/admin/fraud/"+kinds["off_platform"]+"/review", map[string]any{"status": "dismissed", "note": "again"}); code != 409 {
		t.Fatalf("review twice: %d", code)
	}

	// KPIs: one paid order and some views.
	b := buyer(t, e, "kpi_buyer")
	_ = b
	seedOrder(t, e, "kpi_buyer", "shill_seller", lid, 1500, false)
	m := &admin.Metrics{DB: e.db}
	for i := 0; i < 4; i++ {
		m.CountView(ctx, lid)
	}
	_, k := do(t, aud.c, "GET", e.ts.URL+"/v1/admin/kpis?days=7", nil)
	if k["gmv_cents"].(float64) != 1500 || k["orders"].(float64) != 1 || k["views"].(float64) != 4 || k["conversion"].(float64) != 0.25 || len(k["series"].([]any)) != 8 {
		t.Fatalf("kpis: %v", k)
	}
}
