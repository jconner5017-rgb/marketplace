package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/email"
)

// mustConfirm reports whether a member has to confirm their email before
// acting: only new accounts, and only once mail is actually being sent.
func (s *Server) mustConfirm(u auth.User) bool {
	return s.mail.Enabled() && u.VerifyRequired && !u.EmailVerified
}

// verified wraps actions that reach other members or move money: buying,
// bidding, offers, messages, and becoming a seller.
func (s *Server) verified(next http.HandlerFunc) http.HandlerFunc {
	return requireUser(func(w http.ResponseWriter, r *http.Request) {
		if u, _ := currentUser(r); s.mustConfirm(u) {
			writeError(w, http.StatusForbidden, "email_unconfirmed",
				"confirm your email first: open the link we sent to "+u.Email+", or send a new one from the banner at the top of the page", nil)
			return
		}
		next(w, r)
	})
}

func (s *Server) sendConfirm(r *http.Request, u auth.User) error {
	if !s.mail.Enabled() {
		return nil
	}
	ctx := r.Context()
	t, err := s.mail.NewToken(ctx, s.db, u.ID, u.Email, "verify", 48*time.Hour)
	if err != nil {
		return err
	}
	link := s.mail.WebURL + "/verify-email?token=" + t
	return s.mail.Queue(ctx, s.db, u.ID, u.Email, "Confirm your email for Slabs & Singles",
		"Hi "+u.Username+",\n\nConfirm your email to start buying and selling on Slabs & Singles:\n\n"+link+
			"\n\nThe link works for 48 hours. If you didn't sign up, ignore this email.\n\nSlabs & Singles")
}

// POST /v1/me/confirm-email — send a new confirmation link.
func (s *Server) handleResendConfirm(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if u.EmailVerified {
		writeJSON(w, http.StatusOK, map[string]any{"sent": false, "already": true})
		return
	}
	if !s.mail.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "mail_off", "email isn't set up yet", nil)
		return
	}
	if n, err := s.mail.RecentTokens(r.Context(), u.ID, "verify", time.Hour); err != nil {
		s.internalError(w, r, err)
		return
	} else if n >= 5 {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "we've sent several links already; check your spam folder, or try again in an hour", nil)
		return
	}
	if err := s.sendConfirm(r, u); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "email": u.Email})
}

// POST /v1/auth/confirm-email {"token": "..."} — from the emailed link.
func (s *Server) handleConfirmEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &req) || s.mail == nil {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	uid, addr, err := s.mail.UseToken(r.Context(), tx, strings.TrimSpace(req.Token), "verify")
	if errors.Is(err, email.ErrToken) {
		writeError(w, http.StatusGone, "link_expired", "that link has expired or was already used; sign in and send a new one", nil)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ok, err := auth.MarkEmailVerified(r.Context(), tx, uid, addr)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusGone, "link_expired", "this link was for an email address that's since been changed", nil)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"confirmed": true})
}

