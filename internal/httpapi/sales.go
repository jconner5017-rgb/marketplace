package httpapi

import (
	"errors"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/sales"
)

func (s *Server) salesError(w http.ResponseWriter, r *http.Request, err error) {
	var low *sales.BidTooLowError
	switch {
	case errors.As(err, &low):
		writeError(w, http.StatusUnprocessableEntity, "bid_too_low", low.Error(),
			map[string]string{"max_cents": low.Error()})
	case errors.Is(err, sales.ErrNotFound), errors.Is(err, sales.ErrOfferNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, sales.ErrNotAuction), errors.Is(err, sales.ErrAuctionOver),
		errors.Is(err, sales.ErrNoBuyNow), errors.Is(err, sales.ErrAlreadyLeader),
		errors.Is(err, sales.ErrNotAvailable), errors.Is(err, sales.ErrOffersOff),
		errors.Is(err, sales.ErrOfferLimit), errors.Is(err, sales.ErrPendingOffer),
		errors.Is(err, sales.ErrOfferClosed):
		writeError(w, http.StatusConflict, "not_allowed", err.Error(), nil)
	case errors.Is(err, sales.ErrOwnListing):
		writeError(w, http.StatusUnprocessableEntity, "own_listing", err.Error(), nil)
	case errors.Is(err, sales.ErrOfferAtPrice), errors.Is(err, sales.ErrOfferTooLow),
		errors.Is(err, sales.ErrBadCounter), errors.Is(err, sales.ErrTooManyCards):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error(),
			map[string]string{"amount_cents": err.Error()})
	case errors.Is(err, sales.ErrOfferNote), errors.Is(err, sales.ErrNoteTooLong):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error(),
			map[string]string{"message": err.Error()})
	case errors.Is(err, sales.ErrUnknownAction):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
	default:
		s.internalError(w, r, err)
	}
}

// POST /v1/listings/{id}/bids {"max_cents": 2500}
func (s *Server) handleBid(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	var req struct {
		MaxCents int `json:"max_cents"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.bidLimiter.allow("bid:" + u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many bids at once; wait a moment", nil)
		return
	}
	res, err := s.sales.PlaceBid(r.Context(), u.ID, id, req.MaxCents, clientIP(r, s.trustProxy), r.UserAgent())
	if err != nil {
		s.salesError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /v1/listings/{id}/bids — bid history; viewer status when signed in.
func (s *Server) handleBids(w http.ResponseWriter, r *http.Request) {
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	u, _ := currentUser(r)
	h, err := s.sales.Bids(r.Context(), id, u.ID)
	if err != nil {
		s.salesError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

// POST /v1/listings/{id}/buy-now — Buy It Now on an auction with no bids.
func (s *Server) handleAuctionBuyNow(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	if err := s.sales.BuyNow(r.Context(), u.ID, id, clientIP(r, s.trustProxy), r.UserAgent()); err != nil {
		s.salesError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"won": true})
}

// POST /v1/listings/{id}/offers {"amount_cents": 1500, "quantity": 1, "message": ""}
func (s *Server) handleMakeOffer(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	var req struct {
		AmountCents int    `json:"amount_cents"`
		Quantity    int    `json:"quantity"`
		Message     string `json:"message"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	o, err := s.sales.MakeOffer(r.Context(), u.ID, id, req.AmountCents, req.Quantity, req.Message)
	if err != nil {
		s.salesError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"offer": o})
}

// POST /v1/offers/{id}/respond {"action": "accept"|"decline"|"counter", "amount_cents": 1800}
func (s *Server) handleRespondOffer(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "offer not found", nil)
		return
	}
	var req struct {
		Action      string `json:"action"`
		AmountCents int    `json:"amount_cents"`
		Message     string `json:"message"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	o, err := s.sales.Respond(r.Context(), u.ID, id, req.Action, req.AmountCents, req.Message)
	if err != nil {
		s.salesError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"offer": o})
}

// GET /v1/offers?side=buying|selling&listing=<id>
func (s *Server) handleOffers(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	side := r.URL.Query().Get("side")
	if side != "selling" {
		side = "buying"
	}
	lid := r.URL.Query().Get("listing")
	if lid != "" && !isUUID(lid) {
		lid = ""
	}
	os, err := s.sales.OffersFor(r.Context(), u.ID, side, lid, 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"offers": os})
}

// GET /v1/holds?side=buying|selling — won auctions and accepted offers waiting for payment.
func (s *Server) handleHolds(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	side := r.URL.Query().Get("side")
	if side != "selling" {
		side = "buying"
	}
	hs, err := s.sales.HoldsFor(r.Context(), u.ID, side, 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"holds": hs})
}
