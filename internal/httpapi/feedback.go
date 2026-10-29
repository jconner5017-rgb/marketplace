package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jconner5017-rgb/marketplace/internal/feedback"
)

func (s *Server) feedbackError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *feedback.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, feedback.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, feedback.ErrResolveFirst):
		writeError(w, http.StatusConflict, "resolve_first", err.Error(), nil)
	case errors.Is(err, feedback.ErrNotYet), errors.Is(err, feedback.ErrClosed), errors.Is(err, feedback.ErrAlreadyLeft),
		errors.Is(err, feedback.ErrCantReply):
		writeError(w, http.StatusConflict, "cannot_rate", err.Error(), nil)
	case errors.Is(err, feedback.ErrNoRelation):
		writeError(w, http.StatusForbidden, "no_relation", err.Error(), nil)
	default:
		s.internalError(w, r, err)
	}
}

// POST /v1/orders/{id}/feedback {"rating", "comment", "item_rating", "shipping_rating", "communication_rating"}
func (s *Server) handleOrderFeedback(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var in feedback.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := s.feedback.Leave(r.Context(), u.ID, id, in)
	if err != nil {
		s.feedbackError(w, r, err)
		return
	}
	o, err := s.checkout.Order(r.Context(), u.ID, id)
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": o, "result": res})
}

// POST /v1/feedback/{id}/reply {"reply"} — a seller's one public answer.
func (s *Server) handleFeedbackReply(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Reply string `json:"reply"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.feedback.Reply(r.Context(), u.ID, id, req.Reply); err != nil {
		s.feedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) memberID(r *http.Request) (string, bool) {
	var id string
	err := s.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE lower(username) = lower($1) AND status <> 'banned'`,
		r.PathValue("username")).Scan(&id)
	return id, err == nil
}

// GET /v1/members/{username}/feedback?role=seller|buyer&filter=positive|neutral|negative
func (s *Server) handleMemberFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := s.memberID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "member not found", nil)
		return
	}
	role := r.URL.Query().Get("role")
	if role != "buyer" {
		role = "seller"
	}
	sum, err := s.feedback.SummaryFor(r.Context(), id, role)
	if err != nil {
		s.feedbackError(w, r, err)
		return
	}
	viewer := ""
	if u, ok := currentUser(r); ok {
		viewer = u.ID
	}
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 200 {
		limit = n
	}
	entries, err := s.feedback.Received(r.Context(), id, role, r.URL.Query().Get("filter"), viewer, limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": sum, "feedback": entries})
}

// GET /v1/members/{username}/buyer-record — for sellers dealing with this buyer.
func (s *Server) handleBuyerRecord(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	rel, err := s.feedback.BuyerReliability(r.Context(), u.ID, r.PathValue("username"))
	if err != nil {
		s.feedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buyer": rel})
}

// GET /v1/seller/feedback — automatic feedback settings and outside-feedback claims.
func (s *Server) handleSellerFeedback(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var mode, comment string
	if err := s.db.QueryRowContext(r.Context(), `SELECT auto_feedback, auto_feedback_comment FROM seller_profiles WHERE user_id = $1`,
		u.ID).Scan(&mode, &comment); err != nil {
		writeError(w, http.StatusNotFound, "not_seller", "set up selling first", nil)
		return
	}
	claims, err := s.feedback.MyClaims(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auto_feedback": mode, "auto_feedback_comment": comment, "outside": claims,
		"platforms": feedback.Platforms})
}

// PUT /v1/seller/feedback {"auto_feedback": "off"|"paid"|"delivered", "auto_feedback_comment"}
func (s *Server) handleSetSellerFeedback(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Mode    string `json:"auto_feedback"`
		Comment string `json:"auto_feedback_comment"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Mode != "off" && req.Mode != "paid" && req.Mode != "delivered" {
		writeError(w, http.StatusUnprocessableEntity, "invalid", "choose off, when paid, or when delivered", map[string]string{"auto_feedback": "invalid"})
		return
	}
	if len([]rune(req.Comment)) < 3 || len([]rune(req.Comment)) > 200 || feedback.Extortion(req.Comment) {
		writeError(w, http.StatusUnprocessableEntity, "invalid", "write a short comment (3 to 200 characters)", map[string]string{"auto_feedback_comment": "3 to 200 characters"})
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE seller_profiles SET auto_feedback = $2, auto_feedback_comment = $3, updated_at = now()
		WHERE user_id = $1`, u.ID, req.Mode, req.Comment)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not_seller", "set up selling first", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /v1/seller/outside-feedback {"platform", "profile_url", "feedback_count", "positive_percent"}
func (s *Server) handleClaimOutside(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if _, err := s.sellers.Get(r.Context(), u.ID); err != nil {
		writeError(w, http.StatusNotFound, "not_seller", "set up selling first", nil)
		return
	}
	var req struct {
		Platform string  `json:"platform"`
		URL      string  `json:"profile_url"`
		Count    int     `json:"feedback_count"`
		Percent  float64 `json:"positive_percent"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := s.feedback.ClaimOutside(r.Context(), u.ID, req.Platform, req.URL, req.Count, req.Percent)
	if err != nil {
		s.feedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"claim": c})
}

// GET /v1/admin/outside-feedback?status=pending
func (s *Server) handleAdminOutside(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	if st != "verified" && st != "rejected" && st != "" {
		st = "pending"
	}
	cs, err := s.feedback.Claims(r.Context(), st)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"claims": cs})
}

// POST /v1/admin/outside-feedback/{id} {"decision": "verified"|"rejected", "note"}
func (s *Server) handleAdminReviewOutside(w http.ResponseWriter, r *http.Request) {
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
	a := s.actor(r)
	c, err := s.feedback.ReviewClaim(r.Context(), a.ID, id, req.Decision, req.Note)
	if err != nil {
		s.feedbackError(w, r, err)
		return
	}
	if err := s.audit(r, "seller.outside_feedback_"+req.Decision, "user", c.UserID, req.Note,
		map[string]any{"platform": c.Platform, "profile_url": c.ProfileURL, "count": c.FeedbackCount, "percent": c.PositivePercent}); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"claim": c})
}
