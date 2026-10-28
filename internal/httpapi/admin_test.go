package httpapi

import (
	"context"
	"encoding/base32"
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
