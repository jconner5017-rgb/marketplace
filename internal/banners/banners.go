// Package banners holds the site's editable promo banners: an image, an
// optional link and a description, set by staff in the back office.
package banners

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"
)

// Display size of the homepage banner: the standard 970 × 250 "billboard".
// Staff upload it at double size so it stays sharp on high-resolution screens.
const (
	Width        = 970
	Height       = 250
	UploadWidth  = Width * 2
	UploadHeight = Height * 2
)

var ErrNotFound = errors.New("banner not found")

// FieldError is a validation problem with one field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

type Banner struct {
	Slot      string     `json:"slot"`
	ImageURL  string     `json:"image_url"`
	LinkURL   string     `json:"link_url"`
	AltText   string     `json:"alt_text"`
	Active    bool       `json:"active"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type Service struct{ DB *sql.DB }

// Get returns a banner as stored, active or not (for the back office).
func (s *Service) Get(ctx context.Context, slot string) (Banner, error) {
	var b Banner
	var at time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT slot, image_url, link_url, alt_text, active, updated_at FROM site_banners WHERE slot = $1`, slot).
		Scan(&b.Slot, &b.ImageURL, &b.LinkURL, &b.AltText, &b.Active, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return Banner{}, ErrNotFound
	}
	b.UpdatedAt = &at
	return b, err
}

// Live returns the banner only when it should be shown; ok is false otherwise.
func (s *Service) Live(ctx context.Context, slot string) (Banner, bool, error) {
	b, err := s.Get(ctx, slot)
	if errors.Is(err, ErrNotFound) {
		return Banner{}, false, nil
	}
	if err != nil || !b.Active || b.ImageURL == "" {
		return Banner{}, false, err
	}
	b.UpdatedAt = nil
	return b, true, nil
}

// CleanLink accepts a path on this site ("/search?q=prizm") or an http(s) address.
func CleanLink(raw string) (string, bool) {
	l := strings.TrimSpace(raw)
	if l == "" {
		return "", true
	}
	if strings.HasPrefix(l, "/") {
		return l, !strings.HasPrefix(l, "//") && !strings.ContainsAny(l, " \\")
	}
	u, err := url.Parse(l)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

// Save validates and stores a banner. isPhoto reports whether an image URL
// is a photo uploaded to this site.
func (s *Service) Save(ctx context.Context, tx *sql.Tx, b Banner, by string, isPhoto func(string) bool) (Banner, error) {
	b.ImageURL = strings.TrimSpace(b.ImageURL)
	b.AltText = strings.TrimSpace(b.AltText)
	link, ok := CleanLink(b.LinkURL)
	if !ok {
		return Banner{}, &FieldError{"link_url", "use a page on this site starting with / or a full web address starting with https://"}
	}
	b.LinkURL = link
	if b.ImageURL != "" && !isPhoto(b.ImageURL) {
		return Banner{}, &FieldError{"image_url", "upload the banner image here"}
	}
	if b.Active && b.ImageURL == "" {
		return Banner{}, &FieldError{"image_url", "upload a banner image before showing it"}
	}
	if len([]rune(b.AltText)) > 150 {
		return Banner{}, &FieldError{"alt_text", "keep the description under 150 characters"}
	}
	if b.Active && b.AltText == "" {
		return Banner{}, &FieldError{"alt_text", "describe the banner in a few words for people using screen readers"}
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE site_banners SET image_url = $2, link_url = $3, alt_text = $4, active = $5, updated_by = $6, updated_at = now()
		WHERE slot = $1`, b.Slot, b.ImageURL, b.LinkURL, b.AltText, b.Active, by)
	if err != nil {
		return Banner{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Banner{}, ErrNotFound
	}
	now := time.Now()
	b.UpdatedAt = &now
	return b, nil
}
