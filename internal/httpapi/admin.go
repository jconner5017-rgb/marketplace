package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/auth"
)

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// staffOnly allows signed-in staff, before the authenticator check (used by
// the sign-in check itself).
func (s *Server) staffOnly(next http.HandlerFunc) http.HandlerFunc {
	return requireUser(func(w http.ResponseWriter, r *http.Request) {
		u, _ := currentUser(r)
		if !u.IsStaff() {
			writeError(w, http.StatusForbidden, "forbidden", "staff only", nil)
			return
		}
		next(w, r)
	})
}

// requireStaff allows staff whose role has perm and who passed the
// authenticator check within the last 12 hours. perm "" = any staff.
func (s *Server) requireStaff(perm string, next http.HandlerFunc) http.HandlerFunc {
	return s.staffOnly(func(w http.ResponseWriter, r *http.Request) {
		u, _ := currentUser(r)
		at, ok := s.auth.SessionMFA(r.Context(), sessionToken(r))
		if !ok || time.Since(at) > admin.StaffSessionFor {
			writeError(w, http.StatusForbidden, "mfa_required", "enter your authenticator code to continue", nil)
			return
		}
		if perm != "" && !admin.Can(u.Staff(), perm) {
			writeError(w, http.StatusForbidden, "forbidden", "your staff role can't do that", nil)
			return
		}
		next(w, r)
	})
}

// audit writes an audit entry for the current staff member (outside a transaction).
func (s *Server) audit(r *http.Request, action, targetType, targetID, reason string, details map[string]any) error {
	u, _ := currentUser(r)
	return s.auditLog.Log(r.Context(), nil, s.entry(r, u, action, targetType, targetID, reason, details))
}

func (s *Server) entry(r *http.Request, u auth.User, action, targetType, targetID, reason string, details map[string]any) admin.Entry {
	return admin.Entry{ActorID: u.ID, ActorEmail: u.Email, ActorRole: u.Staff(), Action: action,
		TargetType: targetType, TargetID: targetID, Reason: reason, Details: details, IP: clientIP(r, s.trustProxy)}
}

// frozenGuard stops frozen accounts from changing anything except signing out.
func frozenGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := currentUser(r); ok && u.Frozen() && r.Method != http.MethodGet && r.Method != http.MethodHead {
			switch r.URL.Path {
			case "/v1/auth/logout", "/v1/notifications/read-all":
			default:
				writeError(w, http.StatusForbidden, "account_frozen",
					"your account is frozen while we review it; you can browse but not buy, sell, bid or message", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// GET /v1/admin/session — the staff member's role, permissions and sign-in check state.
func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	status, err := s.mfa.Status(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	mfa := status
	var until *time.Time
	if status == "ready" {
		mfa = "verify_required"
		if at, ok := s.auth.SessionMFA(r.Context(), sessionToken(r)); ok && time.Since(at) < admin.StaffSessionFor {
			mfa = "ok"
			t := at.Add(admin.StaffSessionFor)
			until = &t
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": u, "staff_role": u.Staff(), "permissions": admin.Permissions(u.Staff()),
		"mfa": mfa, "mfa_until": until, "mfa_configured": len(s.mfa.Key) == 32,
	})
}

func (s *Server) mfaError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, admin.ErrMFANotConfigured):
		writeError(w, http.StatusServiceUnavailable, "mfa_not_configured", err.Error(), nil)
	case errors.Is(err, admin.ErrMFAEnrolled), errors.Is(err, admin.ErrMFANotEnrolled):
		writeError(w, http.StatusConflict, "mfa_state", err.Error(), nil)
	case errors.Is(err, admin.ErrBadCode):
		writeError(w, http.StatusUnprocessableEntity, "bad_code", err.Error(), map[string]string{"code": err.Error()})
	case errors.Is(err, admin.ErrMFALocked):
		writeError(w, http.StatusTooManyRequests, "mfa_locked", err.Error(), nil)
	default:
		s.internalError(w, r, err)
	}
}

// POST /v1/admin/mfa/setup — a new authenticator secret to scan.
func (s *Server) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	secret, link, err := s.mfa.Setup(r.Context(), u.ID, u.Email)
	if err != nil {
		s.mfaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": secret, "otpauth_url": link})
}

type codeRequest struct {
	Code string `json:"code"`
}

