package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/deals"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

// Marketing > Discounts (2026-11-20): coupons, order discounts and sale events.

func (s *Server) dealsError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *deals.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, deals.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
	case errors.Is(err, deals.ErrNoCoupon), errors.Is(err, deals.ErrUsedCoupon), errors.Is(err, deals.ErrOldCoupon),
		errors.Is(err, deals.ErrSoonCoupon):
		writeError(w, http.StatusUnprocessableEntity, "coupon", err.Error(), map[string]string{"coupon": err.Error()})
	default:
		s.internalError(w, r, err)
	}
}

// GET /v1/seller/discounts: everything on the page, plus the sports & games
// a coupon or sale can be limited to.
func (s *Server) handleSellerDiscounts(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	coupons, err := deals.Coupons(r.Context(), s.db, u.ID, 0)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	rules, err := deals.Rules(r.Context(), s.db, u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := map[string]any{"coupons": coupons, "order_discounts": rules, "has_store": false, "sales": []any{},
		"categories": map[string][]string{"sports": listings.ActiveSports(), "tcg": listings.ActiveGames(), "nonsport": listings.ActiveNonSport()}}
	if _, _, ok := s.stores.ForSeller(r.Context(), u.ID); ok {
		sales, err := s.stores.Sales(r.Context(), u.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		out["has_store"], out["sales"] = true, sales
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /v1/seller/coupons
func (s *Server) handleCreateCoupon(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in deals.CouponInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := deals.CreateCoupon(r.Context(), s.db, u.ID, in, time.Now())
	if err != nil {
		s.dealsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"coupon": c})
}

// PATCH /v1/seller/coupons/{id} {"active": false}
func (s *Server) handleCouponActive(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	var in struct {
		Active bool `json:"active"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := deals.SetCouponActive(r.Context(), s.db, u.ID, id, in.Active); err != nil {
		s.dealsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": in.Active})
}

// POST /v1/seller/order-discounts
func (s *Server) handleCreateOrderDiscount(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in deals.RuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	rule, err := deals.CreateRule(r.Context(), s.db, u.ID, in)
	if err != nil {
		s.dealsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"order_discount": rule})
}

// DELETE /v1/seller/order-discounts/{id}
func (s *Server) handleRemoveOrderDiscount(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	if err := deals.RemoveRule(r.Context(), s.db, u.ID, id); err != nil {
		s.dealsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}

// PUT /v1/cart/coupon {"seller": "username", "code": "SAVE10"} ("" removes it)
func (s *Server) handleCartCoupon(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in struct {
		Seller string `json:"seller"`
		Code   string `json:"code"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.shopping.SetCoupon(r.Context(), u.ID, in.Seller, in.Code); err != nil {
		s.dealsError(w, r, err)
		return
	}
	cart, err := s.shopping.GetCart(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}
