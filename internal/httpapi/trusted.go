package httpapi

import "net/http"

// GET /v1/seller/trusted — the seller's trusted buyers and invite code.
func (s *Server) handleTrustedBuyers(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	list, invite, err := s.checkout.TrustedBuyers(r.Context(), u.ID)
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buyers": list, "invite": invite})
}

// POST /v1/seller/trusted {username}
func (s *Server) handleAddTrusted(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Username string `json:"username"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.checkout.AddTrusted(r.Context(), u.ID, req.Username); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	s.handleTrustedBuyers(w, r)
}

// DELETE /v1/seller/trusted/{username}
func (s *Server) handleRemoveTrusted(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if err := s.checkout.RemoveTrusted(r.Context(), u.ID, r.PathValue("username")); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	s.handleTrustedBuyers(w, r)
}

// POST /v1/seller/trusted/invite — a new invite link; the old one stops working.
func (s *Server) handleResetInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if _, err := s.checkout.ResetInvite(r.Context(), u.ID); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	s.handleTrustedBuyers(w, r)
}

// GET /v1/trust/{code} — whose invite this is.
func (s *Server) handleInviteInfo(w http.ResponseWriter, r *http.Request) {
	_, name, err := s.checkout.InviteSeller(r.Context(), r.PathValue("code"))
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"seller": name})
}

// POST /v1/trust/{code} — accept a seller's invite.
func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	name, err := s.checkout.AcceptInvite(r.Context(), u.ID, r.PathValue("code"))
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"seller": name})
}

// POST /v1/orders/{id}/envelope-offer — seller trusts the buyer and offers an envelope.
func (s *Server) handleOfferEnvelope(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	if err := s.checkout.OfferEnvelope(r.Context(), u.ID, id); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	s.handleOrder(w, r)
}

// POST /v1/orders/{id}/envelope-offer/{answer} — buyer accepts or declines.
func (s *Server) handleAnswerEnvelope(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	a := r.PathValue("answer")
	if a != "accept" && a != "decline" {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	if err := s.checkout.AnswerEnvelope(r.Context(), u.ID, id, a == "accept"); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	s.handleOrder(w, r)
}
