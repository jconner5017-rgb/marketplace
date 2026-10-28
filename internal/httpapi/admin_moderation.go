package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/fees"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

type txLike = *sql.Tx

// ---- Member reports ----

// POST /v1/reports {"target_type","target_id","reason","details"}
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Reason     string `json:"reason"`
		Details    string `json:"details"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.msgLimiter.allow("report:" + u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many reports at once; try again later", nil)
		return
	}
	err := s.moderation.Submit(r.Context(), u.ID, req.TargetType, req.TargetID, req.Reason, req.Details)
	switch {
	case errors.Is(err, admin.ErrReportTarget):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, admin.ErrReportOwn):
		writeError(w, http.StatusUnprocessableEntity, "own", err.Error(), nil)
	case err != nil:
		s.adminError(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
	}
}

// GET /v1/admin/reports?status=open&type=listing&target=<id>
func (s *Server) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = "open"
	}
	if status == "all" {
		status = ""
	}
	rs, err := s.moderation.Reports(r.Context(), status, q.Get("type"), q.Get("target"), 0)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": rs, "reasons": admin.ReportReasons})
}

// POST /v1/admin/reports/{id}/resolve {"status":"actioned"|"dismissed","note","all":true}
func (s *Server) handleAdminResolveReport(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
		All    bool   `json:"all"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	n, err := s.moderation.Resolve(r.Context(), s.actor(r), id, req.Status, req.Note, req.All)
	if errors.Is(err, admin.ErrAlreadyHandled) {
		writeError(w, http.StatusConflict, "handled", err.Error(), nil)
		return
	}
	if err != nil {
		if errors.Is(err, admin.ErrReason) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error(), map[string]string{"note": err.Error()})
			return
		}
		s.adminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"closed": n})
}

// ---- Listings ----

// GET /v1/admin/listings?q=&status=&seller=&format=&reported=1&min_price=&max_price=
func (s *Server) handleAdminListings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := pageParams(r)
	lq := admin.ListingQuery{Q: q.Get("q"), Status: q.Get("status"), Seller: q.Get("seller"), Format: q.Get("format"),
		Reported: q.Get("reported") == "1", Limit: limit, Offset: offset}
	if v := q.Get("min_price"); v != "" {
		lq.MinCents, _ = dollarsToCents(v)
	}
	if v := q.Get("max_price"); v != "" {
		lq.MaxCents, _ = dollarsToCents(v)
	}
	rows, total, err := s.moderation.Listings(r.Context(), lq)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"listings": rows, "total": total, "limit": limit, "offset": offset})
}

// POST /v1/admin/listings/{id}/moderate {"action":"remove"|"restore","reason"}
func (s *Server) handleAdminModerate(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.moderation.ModerateListing(r.Context(), s.actor(r), id, req.Action, req.Reason); err != nil {
		s.adminError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Catalog ----

func (s *Server) catalogError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *listings.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "check the highlighted fields", ve.Fields)
	case errors.Is(err, listings.ErrCatalogNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, listings.ErrCatalogInUse), errors.Is(err, listings.ErrCatalogDup):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	default:
		s.adminError(w, r, err)
	}
}

// GET /v1/admin/catalog — every sport, game and feature with listing counts.
func (s *Server) handleAdminCatalog(w http.ResponseWriter, r *http.Request) {
	cats, err := s.catalog.Categories(r.Context(), true)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	feats, err := s.catalog.Features(r.Context(), true)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats, "features": feats})
}

// catalogChange runs a catalog edit in a transaction with its audit entry,
// then reloads the catalog listings are checked against.
func (s *Server) catalogChange(w http.ResponseWriter, r *http.Request, action, target, reason string,
	fn func(tx txLike) (map[string]any, error)) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	details, err := fn(tx)
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	u, _ := currentUser(r)
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, action, "catalog", target, reason, details)); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.catalog.Load(r.Context()); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/admin/catalog/categories {kind,value,label,short_label,aliases,active,sort}
func (s *Server) handleAdminAddCategory(w http.ResponseWriter, r *http.Request) {
	var in listings.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.catalogChange(w, r, "catalog.category_added", in.Kind+":"+in.Value, "", func(tx txLike) (map[string]any, error) {
		c, err := s.catalog.AddCategory(r.Context(), tx, in)
		return map[string]any{"category": c}, err
	})
}

