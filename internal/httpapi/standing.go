package httpapi

import (
	"errors"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/auth"
)

func (s *Server) standingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, admin.ErrAppealUsed):
		writeError(w, http.StatusConflict, "appeal_used", err.Error(), nil)
	case errors.Is(err, admin.ErrNothingToAppeal):
		writeError(w, http.StatusNotFound, "nothing_to_appeal", err.Error(), nil)
	default:
		s.adminError(w, r, err)
	}
}

// GET /v1/me/standing — the member's warnings, restrictions and appeals.
func (s *Server) handleMyStanding(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	v, err := s.standing.ForMember(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type appealRequest struct {
	About   string `json:"about"`
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

// POST /v1/me/appeals {"about": "warning", "ref": "<id>", "message": "..."}
func (s *Server) handleMyAppeal(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req appealRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	a, err := s.standing.SubmitAppeal(r.Context(), u.ID, req.About, req.Ref, req.Message)
	if err != nil {
		s.standingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"appeal": a})
}

// POST /v1/appeals/account {"email", "password", "message"} — for suspended
// and banned members, who can't sign in.
func (s *Server) handleAccountAppeal(w http.ResponseWriter, r *http.Request) {
	if !s.authLimiter.allow("login:" + clientIP(r, s.trustProxy)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in a few minutes", nil)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Message  string `json:"message"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.auth.DisabledAccount(r.Context(), req.Email, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect, or the account isn't suspended or banned", nil)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	about := "suspension"
	if u.Status == "banned" {
		about = "ban"
	}
	a, err := s.standing.SubmitAppeal(r.Context(), u.ID, about, "", req.Message)
	if err != nil {
		s.standingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"appeal": map[string]any{"status": a.Status, "about": a.About, "created_at": a.CreatedAt}})
}

// POST /v1/admin/users/{id}/warn {"reason"}
func (s *Server) handleAdminWarn(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	wid, err := s.standing.Warn(r.Context(), s.actor(r), id, req.Reason)
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"warning_id": wid})
}

// POST /v1/admin/users/{id}/tier {"tier": 1, "lock": true, "reason"}
func (s *Server) handleAdminTier(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Tier   int    `json:"tier"`
		Lock   bool   `json:"lock"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.standing.SetTier(r.Context(), s.actor(r), id, req.Tier, req.Lock, req.Reason); err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /v1/admin/appeals?status=open
func (s *Server) handleAdminAppeals(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	if st != "" && st != "open" && st != "upheld" && st != "overturned" {
		st = "open"
	}
	as, err := s.standing.Appeals(r.Context(), st)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"appeals": as})
}

// POST /v1/admin/appeals/{id} {"decision": "upheld"|"overturned", "note"}
func (s *Server) handleAdminDecideAppeal(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.standing.Decide(r.Context(), s.actor(r), id, req.Decision, req.Note); err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
