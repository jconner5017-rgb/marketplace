package httpapi

import (
	"errors"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/checkout"
	"github.com/jconner5017-rgb/marketplace/internal/postage"
)

func (s *Server) postageErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, checkout.ErrPostageOff):
		writeError(w, http.StatusServiceUnavailable, "postage_off", err.Error(), nil)
	case errors.Is(err, checkout.ErrPostageOver):
		writeError(w, http.StatusConflict, "label_problem", err.Error(), nil)
	default:
		var ae *postage.APIError
		if errors.As(err, &ae) {
			s.log.Error("easypost", "err", err)
			writeError(w, http.StatusBadGateway, "carrier_error", err.Error(), nil)
			return
		}
		s.labelError(w, r, err)
	}
}

// GET /v1/seller/postage — postage balance, label card and history.
func (s *Server) handlePostageAccount(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	a, err := s.checkout.PostageAccountFor(r.Context(), u.ID)
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// POST /v1/seller/postage/topup {"amount_cents": 1000}
func (s *Server) handlePostageTopUp(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Cents int `json:"amount_cents"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	bal, err := s.checkout.TopUpPostage(r.Context(), u.ID, req.Cents)
	if err != nil {
		s.postageErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"balance_cents": bal})
}

// POST /v1/orders/{id}/postage/quote {"rigid": false}
func (s *Server) handlePostageQuote(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Rigid bool `json:"rigid"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	q, err := s.checkout.QuotePostage(r.Context(), u.ID, id, req.Rigid)
	if err != nil {
		s.postageErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quote": q})
}

// POST /v1/orders/{id}/postage {"rate_id": "...", "pay": "balance" | "card"}
func (s *Server) handleBuyPostage(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var req struct {
		RateID string `json:"rate_id"`
		Pay    string `json:"pay"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	b, err := s.checkout.BuyPostage(r.Context(), u.ID, id, req.RateID, req.Pay)
	if err != nil {
		s.postageErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"postage": b})
}
