// Package httpapi is the marketplace's JSON HTTP API.
package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

const sessionCookie = "mp_session"

type Server struct {
	db           *sql.DB
	auth         *auth.Service
	log          *slog.Logger
	cookieSecure bool
	sessionTTL   time.Duration
	trustProxy   bool
	authLimiter  *rateLimiter

	sellers       *sellers.Service
	listings      *listings.Service
	publicURL     string
	webhookSecret string
}

type Options struct {
	DB           *sql.DB
	Auth         *auth.Service
	Log          *slog.Logger
	CookieSecure bool
	SessionTTL   time.Duration
	TrustProxy   bool

	Sellers       *sellers.Service
	Listings      *listings.Service
	PublicURL     string // base URL for Stripe return links; derived from the request if empty
	WebhookSecret string // Stripe webhook signing secret
}

func New(o Options) *Server {
	return &Server{
		db:            o.DB,
		auth:          o.Auth,
		log:           o.Log,
		cookieSecure:  o.CookieSecure,
		sessionTTL:    o.SessionTTL,
		trustProxy:    o.TrustProxy,
		sellers:       o.Sellers,
		listings:      o.Listings,
		publicURL:     o.PublicURL,
		webhookSecret: o.WebhookSecret,
		// 10 login/signup attempts per IP per 5 minutes.
		authLimiter: newRateLimiter(10, 5*time.Minute),
	}
}

// Handler returns the full HTTP handler with routes and middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.HandleFunc("POST /v1/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /v1/me", requireUser(s.handleMe))

	mux.HandleFunc("GET /v1/admin/users", requireAdmin(s.handleAdminUsers))

	// Selling: Stripe Connect onboarding and settings.
	mux.HandleFunc("POST /v1/seller/onboarding", requireUser(s.handleStartOnboarding))
	mux.HandleFunc("GET /v1/seller", requireUser(s.handleGetSeller))
	mux.HandleFunc("PATCH /v1/seller/settings", requireUser(s.handleSellerSettings))
	mux.HandleFunc("GET /seller/onboarding/return", s.handleOnboardingReturn)
	mux.HandleFunc("GET /seller/onboarding/refresh", s.handleOnboardingRefresh)
	mux.HandleFunc("POST /v1/webhooks/stripe", s.handleStripeWebhook)

	// Listings: seller management.
	mux.HandleFunc("POST /v1/seller/listings", requireUser(s.handleCreateListing))
	mux.HandleFunc("GET /v1/seller/listings", requireUser(s.handleMyListings))
	mux.HandleFunc("GET /v1/seller/listings/{id}", requireUser(s.handleMyListing))
	mux.HandleFunc("PATCH /v1/seller/listings/{id}", requireUser(s.handleUpdateListing))
	mux.HandleFunc("DELETE /v1/seller/listings/{id}", requireUser(s.handleRemoveListing))
	mux.HandleFunc("PUT /v1/seller/listings/external/{externalID}", requireUser(s.handleUpsertExternal))
	mux.HandleFunc("POST /v1/seller/listings/import", requireUser(s.handleImportCSV))
	mux.HandleFunc("GET /v1/seller/listings/import/template", s.handleCSVTemplate)

	// Listings: public browsing.
	mux.HandleFunc("GET /v1/listings", s.handleSearch)
	mux.HandleFunc("GET /v1/listings/facets", s.handleFacets)
	mux.HandleFunc("GET /demo/card/{name}", handleDemoCard)
	mux.HandleFunc("GET /v1/listings/{id}", s.handleGetListing)

	var h http.Handler = mux
	h = s.loadUser(h)
	h = requireJSONForWrites(h)
	h = requestLogger(s.log, h)
	h = recoverer(s.log, h)
	return h
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db_down", "database unreachable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("internal error", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "something went wrong", nil)
}

type signupRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !s.authLimiter.allow("signup:" + clientIP(r, s.trustProxy)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in a few minutes", nil)
		return
	}
	var req signupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.auth.Signup(r.Context(), req.Email, req.Username, req.Password)
	var verr *auth.ValidationError
	switch {
	case errors.As(err, &verr):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "check the highlighted fields", verr.Fields)
		return
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "that email is already registered", map[string]string{"email": "already registered"})
		return
	case errors.Is(err, auth.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "that username is taken", map[string]string{"username": "already taken"})
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}

	token, err := s.auth.StartSession(r.Context(), u.ID, clientIP(r, s.trustProxy), r.UserAgent())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{"user": u})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.authLimiter.allow("login:" + clientIP(r, s.trustProxy)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in a few minutes", nil)
		return
	}
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, token, err := s.auth.Login(r.Context(), req.Email, req.Password, clientIP(r, s.trustProxy), r.UserAgent())
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect", nil)
		return
	case errors.Is(err, auth.ErrAccountDisabled):
		writeError(w, http.StatusForbidden, "account_disabled", "this account is suspended or banned", nil)
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.auth.Logout(r.Context(), c.Value); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.auth.ListUsers(r.Context(), 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}
