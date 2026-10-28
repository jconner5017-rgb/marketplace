package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

func (s *Server) listingError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *listings.ValidationError
	var ie *listings.ImportError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "check the highlighted fields", ve.Fields)
	case errors.As(err, &ie):
		writeError(w, http.StatusUnprocessableEntity, "invalid_csv", ie.Msg, nil)
	case errors.Is(err, listings.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "listing not found", nil)
	case errors.Is(err, listings.ErrCannotSell):
		writeError(w, http.StatusForbidden, "onboarding_incomplete", "finish Stripe onboarding before listing cards", nil)
	case errors.Is(err, listings.ErrDuplicateExtern):
		writeError(w, http.StatusConflict, "external_id_taken", "you already have a listing with that external_id",
			map[string]string{"external_id": "already in use"})
	case errors.Is(err, sellers.ErrNotSeller):
		writeError(w, http.StatusForbidden, "not_a_seller", "start seller onboarding first", nil)
	default:
		s.internalError(w, r, err)
	}
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}
	return true
}

func listingID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "listing not found", nil)
		return "", false
	}
	return id, true
}

func (s *Server) handleCreateListing(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in listings.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	l, err := s.listings.Create(r.Context(), u.ID, in)
	if err != nil {
		s.listingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"listing": l})
}

func (s *Server) handleMyListings(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	limit, offset := pageParams(r)
	ls, total, err := s.listings.ListForSeller(r.Context(), u.ID, limit, offset)
	if err != nil {
		s.listingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"listings": ls, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleMyListing(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	l, err := s.listings.GetForSeller(r.Context(), u.ID, id)
	if err != nil {
		s.listingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"listing": l})
}

func (s *Server) handleUpdateListing(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
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
	writeJSON(w, http.StatusOK, map[string]any{"listing": l})
}

func (s *Server) handleRemoveListing(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	if err := s.listings.Remove(r.Context(), u.ID, id); err != nil {
		s.listingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/seller/listings/external/{externalID} — create or replace by the
// seller's own SKU. This is the call FireCardManager will use.
func (s *Server) handleUpsertExternal(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	ext := strings.TrimSpace(r.PathValue("externalID"))
	var in listings.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ExternalID != nil && strings.TrimSpace(*in.ExternalID) != ext {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "external_id in the body must match the URL",
			map[string]string{"external_id": "does not match URL"})
		return
	}
	in.ExternalID = &ext
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

// POST /v1/seller/listings/import with Content-Type: text/csv
func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "text/csv" {
		writeError(w, http.StatusUnsupportedMediaType, "csv_required", "send the file with Content-Type: text/csv", nil)
		return
	}
	body := http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	res, err := s.listings.ImportCSV(r.Context(), u.ID, body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "the file is over 10 MB", nil)
			return
		}
		s.listingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

const csvTemplate = "external_id,category,game_or_sport,set_name,year,card_number,title,description,condition,grader,grade,cert_number,attributes,photo_urls,price,quantity,package_preset,weight_oz,status\r\n" +
	"SKU-0001,tcg,Pokemon,Base Set,1999,4/102,Charizard Holo Base Set,Clean surface,LP,,,,holo,https://example.com/front.jpg|https://example.com/back.jpg,18.50,1,envelope,1,active\r\n" +
	"SKU-0002,sports,Baseball,Topps Chrome,2018,150,Shohei Ohtani Rookie PSA 10,,,PSA,10,12345678,rookie,https://example.com/ohtani.jpg,19.99,1,bubble,3,active\r\n"

func (s *Server) handleCSVTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="listings-template.csv"`)
	w.Write([]byte(csvTemplate))
}

func pageParams(r *http.Request) (limit, offset int) {
	q := r.URL.Query()
	limit, _ = strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 48
	}
	offset, _ = strconv.Atoi(q.Get("offset"))
	if offset < 0 || offset > 10000 {
		offset = 0
	}
	return limit, offset
}

// parseSearch reads search filters from the query string. It writes a 400
// and returns false when a filter is malformed.
func parseSearch(w http.ResponseWriter, r *http.Request) (listings.SearchParams, bool) {
	q := r.URL.Query()
	limit, offset := pageParams(r)
	p := listings.SearchParams{
		Query:       q.Get("q"),
		Category:    q.Get("category"),
		GameOrSport: q.Get("game"),
		SetName:     q.Get("set"),
		Grader:      q.Get("grader"),
		Condition:   q.Get("condition"),
		Attribute:   q.Get("attribute"),
		Seller:      q.Get("seller"),
		Sort:        q.Get("sort"),
		Limit:       limit,
		Offset:      offset,
	}
	if len(p.Query) > 200 {
		writeError(w, http.StatusBadRequest, "bad_query", "search text is too long", nil)
		return p, false
	}
	bad := map[string]string{}
	if v := q.Get("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil {
			bad["year"] = "must be a year"
		}
		p.Year = y
	}
	if v := q.Get("min_grade"); v != "" {
		g, err := strconv.ParseFloat(v, 64)
		if err != nil {
			bad["min_grade"] = "must be a number"
		}
		p.MinGrade = g
	}
	for key, dst := range map[string]*int{"min_price": &p.MinCents, "max_price": &p.MaxCents} {
		if v := q.Get(key); v != "" {
			c, err := dollarsToCents(v)
			if err != nil {
				bad[key] = "must be a dollar amount"
			}
			*dst = c
		}
	}
	switch p.Sort {
	case "", "newest", "price_asc", "price_desc", "relevance":
	default:
		bad["sort"] = "newest, price_asc, price_desc or relevance"
	}
	if len(bad) > 0 {
		writeError(w, http.StatusBadRequest, "bad_query", "check the search filters", bad)
		return p, false
	}
	return p, true
}

// GET /v1/listings?q=charizard&category=tcg&game=Pokemon&grader=PSA&min_price=5&max_price=50&sort=price_asc
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	p, ok := parseSearch(w, r)
	if !ok {
		return
	}
	res, err := s.listings.Search(r.Context(), p)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /v1/listings/facets — same filters as search; counts per sidebar value.
func (s *Server) handleFacets(w http.ResponseWriter, r *http.Request) {
	p, ok := parseSearch(w, r)
	if !ok {
		return
	}
	f, err := s.listings.Facets(r.Context(), p)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func dollarsToCents(v string) (int, error) {
	f, err := strconv.ParseFloat(strings.TrimPrefix(v, "$"), 64)
	if err != nil || f < 0 || f > 1_000_000 {
		return 0, errors.New("bad amount")
	}
	return int(f*100 + 0.5), nil
}

func (s *Server) handleGetListing(w http.ResponseWriter, r *http.Request) {
	id, ok := listingID(w, r)
	if !ok {
		return
	}
	l, err := s.listings.GetPublic(r.Context(), id)
	if err != nil {
		s.listingError(w, r, err)
		return
	}
	l.ExternalID = nil
	writeJSON(w, http.StatusOK, map[string]any{"listing": l})
}

// GET /v1/catalog — the fixed lists the site and FireCardManager use for menus.
func handleCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{
		"sports":     listings.Sports,
		"games":      listings.Games,
		"conditions": listings.Conditions,
		"graders":    listings.Graders,
		"attributes": listings.Attributes,
	})
}
