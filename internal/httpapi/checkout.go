package httpapi

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/checkout"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

func (s *Server) checkoutError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *checkout.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, checkout.ErrDisabled):
		writeError(w, http.StatusServiceUnavailable, "checkout_disabled", err.Error(), nil)
	case errors.Is(err, checkout.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, checkout.ErrNotPaid):
		writeError(w, http.StatusConflict, "not_paid", err.Error(), nil)
	case errors.Is(err, checkout.ErrAlreadyPaid):
		writeError(w, http.StatusConflict, "already_paid", err.Error(), nil)
	case errors.Is(err, checkout.ErrEmpty), errors.Is(err, checkout.ErrChanged), errors.Is(err, checkout.ErrSellerClosed),
		errors.Is(err, checkout.ErrOwn), errors.Is(err, checkout.ErrState):
		writeError(w, http.StatusConflict, "cannot_checkout", err.Error(), nil)
	default:
		s.internalError(w, r, err)
	}
}

// GET /v1/checkout/status — whether buyers can pay yet.
func (s *Server) handleCheckoutStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.checkout != nil && s.checkout.Ready()})
}

// POST /v1/checkout {"seller": "username"} or {"hold_ids": [...]}
func (s *Server) handleStartCheckout(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if s.checkout == nil {
		s.checkoutError(w, r, checkout.ErrDisabled)
		return
	}
	var req checkout.Request
	if !decodeJSON(w, r, &req) {
		return
	}
	for _, id := range req.HoldIDs {
		if !isUUID(id) {
			writeError(w, http.StatusNotFound, "not_found", "hold not found", nil)
			return
		}
	}
	orderID, url, err := s.checkout.Start(r.Context(), u.ID, u.Email, req)
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"order_id": orderID, "url": url})
}

func orderParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if isUUID(id) {
		return id, true
	}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil && n > 0 {
		return id, true
	}
	writeError(w, http.StatusNotFound, "not_found", "order not found", nil)
	return "", false
}

// POST /v1/orders/{id}/confirm?session_id=cs_... — called when the buyer returns from Stripe.
func (s *Server) handleConfirmOrder(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id := r.PathValue("id")
	if !isUUID(id) || s.checkout == nil {
		writeError(w, http.StatusNotFound, "not_found", "order not found", nil)
		return
	}
	st, err := s.checkout.Confirm(r.Context(), u.ID, id, r.URL.Query().Get("session_id"))
	if errors.Is(err, checkout.ErrNotPaid) {
		writeJSON(w, http.StatusOK, map[string]string{"status": st})
		return
	}
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": st})
}

// POST /v1/orders/{id}/cancel — buyer backed out on Stripe's page.
func (s *Server) handleAbandonOrder(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id := r.PathValue("id")
	if !isUUID(id) || s.checkout == nil {
		writeError(w, http.StatusNotFound, "not_found", "order not found", nil)
		return
	}
	err := s.checkout.Abandon(r.Context(), u.ID, id)
	if errors.Is(err, checkout.ErrAlreadyPaid) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "paid"})
		return
	}
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// GET /v1/orders?side=buying|selling&status=to_ship|shipped|delivered|closed
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	side := r.URL.Query().Get("side")
	if side != "selling" {
		side = "buying"
	}
	if viaAPIKey(r) {
		side = "selling"
	}
	os, err := s.checkout.Orders(r.Context(), u.ID, side, r.URL.Query().Get("status"), 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": os})
}

// GET /v1/orders/{id}
func (s *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	o, err := s.checkout.Order(r.Context(), u.ID, id)
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": o})
}

// POST /v1/orders/{id}/ship {"carrier": "USPS", "tracking_number": "..."}
func (s *Server) handleShipOrder(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Carrier  string `json:"carrier"`
		Tracking string `json:"tracking_number"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.checkout.MarkShipped(r.Context(), u.ID, id, req.Carrier, req.Tracking); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	s.handleOrder(w, r)
}

// POST /v1/orders/{id}/delivered — buyer confirms the cards arrived.
func (s *Server) handleOrderDelivered(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	if err := s.checkout.MarkDelivered(r.Context(), u.ID, id); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	s.handleOrder(w, r)
}

// POST /v1/orders/{id}/problem {"kind": "inr"|"snad"|"other", "summary": "..."}
func (s *Server) handleOrderProblem(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	n, err := s.checkout.ReportProblem(r.Context(), u.ID, id, req.Kind, req.Summary)
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"case_number": n})
}

func parseDay(s string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	return t, err == nil
}

// GET /v1/seller/reports/sales.csv?from=2026-01-01&to=2026-01-31 (to is inclusive)
func (s *Server) handleSalesCSV(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	from := to.AddDate(0, 0, -29)
	q := r.URL.Query()
	if v := q.Get("from"); v != "" {
		t, ok := parseDay(v, loc)
		if !ok {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", "dates look like 2026-01-31", map[string]string{"from": "use YYYY-MM-DD"})
			return
		}
		from = t
	}
	if v := q.Get("to"); v != "" {
		t, ok := parseDay(v, loc)
		if !ok {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", "dates look like 2026-01-31", map[string]string{"to": "use YYYY-MM-DD"})
			return
		}
		to = t
	}
	if to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "choose a range of up to 400 days", map[string]string{"to": "up to 400 days after the start"})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sales-%s-to-%s.csv"`, from.Format("2006-01-02"), to.Format("2006-01-02")))
	if err := s.checkout.SalesCSV(r.Context(), w, u.ID, from, to.AddDate(0, 0, 1)); err != nil {
		s.log.Error("sales csv", "err", err)
	}
}

