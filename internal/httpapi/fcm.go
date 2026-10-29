package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/checkout"
	"github.com/jconner5017-rgb/marketplace/internal/fcm"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

const apiKeyCtx ctxKey = 2

func viaAPIKey(r *http.Request) bool { v, _ := r.Context().Value(apiKeyCtx).(bool); return v }

// sellerAPI accepts a seller API key (Authorization: Bearer ss_key_...) or a
// signed-in session. Writes with an Idempotency-Key are answered once and
// replayed on retries.
func (s *Server) sellerAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "" {
			token, ok := strings.CutPrefix(h, "Bearer ")
			if !ok || s.fcm == nil {
				writeError(w, http.StatusUnauthorized, "bad_api_key", "use Authorization: Bearer <API key>", nil)
				return
			}
			uid, err := s.fcm.Authenticate(r.Context(), strings.TrimSpace(token))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "bad_api_key", "invalid or revoked API key", nil)
				return
			}
			u, err := s.auth.UserByID(r.Context(), uid)
			if err != nil {
				writeError(w, http.StatusForbidden, "account_disabled", "this seller account can't use the API right now", nil)
				return
			}
			if u.Frozen() && r.Method != http.MethodGet {
				writeError(w, http.StatusForbidden, "account_frozen", "this account is frozen while we review it", nil)
				return
			}
			ctx := context.WithValue(context.WithValue(r.Context(), userKey, u), apiKeyCtx, true)
			r = r.WithContext(ctx)
		}
		u, ok := currentUser(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "log in or send an API key", nil)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" || r.Method == http.MethodGet || s.fcm == nil {
			next(w, r)
			return
		}
		if len(key) > 100 {
			writeError(w, http.StatusBadRequest, "bad_idempotency_key", "Idempotency-Key is too long (100 characters at most)", nil)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large", nil)
			return
		}
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])
		prev, err := s.fcm.Recall(r.Context(), u.ID, key, r.Method, r.URL.Path, hash)
		if errors.Is(err, fcm.ErrKeyReused) {
			writeError(w, http.StatusUnprocessableEntity, "idempotency_key_reused", err.Error(), nil)
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if prev != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(prev.Status)
			w.Write(prev.Body)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		rec := httptest.NewRecorder()
		next(rec, r)
		res := rec.Result()
		out, _ := io.ReadAll(res.Body)
		if res.StatusCode < 500 {
			if err := s.fcm.Remember(r.Context(), u.ID, key, r.Method, r.URL.Path, hash, res.StatusCode, out); err != nil {
				s.log.Error("remember idempotency key", "err", err)
			}
		}
		for k, v := range res.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(res.StatusCode)
		w.Write(out)
	}
}

// POST /v1/listings — create or replace a listing by its external_id (SKU).
func (s *Server) handleAPICreateListing(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in listings.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ExternalID == nil || strings.TrimSpace(*in.ExternalID) == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "external_id is required", map[string]string{"external_id": "required"})
		return
	}
	l, created, err := s.listings.UpsertByExternalID(r.Context(), u.ID, in)
	if err != nil {
		s.listingError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"listing": l, "created": created})
}

