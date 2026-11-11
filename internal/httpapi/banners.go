package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/banners"
	"github.com/jconner5017-rgb/marketplace/internal/photos"
)

func (s *Server) bannerError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *banners.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, banners.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "banner not found", nil)
	default:
		s.internalError(w, r, err)
	}
}

// GET /v1/banners/{slot} → the banner when it's switched on, otherwise null.
func (s *Server) handleBanner(w http.ResponseWriter, r *http.Request) {
	b, ok, err := s.banners.Live(r.Context(), r.PathValue("slot"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"banner": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"banner": b})
}

// GET /v1/admin/banners/{slot}
func (s *Server) handleAdminBanner(w http.ResponseWriter, r *http.Request) {
	b, err := s.banners.Get(r.Context(), r.PathValue("slot"))
	if err != nil {
		s.bannerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"banner": b, "size": map[string]int{
		"width": banners.Width, "height": banners.Height,
		"upload_width": banners.UploadWidth, "upload_height": banners.UploadHeight,
		"min_width": photos.BannerMinWidth, "min_height": photos.BannerMinHeight,
	}})
}

// PUT /v1/admin/banners/{slot}
func (s *Server) handleAdminSaveBanner(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var b banners.Banner
	if !decodeJSON(w, r, &b) {
		return
	}
	b.Slot = r.PathValue("slot")
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	isPhoto := func(url string) bool {
		var ok bool
		_ = tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM photos WHERE url = $1)`, url).Scan(&ok)
		return ok
	}
	saved, err := s.banners.Save(r.Context(), tx, b, u.ID, isPhoto)
	if err != nil {
		s.bannerError(w, r, err)
		return
	}
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, "banner.updated", "site_banner", saved.Slot, "",
		map[string]any{"image_url": saved.ImageURL, "link_url": saved.LinkURL, "active": saved.Active})); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"banner": saved})
}

// POST /v1/admin/banners/photo — a banner image (wide sizes allowed).
func (s *Server) handleAdminBannerPhoto(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if s.photos == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_off", "photo uploads aren't available right now", nil)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, photos.MaxUploadBytes+1))
	if err != nil || len(data) > photos.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", photos.ErrTooLarge.Error(), nil)
		return
	}
	p, err := s.photos.UploadBanner(r.Context(), u.ID, data)
	switch {
	case errors.Is(err, photos.ErrNotImage), errors.Is(err, photos.ErrBannerTooSmall), errors.Is(err, photos.ErrDimension), errors.Is(err, photos.ErrTooLarge):
		writeError(w, http.StatusUnprocessableEntity, "bad_photo", err.Error(), nil)
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"photo": p})
}
