package httpapi

import (
	"errors"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/shopping"
)

func (s *Server) savedError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, shopping.ErrSavedMissing):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, shopping.ErrSavedExists):
		writeError(w, http.StatusConflict, "already_saved", err.Error(), nil)
	case errors.Is(err, shopping.ErrSavedFull), errors.Is(err, shopping.ErrSavedEmpty), errors.Is(err, shopping.ErrSavedBad):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error(), nil)
	default:
		s.internalError(w, r, err)
	}
}

// GET /v1/saved-searches
func (s *Server) handleSavedSearches(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	list, err := s.shopping.SavedSearches(r.Context(), u.ID)
	if err != nil {
		s.savedError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"searches": list})
}

// POST /v1/saved-searches {"query": "q=charizard&grader=PSA", "name": "", "notify": true}
func (s *Server) handleSaveSearch(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in struct {
		Query  string `json:"query"`
		Name   string `json:"name"`
		Notify *bool  `json:"notify"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	notify := in.Notify == nil || *in.Notify
	ss, err := s.shopping.SaveSearch(r.Context(), u.ID, in.Name, in.Query, notify)
	if err != nil {
		s.savedError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"search": ss})
}

// PATCH /v1/saved-searches/{id} {"notify": false} or {"seen": true}
func (s *Server) handleUpdateSaved(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in struct {
		Notify *bool `json:"notify"`
		Seen   bool  `json:"seen"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", shopping.ErrSavedMissing.Error(), nil)
		return
	}
	if in.Notify != nil {
		if err := s.shopping.SetSavedNotify(r.Context(), u.ID, id, *in.Notify); err != nil {
			s.savedError(w, r, err)
			return
		}
	}
	if in.Seen {
		if err := s.shopping.SeenSaved(r.Context(), u.ID, id); err != nil {
			s.savedError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /v1/saved-searches/{id}
func (s *Server) handleDeleteSaved(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", shopping.ErrSavedMissing.Error(), nil)
		return
	}
	if err := s.shopping.DeleteSaved(r.Context(), u.ID, id); err != nil {
		s.savedError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/watchlist/ids — just the listing IDs, for heart buttons.
func (s *Server) handleWatchIDs(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	rows, err := s.db.QueryContext(r.Context(), `SELECT listing_id FROM watchlist WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1000`, u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			s.internalError(w, r, err)
			return
		}
		ids = append(ids, id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": ids})
}
