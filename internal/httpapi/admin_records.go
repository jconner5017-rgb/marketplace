package httpapi

import (
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
)

// GET /v1/admin/orders?q=&status=&party=&shipping=late|awaiting|in_transit|exception|delivered
func (s *Server) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := pageParams(r)
	rows, total, err := s.records.Orders(r.Context(), admin.OrderQuery{Q: q.Get("q"), Status: q.Get("status"),
		Party: q.Get("party"), Shipping: q.Get("shipping"), Limit: limit, Offset: offset})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": rows, "total": total, "limit": limit, "offset": offset})
}

// GET /v1/admin/orders/{id} — by ID or order number.
func (s *Server) handleAdminOrder(w http.ResponseWriter, r *http.Request) {
	o, err := s.records.Order(r.Context(), r.PathValue("id"))
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": o})
}

// GET /v1/admin/cases?status=open|...&mine=1
func (s *Server) handleAdminCases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = "open"
	}
	if status == "all" {
		status = ""
	}
	cq := admin.CaseQuery{Status: status}
	if q.Get("mine") == "1" {
		u, _ := currentUser(r)
		cq.Assigned = u.ID
	}
	cs, err := s.records.Cases(r.Context(), cq)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": cs})
}

// GET /v1/admin/cases/{id}
func (s *Server) handleAdminCase(w http.ResponseWriter, r *http.Request) {
	c, err := s.records.Case(r.Context(), r.PathValue("id"))
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"case": c})
}

// POST /v1/admin/cases {"order","kind","summary"}
func (s *Server) handleAdminOpenCase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Order   string `json:"order"`
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	id, err := s.records.OpenCase(r.Context(), s.actor(r), req.Order, req.Kind, req.Summary)
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// PATCH /v1/admin/cases/{id} {"status","assigned_to","outcome","reason"}
func (s *Server) handleAdminUpdateCase(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var u admin.CaseUpdate
	if !decodeJSON(w, r, &u) {
		return
	}
	if u.Assign != nil && *u.Assign == "me" {
		me, _ := currentUser(r)
		u.Assign = &me.ID
	}
	if err := s.records.UpdateCase(r.Context(), s.actor(r), id, u); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/admin/cases/{id}/notes {"body","shared":true}
func (s *Server) handleAdminCaseNote(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Body   string `json:"body"`
		Shared bool   `json:"shared"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.records.AddCaseNote(r.Context(), s.actor(r), id, req.Body, !req.Shared); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/admin/refunds?kind=refund|chargebacks|...
func (s *Server) handleAdminRefunds(w http.ResponseWriter, r *http.Request) {
	rs, err := s.records.Refunds(r.Context(), "", r.URL.Query().Get("kind"), 200)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refunds": rs})
}

// POST /v1/admin/refunds {"order","kind","amount_cents","stripe_ref","note"}
func (s *Server) handleAdminRecordRefund(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Order       string `json:"order"`
		Kind        string `json:"kind"`
		AmountCents int    `json:"amount_cents"`
		StripeRef   string `json:"stripe_ref"`
		Note        string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.records.RecordRefund(r.Context(), s.actor(r), req.Order, req.Kind, req.AmountCents, req.StripeRef, req.Note); err != nil {
		if err == admin.ErrReason {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error(), map[string]string{"note": err.Error()})
			return
		}
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/admin/feedback?filter=negative|removed|reported&user=
func (s *Server) handleAdminFeedback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fs, err := s.records.Feedback(r.Context(), admin.FeedbackQuery{Filter: q.Get("filter"), User: q.Get("user")})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": fs})
}

// POST /v1/admin/feedback/{id}/moderate {"action":"remove"|"restore","reason"}
func (s *Server) handleAdminModerateFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.records.ModerateFeedback(r.Context(), s.actor(r), id, req.Action, req.Reason); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/admin/messages?user=&listing=&blocked=1 — conversations, without their text.
func (s *Server) handleAdminThreads(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ts, err := s.records.Threads(r.Context(), admin.ThreadQuery{User: q.Get("user"), Listing: q.Get("listing"), BlockedOnly: q.Get("blocked") == "1"})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": ts})
}

// POST /v1/admin/messages/{id}/read {"reason"} — the full conversation, logged.
func (s *Server) handleAdminThreadLog(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req reasonRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	t, msgs, err := s.records.ThreadLog(r.Context(), s.actor(r), id, req.Reason)
	if err != nil {
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": t, "messages": msgs})
}