// POST /v1/listings/bulk {"listings": [...]} — up to 500 creates/replaces by SKU.
func (s *Server) handleAPIBulk(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		Listings []listings.Input `json:"listings"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Listings) == 0 || len(req.Listings) > 500 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "send 1 to 500 listings per call", nil)
		return
	}
	type result struct {
		ExternalID string            `json:"external_id"`
		Status     string            `json:"status"` // created, updated, error
		ListingID  string            `json:"listing_id,omitempty"`
		Error      string            `json:"error,omitempty"`
		Fields     map[string]string `json:"fields,omitempty"`
	}
	out := make([]result, 0, len(req.Listings))
	counts := map[string]int{}
	for _, in := range req.Listings {
		res := result{}
		if in.ExternalID != nil {
			res.ExternalID = strings.TrimSpace(*in.ExternalID)
		}
		if res.ExternalID == "" {
			res.Status, res.Error = "error", "external_id is required"
		} else if l, created, err := s.listings.UpsertByExternalID(r.Context(), u.ID, in); err != nil {
			res.Status, res.Error = "error", err.Error()
			var ve *listings.ValidationError
			if errors.As(err, &ve) {
				res.Error, res.Fields = "check the fields", ve.Fields
			} else if !isClientListingErr(err) {
				s.internalError(w, r, err)
				return
			}
		} else {
			res.Status, res.ListingID = "updated", l.ID
			if created {
				res.Status = "created"
			}
		}
		counts[res.Status]++
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out, "created": counts["created"], "updated": counts["updated"], "errors": counts["error"]})
}

func isClientListingErr(err error) bool {
	return errors.Is(err, listings.ErrBlocked) || errors.Is(err, listings.ErrSellerRejected) ||
		errors.Is(err, listings.ErrFormatChange) || errors.Is(err, listings.ErrNotFound) || errors.Is(err, listings.ErrHasBids) ||
		errors.Is(err, listings.ErrAuctionOver) || errors.Is(err, listings.ErrCannotSell) ||
		errors.Is(err, listings.ErrDuplicateExtern) || errors.Is(err, sellers.ErrNotSeller)
}

func (s *Server) listingByExternal(w http.ResponseWriter, r *http.Request) (string, bool) {
	u, _ := currentUser(r)
	ext := strings.TrimSpace(r.PathValue("externalID"))
	var id string
	err := s.db.QueryRowContext(r.Context(), `SELECT id FROM listings WHERE seller_id = $1 AND external_id = $2 AND status <> 'removed'`,
		u.ID, ext).Scan(&id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no listing with that external_id", nil)
		return "", false
	}
	return id, true
}

// PATCH /v1/listings/{externalID} — change price, quantity, photos or other fields.
func (s *Server) handleAPIPatchListing(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := s.listingByExternal(w, r)
	if !ok {
		return
	}
	var p listings.Patch
	if !decodeJSON(w, r, &p) {
		return
	}
	l, err := s.listings.Update(r.Context(), u.ID, id, p)
	if err != nil {
		s.listingError(w, r, err)
		return
	}
	if l.Quantity == 0 && s.checkout != nil {
		if err := s.checkout.ReleaseListing(r.Context(), id); err != nil {
			s.log.Error("release listing", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"listing": l})
}

// DELETE /v1/listings/{externalID} — e.g. it sold on eBay.
func (s *Server) handleAPIDeleteListing(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := s.listingByExternal(w, r)
	if !ok {
		return
	}
	if err := s.listings.Remove(r.Context(), u.ID, id); err != nil {
		s.listingError(w, r, err)
		return
	}
	if s.checkout != nil {
		if err := s.checkout.ReleaseListing(r.Context(), id); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/orders/{id}/oversold {"external_id": "...", "sold_elsewhere_at": "2026-10-29T14:03:00Z"}
func (s *Server) handleOversold(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := orderParam(w, r)
	if !ok {
		return
	}
	var req struct {
		ExternalID string    `json:"external_id"`
		SoldAt     time.Time `json:"sold_elsewhere_at"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SoldAt.IsZero() || req.SoldAt.After(time.Now().Add(5*time.Minute)) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "sold_elsewhere_at must be the time of the other sale",
			map[string]string{"sold_elsewhere_at": "RFC 3339 time"})
		return
	}
	outcome, err := s.checkout.Oversold(r.Context(), u.ID, id, req.ExternalID, req.SoldAt)
	if errors.Is(err, checkout.ErrNotOurs) {
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	if err != nil {
		s.checkoutError(w, r, err)
		return
	}
	msg := "This sale came first; cancel the other one."
	if outcome == "refunded" {
		msg = "The other sale came first, so this order was refunded in full."
	}
	writeJSON(w, http.StatusOK, map[string]string{"outcome": outcome, "message": msg})
}

// GET /v1/seller/connect — API keys, webhook settings and recent deliveries.
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	keys, err := s.fcm.Keys(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ep, err := s.fcm.Endpoint(r.Context(), u.ID)
	if errors.Is(err, fcm.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_seller", "set up selling first", nil)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ds, err := s.fcm.Deliveries(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys, "endpoint": ep, "deliveries": ds})
}

// POST /v1/seller/api-keys {"name"} — the key is in the answer once.
func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if viaAPIKey(r) {
		writeError(w, http.StatusForbidden, "forbidden", "API keys are made from the website", nil)
		return
	}
	if _, err := s.sellers.Get(r.Context(), u.ID); err != nil {
		writeError(w, http.StatusNotFound, "not_seller", "set up selling first", nil)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	plain, k, err := s.fcm.CreateKey(r.Context(), u.ID, req.Name)
	if errors.Is(err, fcm.ErrTooMany) {
		writeError(w, http.StatusConflict, "too_many_keys", err.Error(), nil)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"api_key": plain, "key": k})
}

// DELETE /v1/seller/api-keys/{id}
func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if err := s.fcm.RevokeKey(r.Context(), u.ID, r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "key not found", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/seller/connect {"webhook_url", "inventory_url", "rotate_secret"}
func (s *Server) handleSetConnect(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		WebhookURL   string `json:"webhook_url"`
		InventoryURL string `json:"inventory_url"`
		Rotate       bool   `json:"rotate_secret"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	secret, err := s.fcm.SetEndpoint(r.Context(), u.ID, req.WebhookURL, req.InventoryURL, req.Rotate)
	var fe *fcm.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
		return
	case errors.Is(err, fcm.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_seller", "set up selling first", nil)
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	out := map[string]any{"ok": true}
	if secret != "" {
		out["signing_secret"] = secret
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /v1/seller/connect/test — queue a ping webhook.
func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	err := s.fcm.SendTest(r.Context(), u.ID)
	var fe *fcm.FieldError
	if errors.As(err, &fe) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}