// PATCH /v1/admin/catalog/categories/{id}
func (s *Server) handleAdminEditCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	var in listings.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	s.catalogChange(w, r, "catalog.category_changed", strconv.Itoa(id), "", func(tx txLike) (map[string]any, error) {
		before, after, err := s.catalog.UpdateCategory(r.Context(), tx, id, in)
		return map[string]any{"before": before, "after": after}, err
	})
}

// DELETE /v1/admin/catalog/categories/{id}
func (s *Server) handleAdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	s.catalogChange(w, r, "catalog.category_deleted", strconv.Itoa(id), "", func(tx txLike) (map[string]any, error) {
		c, err := s.catalog.DeleteCategory(r.Context(), tx, id)
		return map[string]any{"category": c.Kind + ":" + c.Value}, err
	})
}

// PUT /v1/admin/catalog/features/{key} {label,active,sort}
func (s *Server) handleAdminSaveFeature(w http.ResponseWriter, r *http.Request) {
	var f listings.Feature
	if !decodeJSON(w, r, &f) {
		return
	}
	f.Key = r.PathValue("key")
	s.catalogChange(w, r, "catalog.feature_saved", f.Key, "", func(tx txLike) (map[string]any, error) {
		return map[string]any{"feature": f}, s.catalog.SaveFeature(r.Context(), tx, f)
	})
}

// DELETE /v1/admin/catalog/features/{key}
func (s *Server) handleAdminDeleteFeature(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	s.catalogChange(w, r, "catalog.feature_deleted", key, "", func(tx txLike) (map[string]any, error) {
		return nil, s.catalog.DeleteFeature(r.Context(), tx, key)
	})
}

// ---- Fees ----

// GET /v1/fees — the fee schedule in force (public; the listing form uses it).
func (s *Server) handlePublicFees(w http.ResponseWriter, r *http.Request) {
	cur, err := s.fees.Current(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	next, err := s.fees.Upcoming(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	for i := range next {
		next[i].CreatedBy = nil
	}
	cur.CreatedBy = nil
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"current": cur, "upcoming": next})
}

// GET /v1/admin/fees — current, history and upgrades.
func (s *Server) handleAdminFees(w http.ResponseWriter, r *http.Request) {
	cur, err := s.fees.Current(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	hist, err := s.fees.History(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ups, err := s.fees.Upgrades(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": cur, "history": hist, "upgrades": ups})
}

func (s *Server) feeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *fees.FieldError
	if errors.As(err, &fe) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", fe.Msg, map[string]string{fe.Field: fe.Msg})
		return
	}
	s.adminError(w, r, err)
}

// POST /v1/admin/fees {flat_cents, threshold_cents, percent_bps, cap_cents, effective_at, note}
func (s *Server) handleAdminAddFees(w http.ResponseWriter, r *http.Request) {
	var req struct {
		fees.Schedule
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.EffectiveAt.IsZero() {
		req.EffectiveAt = time.Now()
	}
	u, _ := currentUser(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	before, _ := s.fees.Current(r.Context())
	if req.Note == "" {
		req.Note = req.Reason
	}
	sch, err := s.fees.Add(r.Context(), tx, req.Schedule, u.ID)
	if err != nil {
		s.feeError(w, r, err)
		return
	}
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, "fees.scheduled", "fees", strconv.Itoa(sch.ID), req.Note,
		map[string]any{"before": before, "after": sch})); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"schedule": sch})
}

// DELETE /v1/admin/fees/{id} — cancel fees that haven't started.
func (s *Server) handleAdminCancelFees(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	u, _ := currentUser(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	if err := s.fees.CancelUpcoming(r.Context(), tx, id); err != nil {
		s.feeError(w, r, err)
		return
	}
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, "fees.cancelled", "fees", strconv.Itoa(id), "", nil)); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /v1/admin/fees/upgrades/{key} {label, description, price_cents, active}
func (s *Server) handleAdminUpgrade(w http.ResponseWriter, r *http.Request) {
	var up fees.Upgrade
	if !decodeJSON(w, r, &up) {
		return
	}
	up.Key = r.PathValue("key")
	u, _ := currentUser(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback()
	old, err := s.fees.UpdateUpgrade(r.Context(), tx, up)
	if err != nil {
		s.feeError(w, r, err)
		return
	}
	if err := s.auditLog.Log(r.Context(), tx, s.entry(r, u, "fees.upgrade_changed", "upgrade", up.Key, "",
		map[string]any{"before": old, "after": up})); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