// GET /v1/seller/reports/summary?year=2026
func (s *Server) handleSalesSummary(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	year := time.Now().Year()
	if v, err := strconv.Atoi(r.URL.Query().Get("year")); err == nil && v >= 2025 && v <= year {
		year = v
	}
	months, total, err := s.checkout.Summary(r.Context(), u.ID, year)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"year": year, "months": months, "total": total})
}

// GET /v1/orders/{id}/evidence — the seller's chargeback evidence packet (HTML download).
func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	s.evidence(w, r, u.ID, false)
}

// GET /v1/admin/orders/{id}/evidence
func (s *Server) handleAdminEvidence(w http.ResponseWriter, r *http.Request) {
	s.evidence(w, r, "", true)
}

func (s *Server) evidence(w http.ResponseWriter, r *http.Request, userID string, staff bool) {
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	if err := s.checkout.Evidence(r.Context(), &buf, userID, id, staff); err != nil {
		s.checkoutError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="order-`+id+`-evidence.html"`)
	w.Write(buf.Bytes())
}

func (s *Server) labelError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, checkout.ErrNoLabels):
		writeError(w, http.StatusServiceUnavailable, "labels_off", err.Error(), nil)
	case errors.Is(err, checkout.ErrNoShipFrom), errors.Is(err, checkout.ErrNoCard), errors.Is(err, checkout.ErrHasLabel),
		errors.Is(err, checkout.ErrRateExpired), errors.Is(err, checkout.ErrLabelUsed), errors.Is(err, shipping.ErrNoRates),
		errors.Is(err, shipping.ErrBadAddress):
		writeError(w, http.StatusConflict, "label_problem", err.Error(), nil)
	default:
		var fe *checkout.FieldError
		if errors.As(err, &fe) {
			writeError(w, http.StatusUnprocessableEntity, "invalid", fe.Msg, map[string]string{fe.Field: fe.Msg})
			return
		}
		if errors.Is(err, checkout.ErrNotFound) || errors.Is(err, checkout.ErrState) {
			s.checkoutError(w, r, err)
			return
		}
		s.log.Error("label", "err", err)
		writeError(w, http.StatusBadGateway, "carrier_error", err.Error(), nil)
	}
}

// GET /v1/seller/shipping — ship-from address and label card.
func (s *Server) handleShippingSettings(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	st, err := s.checkout.LabelSettings(r.Context(), u.ID)
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// PUT /v1/seller/shipping/from {name, line1, line2, city, state, zip, phone}
func (s *Server) handleShipFrom(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var a shipping.Address
	if !decodeJSON(w, r, &a) {
		return
	}
	a.Email = u.Email
	v, err := s.checkout.SetShipFrom(r.Context(), u.ID, a)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ship_from": v})
}

// POST /v1/seller/shipping/card — Stripe page for saving a card for labels.
func (s *Server) handleLabelCard(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	url, err := s.checkout.StartCardSetup(r.Context(), u.ID, u.Email)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// POST /v1/seller/shipping/card/finish {"session_id"}
func (s *Server) handleLabelCardFinish(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		SessionID string `json:"session_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	card, err := s.checkout.FinishCardSetup(r.Context(), u.ID, req.SessionID)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"card": card})
}

// POST /v1/orders/{id}/label/rates
func (s *Server) handleLabelRates(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	rates, err := s.checkout.LabelRates(r.Context(), u.ID, id)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rates": rates})
}

// POST /v1/orders/{id}/label {"rate_id"}
func (s *Server) handleBuyLabel(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var req struct {
		RateID string `json:"rate_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	l, err := s.checkout.BuyLabel(r.Context(), u.ID, id, req.RateID)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"label": l})
}

// POST /v1/orders/{id}/label/void
func (s *Server) handleVoidLabel(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	if err := s.checkout.VoidLabel(r.Context(), u.ID, id); err != nil {
		s.labelError(w, r, err)
		return
	}
	s.handleOrder(w, r)
}

// POST /v1/webhooks/shipping/{provider}?token=... — carrier tracking updates.
func (s *Server) handleShippingWebhook(w http.ResponseWriter, r *http.Request) {
	p := s.checkout.Ship
	if p == nil || r.PathValue("provider") != p.Name() || s.shippingToken == "" ||
		subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(s.shippingToken)) != 1 {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	events, err := p.ParseTrackingWebhook(r)
	if errors.Is(err, shipping.ErrBadWebhook) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	n, err := s.checkout.ApplyTracking(r.Context(), events)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}
