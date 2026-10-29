package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

// GET /v1/admin/ip-rules — the admin IP allowlist and the caller's address.
func (s *Server) handleIPRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.ipRules.List(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules, "your_ip": clientIP(r, s.trustProxy),
		"enforced": !s.ipRules.Disabled && len(rules) > 0, "switched_off": s.ipRules.Disabled})
}

// POST /v1/admin/ip-rules {"cidr": "203.0.113.0/24", "label": "Office"}
func (s *Server) handleAddIPRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CIDR  string `json:"cidr"`
		Label string `json:"label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	rule, err := s.ipRules.Add(r.Context(), s.actor(r), req.CIDR, req.Label)
	if errors.Is(err, admin.ErrLockout) {
		writeError(w, http.StatusConflict, "lockout", err.Error(), nil)
		return
	}
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
}

// DELETE /v1/admin/ip-rules/{id}
func (s *Server) handleRemoveIPRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "rule not found", nil)
		return
	}
	err = s.ipRules.Remove(r.Context(), s.actor(r), id)
	if errors.Is(err, admin.ErrLockout) {
		writeError(w, http.StatusConflict, "lockout", err.Error(), nil)
		return
	}
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
