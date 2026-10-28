package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

func (s *Server) actor(r *http.Request) admin.Actor {
	u, _ := currentUser(r)
	return admin.Actor{ID: u.ID, Email: u.Email, Role: u.Staff(), IP: clientIP(r, s.trustProxy)}
}

func (s *Server) adminError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *admin.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, admin.ErrReason), errors.Is(err, admin.ErrSuspendUntil), errors.Is(err, admin.ErrBadStatus):
		field := "reason"
		if errors.Is(err, admin.ErrSuspendUntil) {
			field = "until"
		}
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error(), map[string]string{field: err.Error()})
	case errors.Is(err, admin.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, admin.ErrSelf), errors.Is(err, admin.ErrProtected), errors.Is(err, admin.ErrForbidden),
		errors.Is(err, admin.ErrOwner):
		writeError(w, http.StatusForbidden, "forbidden", err.Error(), nil)
	case errors.Is(err, admin.ErrLastSuper), errors.Is(err, admin.ErrTaken):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	default:
		s.internalError(w, r, err)
	}
}

func uuidParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return "", false
	}
	return id, true
}

// GET /v1/admin/users?q=&status=&staff=1&sellers=1&limit=&offset=
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := pageParams(r)
	rows, total, err := s.users.Search(r.Context(), admin.MemberQuery{
		Q: q.Get("q"), Status: q.Get("status"), Staff: q.Get("staff") == "1", Sellers: q.Get("sellers") == "1",
		Limit: limit, Offset: offset,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": rows, "total": total, "limit": limit, "offset": offset})
}

// GET /v1/admin/users/{id}
func (s *Server) handleAdminUser(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	m, err := s.users.Get(r.Context(), id)
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": m})
}

// PATCH /v1/admin/users/{id} {"email","username","email_verified","reason"}
func (s *Server) handleAdminEditUser(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var p admin.ProfileChange
	if !decodeJSON(w, r, &p) {
		return
	}
	if err := s.users.UpdateProfile(r.Context(), s.actor(r), id, p); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/admin/users/{id}/status {"status":"suspended","until":"2026-11-30T00:00:00Z","reason":"..."}
func (s *Server) handleAdminUserStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var c admin.StatusChange
	if !decodeJSON(w, r, &c) {
		return
	}
	if err := s.users.SetStatus(r.Context(), s.actor(r), id, c); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

// POST /v1/admin/users/{id}/sign-out {"reason"}
func (s *Server) handleAdminSignOut(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req reasonRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.users.SignOutEverywhere(r.Context(), s.actor(r), id, req.Reason); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/admin/users/{id}/notes {"body"}
func (s *Server) handleAdminNote(w http.ResponseWriter, r *http.Request) {
	a := s.actor(r)
	if !admin.Can(a.Role, admin.UsersEdit) && !admin.Can(a.Role, admin.UsersRestrict) {
		writeError(w, http.StatusForbidden, "forbidden", "your staff role can't add notes", nil)
		return
	}
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	n, err := s.users.AddNote(r.Context(), a, id, req.Body)
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"note": n})
}

// POST /v1/admin/users/{id}/mfa-reset {"reason"}
func (s *Server) handleAdminMFAReset(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req reasonRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.users.ResetMFA(r.Context(), s.actor(r), id, req.Reason); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/admin/staff
func (s *Server) handleAdminStaff(w http.ResponseWriter, r *http.Request) {
	rows, _, err := s.users.Search(r.Context(), admin.MemberQuery{Staff: true, Limit: 100})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": rows, "roles": admin.Roles})
}

// POST /v1/admin/staff {"email" or "user_id", "role" ("" removes), "reason"}
func (s *Server) handleAdminSetStaff(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email  string `json:"email"`
		UserID string `json:"user_id"`
		Role   string `json:"role"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	id := req.UserID
	if id == "" {
		var err error
		if id, err = s.users.IDByEmail(r.Context(), req.Email); err != nil {
			if errors.Is(err, admin.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "no member has that email; they need to register first",
					map[string]string{"email": "no member with this email"})
				return
			}
			s.internalError(w, r, err)
			return
		}
	} else if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "member not found", nil)
		return
	}
	if err := s.users.SetStaffRole(r.Context(), s.actor(r), id, req.Role, req.Reason); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/admin/sellers?review=pending|approved|rejected
func (s *Server) handleAdminSellers(w http.ResponseWriter, r *http.Request) {
	review := r.URL.Query().Get("review")
	switch review {
	case "", "pending", "approved", "rejected":
	default:
		review = ""
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := s.users.Sellers(r.Context(), review, limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sellers": rows})
}

// POST /v1/admin/sellers/{id}/review {"decision":"approve"|"reject"|"pending","note"}
func (s *Server) handleAdminReviewSeller(w http.ResponseWriter, r *http.Request) {
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
	if err := s.users.ReviewSeller(r.Context(), s.actor(r), id, req.Decision, req.Note); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
