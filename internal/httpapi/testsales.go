package httpapi

import (
	"errors"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/checkout"
)

func (s *Server) testSaleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, checkout.ErrTestNoShipFrom), errors.Is(err, checkout.ErrNotSellerYet):
		writeError(w, http.StatusConflict, "cannot_create", err.Error(), nil)
	case errors.Is(err, checkout.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "test sale not found", nil)
	default:
		s.internalError(w, r, err)
	}
}

// GET /v1/admin/test-sales — the signed-in staff member's open test sales.
func (s *Server) handleTestSales(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if s.checkout == nil {
		s.checkoutError(w, r, checkout.ErrDisabled)
		return
	}
	list, err := s.checkout.TestSales(r.Context(), u.ID)
	if err != nil {
		s.testSaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"test_sales": list})
}

// POST /v1/admin/test-sales — a paid test order (no payment) in the staff
// member's own seller orders, to try printing envelopes and shipping.
func (s *Server) handleCreateTestSale(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if s.checkout == nil {
		s.checkoutError(w, r, checkout.ErrDisabled)
		return
	}
	t, err := s.checkout.CreateTestSale(r.Context(), u.ID)
	if err != nil {
		s.testSaleError(w, r, err)
		return
	}
	_ = s.audit(r, "test_sale.created", "order", t.ID, "", map[string]any{"number": t.Number})
	writeJSON(w, http.StatusCreated, map[string]any{"test_sale": t})
}

// DELETE /v1/admin/test-sales/{id}
func (s *Server) handleDeleteTestSale(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if s.checkout == nil {
		s.checkoutError(w, r, checkout.ErrDisabled)
		return
	}
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "test sale not found", nil)
		return
	}
	if err := s.checkout.DeleteTestSale(r.Context(), u.ID, id); err != nil {
		s.testSaleError(w, r, err)
		return
	}
	_ = s.audit(r, "test_sale.deleted", "order", id, "", nil)
	w.WriteHeader(http.StatusNoContent)
}