// POST /v1/auth/forgot {"email": "..."} — always answers the same way, so
// it can't be used to find out who has an account.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !s.authLimiter.allow("forgot:" + clientIP(r, s.trustProxy)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in a few minutes", nil)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.mail.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "mail_off", "password reset by email isn't available yet; contact support", nil)
		return
	}
	u, found, err := s.auth.ResetTarget(r.Context(), req.Email)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if found {
		if n, err := s.mail.RecentTokens(r.Context(), u.ID, "reset", time.Hour); err == nil && n < 5 {
			t, err := s.mail.NewToken(r.Context(), s.db, u.ID, u.Email, "reset", time.Hour)
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			link := s.mail.WebURL + "/reset-password?token=" + t
			if err := s.mail.Queue(r.Context(), s.db, u.ID, u.Email, "Reset your Slabs & Singles password",
				"Hi "+u.Username+",\n\nSomeone (hopefully you) asked to reset your password. Choose a new one here:\n\n"+link+
					"\n\nThe link works for one hour. If you didn't ask, ignore this email; your password stays the same.\n\nSlabs & Singles"); err != nil {
				s.internalError(w, r, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

// POST /v1/auth/reset {"token": "...", "password": "..."}
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if !s.authLimiter.allow("reset:" + clientIP(r, s.trustProxy)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in a few minutes", nil)
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) || s.mail == nil {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	uid, addr, err := s.mail.UseToken(r.Context(), tx, strings.TrimSpace(req.Token), "reset")
	if errors.Is(err, email.ErrToken) {
		writeError(w, http.StatusGone, "link_expired", "that link has expired or was already used; ask for a new one", nil)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var verr *auth.ValidationError
	if err := auth.SetPassword(r.Context(), tx, uid, req.Password); errors.As(err, &verr) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "check the highlighted fields", verr.Fields)
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Opening the link proves they own the address.
	if _, err := auth.MarkEmailVerified(r.Context(), tx, uid, addr); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reset": true})
}

// POST /v1/email/unsubscribe?u=...&t=... (also the one-click header link)
func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if s.mail == nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	q := r.URL.Query()
	if err := s.mail.Unsubscribe(r.Context(), q.Get("u"), q.Get("t")); errors.Is(err, email.ErrToken) {
		writeError(w, http.StatusBadRequest, "bad_link", "that unsubscribe link isn't valid", nil)
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unsubscribed": true})
}

// PUT /v1/me/email-prefs {"announcements": bool}
func (s *Server) handleEmailPrefs(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Announcements *bool `json:"announcements"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Announcements != nil {
		if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET email_opt_out = $2 WHERE id = $1`, u.ID, !*req.Announcements); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	s.handleGetEmailPrefs(w, r)
}

// GET /v1/me/email-prefs
func (s *Server) handleGetEmailPrefs(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var optOut bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT email_opt_out FROM users WHERE id = $1`, u.ID).Scan(&optOut); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"announcements": !optOut, "email": u.Email, "confirmed": u.EmailVerified})
}

// ---- Staff email blasts ----

func (s *Server) mailError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *email.FieldError
	if errors.As(err, &fe) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
		return
	}
	s.internalError(w, r, err)
}

// GET /v1/admin/email — blasts so far, audience sizes and whether mail is on.
func (s *Server) handleAdminEmail(w http.ResponseWriter, r *http.Request) {
	if s.mail == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "broadcasts": []any{}, "audiences": map[string]int{}})
		return
	}
	bs, err := s.mail.Broadcasts(r.Context(), 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	counts := map[string]int{}
	for k := range email.Audiences {
		n, err := s.mail.CountRecipients(r.Context(), k)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		counts[k] = n
	}
	var queued int
	s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM email_outbox WHERE status = 'queued'`).Scan(&queued)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": s.mail.Enabled(), "postal": s.mail.Postal != "", "broadcasts": bs,
		"audiences": counts, "queued": queued})
}

// POST /v1/admin/email {"subject", "body", "audience", "test": bool}
func (s *Server) handleAdminSendEmail(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		Audience string `json:"audience"`
		Test     bool   `json:"test"`
	}
	if !decodeJSON(w, r, &req) || s.mail == nil {
		return
	}
	req.Subject, req.Body = strings.TrimSpace(req.Subject), strings.TrimSpace(req.Body)
	if req.Test {
		if err := s.mail.TestBlast(r.Context(), u.ID, u.Email, req.Subject, req.Body); err != nil {
			s.mailError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"test": true, "to": u.Email})
		return
	}
	id, n, err := s.mail.SendBlast(r.Context(), u.ID, req.Subject, req.Body, req.Audience)
	if err != nil {
		s.mailError(w, r, err)
		return
	}
	if err := s.audit(r, "email.broadcast", "email_broadcast", strconv.FormatInt(id, 10), "", map[string]any{"subject": req.Subject, "audience": req.Audience, "recipients": n}); err != nil {
		s.log.Error("audit email blast", "err", err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "recipients": n})
}