// POST /v1/admin/mfa/confirm {"code": "123456"} — finish setup; returns backup codes once.
func (s *Server) handleMFAConfirm(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req codeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.authLimiter.allow("mfa:" + u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, wait a few minutes", nil)
		return
	}
	codes, err := s.mfa.Confirm(r.Context(), u.ID, req.Code)
	if err != nil {
		s.mfaError(w, r, err)
		return
	}
	if err := s.auth.MarkSessionMFA(r.Context(), sessionToken(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.audit(r, "staff.mfa_enrolled", "user", u.ID, "", nil); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

// POST /v1/admin/mfa/verify {"code": "123456"} — the authenticator (or a backup code) check.
func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req codeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.authLimiter.allow("mfa:" + u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, wait a few minutes", nil)
		return
	}
	res, err := s.mfa.Verify(r.Context(), u.ID, req.Code)
	if err != nil {
		switch {
		case res.JustLocked:
			s.audit(r, "staff.mfa_locked", "user", u.ID, "", map[string]any{"failures": admin.MaxCodeFailures})
		case errors.Is(err, admin.ErrBadCode):
			s.audit(r, "staff.mfa_failed", "user", u.ID, "", nil)
		}
		s.mfaError(w, r, err)
		return
	}
	if err := s.auth.MarkSessionMFA(r.Context(), sessionToken(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	details := map[string]any{}
	if res.UsedBackup {
		details["backup_code"] = true
		details["backup_codes_left"] = res.BackupsLeft
	}
	if err := s.audit(r, "staff.sign_in", "user", u.ID, "", details); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "used_backup": res.UsedBackup, "backup_codes_left": res.BackupsLeft})
}

// GET /v1/admin/dashboard — headline numbers and open queues.
func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type stat struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Value int64  `json:"value"`
		Money bool   `json:"money,omitempty"`
	}
	queries := []struct {
		key, label, sql string
		money           bool
	}{
		{"users", "Members", `SELECT count(*) FROM users`, false},
		{"new_users_7d", "New members, 7 days", `SELECT count(*) FROM users WHERE created_at > now() - interval '7 days'`, false},
		{"active_30d", "Signed in, 30 days", `SELECT count(DISTINCT user_id) FROM login_events WHERE success AND created_at > now() - interval '30 days'`, false},
		{"sellers", "Sellers taking payments", `SELECT count(*) FROM seller_profiles WHERE charges_enabled`, false},
		{"listings", "Active listings", `SELECT count(*) FROM listings WHERE status = 'active' AND quantity > 0`, false},
		{"auctions", "Live auctions", `SELECT count(*) FROM listings WHERE auction_state = 'live' AND status = 'active'`, false},
		{"bids_24h", "Bids, 24 hours", `SELECT count(*) FROM bids WHERE created_at > now() - interval '1 day'`, false},
		{"offers_open", "Offers waiting", `SELECT count(*) FROM offers WHERE status = 'pending'`, false},
		{"holds", "Awaiting payment", `SELECT count(*) FROM holds WHERE status = 'awaiting_payment'`, false},
		{"holds_cents", "Value awaiting payment", `SELECT coalesce(sum(unit_cents * quantity), 0) FROM holds WHERE status = 'awaiting_payment'`, true},
		{"failed_logins_24h", "Failed sign-ins, 24 hours", `SELECT count(*) FROM login_events WHERE NOT success AND created_at > now() - interval '1 day'`, false},
		{"blocked_msgs_7d", "Blocked messages, 7 days", `SELECT count(*) FROM messages WHERE blocked IS NOT NULL AND created_at > now() - interval '7 days'`, false},
		{"restricted", "Frozen, suspended or banned", `SELECT count(*) FROM users WHERE status <> 'active'`, false},
		{"reports_open", "Open reports", `SELECT count(*) FROM reports WHERE status = 'open'`, false},
		{"sellers_pending", "Sellers waiting for review", `SELECT count(*) FROM seller_profiles WHERE review_status = 'pending' AND charges_enabled`, false},
		{"listings_blocked", "Listings taken down", `SELECT count(*) FROM listings WHERE status = 'blocked'`, false},
	}
	out := []stat{}
	for _, q := range queries {
		var v int64
		if err := s.db.QueryRowContext(ctx, q.sql).Scan(&v); err != nil {
			s.internalError(w, r, err)
			return
		}
		out = append(out, stat{q.key, q.label, v, q.money})
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": out})
}

// GET /v1/admin/audit?actor=&action=&target_type=&target_id=&before=
func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	f := admin.Filter{ActorID: q.Get("actor"), Action: strings.TrimSpace(q.Get("action")),
		TargetType: q.Get("target_type"), TargetID: q.Get("target_id"), BeforeID: before, Limit: limit}
	if f.ActorID != "" && !isUUID(f.ActorID) {
		f.ActorID = "00000000-0000-0000-0000-000000000000"
	}
	es, err := s.auditLog.List(r.Context(), f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": es})
}

// GET /v1/admin/audit/verify — recompute the hash chain.
func (s *Server) handleAuditVerify(w http.ResponseWriter, r *http.Request) {
	res, err := s.auditLog.Verify(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.audit(r, "audit.verified", "", "", "", map[string]any{"checked": res.Checked, "ok": res.OK})
	writeJSON(w, http.StatusOK, res)
}
