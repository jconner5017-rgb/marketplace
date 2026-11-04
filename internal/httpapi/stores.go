package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jconner5017-rgb/marketplace/internal/email"
	"github.com/jconner5017-rgb/marketplace/internal/stores"
)

// NewStores builds the store service. Follower updates are emailed (as
// announcements, with the unsubscribe footer) when bulk mail is set up.
func NewStores(db *sql.DB, mail *email.Service) *stores.Service {
	svc := &stores.Service{DB: db}
	if mail.Enabled() && strings.TrimSpace(mail.Postal) != "" {
		svc.Email = func(ctx context.Context, tx *sql.Tx, userID, subject, body string) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO email_outbox (user_id, to_email, kind, subject, body)
				SELECT id, email, 'broadcast', $2, $3 FROM users WHERE id = $1`, userID, subject, body)
			return err
		}
	}
	return svc
}

func (s *Server) storeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *stores.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, stores.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "store not found", nil)
	case errors.Is(err, stores.ErrNotSeller), errors.Is(err, stores.ErrNotActive):
		writeError(w, http.StatusForbidden, "store_unavailable", err.Error(), nil)
	case errors.Is(err, stores.ErrSaleClash), errors.Is(err, stores.ErrTooSoon), errors.Is(err, stores.ErrOwnStore):
		writeError(w, http.StatusConflict, "store_conflict", err.Error(), nil)
	default:
		s.internalError(w, r, err)
	}
}

// ownPhoto: a logo or banner must be a photo this member uploaded here.
func (s *Server) ownPhoto(ctx context.Context, userID string) func(string) bool {
	return func(u string) bool {
		var ok bool
		_ = s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM photos WHERE url = $1 AND owner_id = $2)`, u, userID).Scan(&ok)
		return ok
	}
}

// GET /v1/stores/{slug}
func (s *Server) handleStore(w http.ResponseWriter, r *http.Request) {
	viewer := ""
	if u, ok := currentUser(r); ok {
		viewer = u.ID
	}
	st, err := s.stores.BySlug(r.Context(), r.PathValue("slug"), viewer)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"store": st.Public()})
}

// GET /v1/stores/by-seller/{username} → the seller's open store, if any.
func (s *Server) handleStoreBySeller(w http.ResponseWriter, r *http.Request) {
	var id string
	err := s.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE lower(username) = lower($1)`, r.PathValue("username")).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"store": nil})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	slug, name, ok := s.stores.ForSeller(r.Context(), id)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"store": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"store": map[string]string{"slug": slug, "name": name}})
}

// POST and DELETE /v1/stores/{slug}/follow
func (s *Server) handleFollowStore(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	st, err := s.stores.Follow(r.Context(), u.ID, r.PathValue("slug"), r.Method == http.MethodPost)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"following": st.Following, "followers": st.Followers})
}

// GET /v1/me/stores — stores the member follows.
func (s *Server) handleFollowedStores(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	list, err := s.stores.Following(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stores": list})
}

// GET /v1/seller/store — the seller's store (or null) and its sales.
func (s *Server) handleMyStore(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	st, err := s.stores.Mine(r.Context(), u.ID)
	if errors.Is(err, stores.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"store": nil, "sales": []stores.Sale{}})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	sales, err := s.stores.Sales(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"store": st, "sales": sales})
}

// PUT /v1/seller/store — open the store, or save its settings.
func (s *Server) handleSaveStore(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in stores.Settings
	if !decodeJSON(w, r, &in) {
		return
	}
	st, err := s.stores.Apply(r.Context(), u.ID, in, s.ownPhoto(r.Context(), u.ID))
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"store": st})
}

// PUT /v1/seller/store/categories {"categories": [{"id", "name"}]}
func (s *Server) handleStoreCategories(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Categories []stores.Category `json:"categories"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.stores.SetCategories(r.Context(), u.ID, req.Categories); err != nil {
		s.storeError(w, r, err)
		return
	}
	s.handleMyStore(w, r)
}

// POST /v1/seller/store/categorize {"category_id": 3, "listing_ids": [...]} (0 = no category)
func (s *Server) handleCategorize(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		CategoryID int      `json:"category_id"`
		ListingIDs []string `json:"listing_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	n, err := s.stores.Categorize(r.Context(), u.ID, req.CategoryID, req.ListingIDs)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n})
}

// POST /v1/seller/store/sales
func (s *Server) handleCreateSale(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in stores.SaleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	sale, err := s.stores.CreateSale(r.Context(), u.ID, in)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"sale": sale})
}

// DELETE /v1/seller/store/sales/{id}
func (s *Server) handleEndSale(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	if err := s.stores.EndSale(r.Context(), u.ID, id); err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ended": true})
}

// POST /v1/seller/store/updates {"subject", "body"}
func (s *Server) handleStoreUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	n, emailed, err := s.stores.SendUpdate(r.Context(), u.ID, req.Subject, req.Body)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"recipients": n, "emailed": emailed})
}

// GET /v1/admin/stores?status=pending
func (s *Server) handleAdminStores(w http.ResponseWriter, r *http.Request) {
	list, err := s.stores.List(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stores": list})
}

// POST /v1/admin/stores/{username}/review {"status": "active"|"rejected"|"suspended", "note", "reason"}
func (s *Server) handleAdminReviewStore(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	var sellerID string
	if err := s.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE lower(username) = lower($1)`, r.PathValue("username")).Scan(&sellerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "store not found", nil)
			return
		}
		s.internalError(w, r, err)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	if err := s.stores.Review(r.Context(), tx, sellerID, req.Status, req.Note, u.ID); err != nil {
		s.storeError(w, r, err)
		return
	}
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, "store."+req.Status, "store", sellerID, req.Reason,
		map[string]any{"note": req.Note})); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": req.Status})
}
