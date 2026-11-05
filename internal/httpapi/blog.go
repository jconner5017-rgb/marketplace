package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jconner5017-rgb/marketplace/internal/blog"
)

func (s *Server) blogError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *blog.FieldError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
	case errors.Is(err, blog.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "post not found", nil)
	default:
		s.internalError(w, r, err)
	}
}

// GET /v1/blog
func (s *Server) handleBlog(w http.ResponseWriter, r *http.Request) {
	list, err := s.blog.Published(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": list})
}

// GET /v1/blog/{slug}
func (s *Server) handleBlogPost(w http.ResponseWriter, r *http.Request) {
	p, err := s.blog.BySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.blogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"post": p})
}

// GET /v1/admin/blog
func (s *Server) handleAdminBlog(w http.ResponseWriter, r *http.Request) {
	list, err := s.blog.All(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": list})
}

func blogID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "post not found", nil)
		return 0, false
	}
	return id, true
}

// GET /v1/admin/blog/{id}
func (s *Server) handleAdminBlogPost(w http.ResponseWriter, r *http.Request) {
	id, ok := blogID(w, r)
	if !ok {
		return
	}
	p, err := s.blog.Get(r.Context(), id)
	if err != nil {
		s.blogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"post": p})
}

// POST /v1/admin/blog and PATCH /v1/admin/blog/{id}
func (s *Server) handleAdminSaveBlog(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var p blog.Post
	if !decodeJSON(w, r, &p) {
		return
	}
	p.ID = 0
	action := "blog.created"
	if r.PathValue("id") != "" {
		id, ok := blogID(w, r)
		if !ok {
			return
		}
		p.ID, action = id, "blog.updated"
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	saved, err := s.blog.Save(r.Context(), tx, p, u.ID)
	if err != nil {
		s.blogError(w, r, err)
		return
	}
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, action, "blog_post", strconv.Itoa(saved.ID), "",
		map[string]any{"slug": saved.Slug, "title": saved.Title, "status": saved.Status})); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	status := http.StatusOK
	if action == "blog.created" {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"post": saved})
}

// DELETE /v1/admin/blog/{id}
func (s *Server) handleAdminDeleteBlog(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, ok := blogID(w, r)
	if !ok {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	if err := s.blog.Delete(r.Context(), tx, id); err != nil {
		s.blogError(w, r, err)
		return
	}
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, "blog.deleted", "blog_post", strconv.Itoa(id), "", nil)); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
