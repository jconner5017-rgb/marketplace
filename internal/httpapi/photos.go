package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jconner5017-rgb/marketplace/internal/photos"
)

// POST /v1/photos — body is the raw image (JPEG, PNG or GIF). Returns the
// URL to put in a listing's photo_urls. Works with a session or an API key.
func (s *Server) handleUploadPhoto(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if s.photos == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_off", "photo uploads aren't available right now", nil)
		return
	}
	var seller bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM seller_profiles WHERE user_id = $1)`, u.ID).Scan(&seller); err != nil {
		s.internalError(w, r, err)
		return
	}
	if !seller {
		writeError(w, http.StatusForbidden, "not_a_seller", "set up selling before uploading photos", nil)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, photos.MaxUploadBytes+1))
	if err != nil || len(data) > photos.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", photos.ErrTooLarge.Error(), nil)
		return
	}
	p, err := s.photos.Upload(r.Context(), u.ID, data)
	switch {
	case errors.Is(err, photos.ErrTooMany):
		writeError(w, http.StatusTooManyRequests, "too_many_photos", err.Error(), nil)
		return
	case errors.Is(err, photos.ErrNotImage), errors.Is(err, photos.ErrTooSmall), errors.Is(err, photos.ErrDimension), errors.Is(err, photos.ErrTooLarge):
		writeError(w, http.StatusUnprocessableEntity, "bad_photo", err.Error(), nil)
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"photo": p})
}

// GET /photos/{id}.jpg — photos kept in the database. Cached for a year:
// a photo never changes, a new upload gets a new id.
func (s *Server) handlePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutSuffix(r.PathValue("file"), ".jpg")
	if !ok || len(id) != 36 || s.photos == nil {
		http.NotFound(w, r)
		return
	}
	data, ct, err := s.photos.Get(r.Context(), id)
	if errors.Is(err, photos.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", `"`+id+`"`)
	if r.Header.Get("If-None-Match") == `"`+id+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Write(data)
}
