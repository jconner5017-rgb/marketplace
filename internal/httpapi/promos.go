package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/promos"
)

var eastern = func() *time.Location {
	if l, err := time.LoadLocation("America/New_York"); err == nil {
		return l
	}
	return time.FixedZone("ET", -5*3600)
}()

func (s *Server) promoSvc() *promos.Service { return &promos.Service{DB: s.db} }

func (s *Server) promoError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *promos.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, promos.ErrNotFound):
		writeError(w, http.StatusNotFound, "promo_invalid", "That promo code isn't valid.", map[string]string{"code": "That promo code isn't valid."})
	case errors.Is(err, promos.ErrUsed), errors.Is(err, promos.ErrExpired), errors.Is(err, promos.ErrFull), errors.Is(err, promos.ErrNotNew):
		msg := err.Error()
		msg = string(msg[0]-32) + msg[1:] + "."
		writeError(w, http.StatusConflict, "promo_unavailable", msg, map[string]string{"code": msg})
	default:
		s.internalError(w, r, err)
	}
}

// promoInput is the back office form. Blank optional fields mean "no limit".
type promoInput struct {
	Code           string  `json:"code"`
	Label          string  `json:"label"`
	Audience       string  `json:"audience"`
	FeeFreeSales   int     `json:"fee_free_sales"`
	MaxRedemptions *int    `json:"max_redemptions"`
	NewSellersOnly bool    `json:"new_sellers_only"`
	ExpiresAt      *string `json:"expires_at"`
	ValidDays      *int    `json:"valid_days"`
	Active         bool    `json:"active"`
	Reason         string  `json:"reason"`
}

func (in promoInput) code() (promos.Code, error) {
	c := promos.Code{Code: in.Code, Label: in.Label, Audience: in.Audience, FeeFreeSales: in.FeeFreeSales,
		MaxRedemptions: in.MaxRedemptions, NewSellersOnly: in.NewSellersOnly, ValidDays: in.ValidDays, Active: in.Active}
	if c.MaxRedemptions != nil && *c.MaxRedemptions == 0 {
		c.MaxRedemptions = nil
	}
	if c.ValidDays != nil && *c.ValidDays == 0 {
		c.ValidDays = nil
	}
	if in.ExpiresAt != nil && strings.TrimSpace(*in.ExpiresAt) != "" {
		v := strings.TrimSpace(*in.ExpiresAt)
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			// A plain date is the last day the code works, to the end of that
			// day in US Eastern time.
			d, derr := time.ParseInLocation("2006-01-02", v, eastern)
			if derr != nil {
				return c, &promos.FieldError{Field: "expires_at", Msg: "enter a date like 2026-12-31"}
			}
			t = d.AddDate(0, 0, 1).Add(-time.Second)
		}
		c.ExpiresAt = &t
	}
	return c, nil
}

// GET /v1/admin/promos
func (s *Server) handleAdminPromos(w http.ResponseWriter, r *http.Request) {
	list, err := s.promoSvc().List(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"promos": list})
}

// GET /v1/admin/promos/{id}
func (s *Server) handleAdminPromo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	svc := s.promoSvc()
	c, err := svc.Get(r.Context(), id)
	if err != nil {
		s.promoError(w, r, err)
		return
	}
	reds, err := svc.Redemptions(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"promo": c, "redemptions": reds})
}

// POST /v1/admin/promos and PATCH /v1/admin/promos/{id}
func (s *Server) handleAdminSavePromo(w http.ResponseWriter, r *http.Request) {
	var in promoInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := in.code()
	if err != nil {
		s.promoError(w, r, err)
		return
	}
	u, _ := currentUser(r)
	svc := s.promoSvc()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	action, details := "promo.created", map[string]any{}
	if idText := r.PathValue("id"); idText != "" {
		id, err := strconv.Atoi(idText)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "not found", nil)
			return
		}
		before, err := svc.Get(r.Context(), id)
		if err != nil {
			s.promoError(w, r, err)
			return
		}
		c.ID = id
		if c, err = svc.Update(r.Context(), tx, c); err != nil {
			s.promoError(w, r, err)
			return
		}
		action, details["before"] = "promo.updated", before
	} else if c, err = svc.Create(r.Context(), tx, c, u.ID); err != nil {
		s.promoError(w, r, err)
		return
	}
	details["after"] = c
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, action, "promo", strconv.Itoa(c.ID), in.Reason, details)); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	status := http.StatusOK
	if action == "promo.created" {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"promo": c})
}

// GET /v1/seller/promos — the seller's fee-free sales.
func (s *Server) handleSellerPromos(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	svc := s.promoSvc()
	reds, err := svc.ForSeller(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	left, now := 0, time.Now()
	type out struct {
		promos.Redemption
		Remaining int `json:"remaining"`
	}
	list := make([]out, 0, len(reds))
	for _, x := range reds {
		x.Seller = ""
		left += x.Remaining(now)
		list = append(list, out{x, x.Remaining(now)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"fee_free_sales_left": left, "redemptions": list})
}

// POST /v1/seller/promos {code}
func (s *Server) handleRedeemPromo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.promoLimiter.allow("promo:" + clientIP(r, s.trustProxy)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many tries. Wait a few minutes and try again.", nil)
		return
	}
	u, _ := currentUser(r)
	if _, err := s.promoSvc().Redeem(r.Context(), u.ID, req.Code); err != nil {
		s.promoError(w, r, err)
		return
	}
	s.handleSellerPromos(w, r)
}
