// Package httpapi is the marketplace's JSON HTTP API.
package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/blocklist"
	"github.com/jconner5017-rgb/marketplace/internal/checkout"
	"github.com/jconner5017-rgb/marketplace/internal/fcm"
	"github.com/jconner5017-rgb/marketplace/internal/feedback"
	"github.com/jconner5017-rgb/marketplace/internal/fees"
	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/photos"
	"github.com/jconner5017-rgb/marketplace/internal/sales"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
	"github.com/jconner5017-rgb/marketplace/internal/shopping"
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

	shopping   *shopping.Service
	inbox      *inbox.Service
	msgLimiter *rateLimiter

	sales      *sales.Service
	bidLimiter *rateLimiter

	mfa        *admin.MFA
	auditLog   *admin.Audit
	users      *admin.Users
	moderation *admin.Moderation
	records    *admin.Records
	fraud      *admin.Fraud
	metrics    *admin.Metrics
	catalog    *listings.Catalog
	fees       *fees.Service
	checkout   *checkout.Service
	ipRules    *admin.IPRules
	standing   *admin.Standing
	fcm        *fcm.Service
	feedback   *feedback.Service
	photos     *photos.Service
	// shippingToken guards the carrier tracking webhook URL.
	shippingToken string
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

	Shopping *shopping.Service
	Inbox    *inbox.Service
	Sales    *sales.Service
	MFA      *admin.MFA
	Audit    *admin.Audit
	Catalog  *listings.Catalog
	Fees     *fees.Service
	Checkout *checkout.Service
	FCM      *fcm.Service
	Feedback *feedback.Service
	Photos   *photos.Service
	// ShippingWebhookToken must be in the tracking webhook URL (?token=).
	ShippingWebhookToken string
	// AdminIPAllowlistOff turns off the admin IP allowlist (break-glass).
	AdminIPAllowlistOff bool
}

func New(o Options) *Server {
	s := &Server{
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
		shopping:      o.Shopping,
		inbox:         o.Inbox,
		sales:         o.Sales,
		mfa:           o.MFA,
		auditLog:      o.Audit,
		users:         &admin.Users{DB: o.DB, Audit: o.Audit, OwnerEmails: o.Auth.AdminEmails},
		moderation:    &admin.Moderation{DB: o.DB, Audit: o.Audit},
		records:       &admin.Records{DB: o.DB, Audit: o.Audit},
		fraud:         &admin.Fraud{DB: o.DB, Audit: o.Audit},
		metrics:       &admin.Metrics{DB: o.DB},
		catalog:       o.Catalog,
		fees:          o.Fees,
		checkout:      o.Checkout,
		fcm:           o.FCM,
		feedback:      o.Feedback,
		photos:        o.Photos,
		shippingToken: o.ShippingWebhookToken,
		ipRules:       &admin.IPRules{DB: o.DB, Audit: o.Audit, Disabled: o.AdminIPAllowlistOff},
		// 30 bids or offers per user per minute.
		bidLimiter: newRateLimiter(30, time.Minute),
		// 20 messages per user per 10 minutes.
		msgLimiter: newRateLimiter(20, 10*time.Minute),
		// 10 login/signup attempts per IP per 5 minutes.
		authLimiter: newRateLimiter(10, 5*time.Minute),
	}
	s.standing = &admin.Standing{DB: o.DB, Audit: o.Audit, Users: s.users, Mod: s.moderation}
	return s
}

// Handler returns the full HTTP handler with routes and middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.HandleFunc("POST /v1/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /v1/me", requireUser(s.handleMe))

	// Staff back office. Everything but the sign-in check needs a recent
	// authenticator code and the right permission.
	mux.HandleFunc("GET /v1/admin/session", s.staffOnly(s.handleAdminSession))
	mux.HandleFunc("POST /v1/admin/mfa/setup", s.staffOnly(s.handleMFASetup))
	mux.HandleFunc("POST /v1/admin/mfa/confirm", s.staffOnly(s.handleMFAConfirm))
	mux.HandleFunc("POST /v1/admin/mfa/verify", s.staffOnly(s.handleMFAVerify))
	mux.HandleFunc("GET /v1/admin/dashboard", s.requireStaff("", s.handleAdminDashboard))
	mux.HandleFunc("GET /v1/admin/audit", s.requireStaff(admin.AuditView, s.handleAuditList))
	mux.HandleFunc("GET /v1/admin/audit/verify", s.requireStaff(admin.AuditView, s.handleAuditVerify))
	mux.HandleFunc("GET /v1/admin/users", s.requireStaff(admin.UsersView, s.handleAdminUsers))
	mux.HandleFunc("GET /v1/admin/users/{id}", s.requireStaff(admin.UsersView, s.handleAdminUser))
	mux.HandleFunc("PATCH /v1/admin/users/{id}", s.requireStaff(admin.UsersEdit, s.handleAdminEditUser))
	mux.HandleFunc("POST /v1/admin/users/{id}/status", s.requireStaff(admin.UsersRestrict, s.handleAdminUserStatus))
	mux.HandleFunc("POST /v1/admin/users/{id}/sign-out", s.requireStaff(admin.UsersRestrict, s.handleAdminSignOut))
	mux.HandleFunc("POST /v1/admin/users/{id}/notes", s.requireStaff(admin.UsersView, s.handleAdminNote))
	mux.HandleFunc("POST /v1/admin/users/{id}/mfa-reset", s.requireStaff(admin.StaffManage, s.handleAdminMFAReset))
	mux.HandleFunc("GET /v1/admin/ip-rules", s.requireStaff(admin.StaffManage, s.handleIPRules))
	mux.HandleFunc("POST /v1/admin/ip-rules", s.requireStaff(admin.StaffManage, s.handleAddIPRule))
	mux.HandleFunc("DELETE /v1/admin/ip-rules/{id}", s.requireStaff(admin.StaffManage, s.handleRemoveIPRule))
	mux.HandleFunc("POST /v1/admin/users/{id}/warn", s.requireStaff(admin.UsersRestrict, s.handleAdminWarn))
	mux.HandleFunc("POST /v1/admin/users/{id}/tier", s.requireStaff(admin.SellersReview, s.handleAdminTier))
	mux.HandleFunc("GET /v1/admin/appeals", s.requireStaff(admin.UsersRestrict, s.handleAdminAppeals))
	mux.HandleFunc("POST /v1/admin/appeals/{id}", s.requireStaff(admin.UsersRestrict, s.handleAdminDecideAppeal))
	mux.HandleFunc("GET /v1/admin/orders/{id}/evidence", s.requireStaff(admin.OrdersView, s.handleAdminEvidence))
	mux.HandleFunc("GET /v1/admin/outside-feedback", s.requireStaff(admin.SellersReview, s.handleAdminOutside))
	mux.HandleFunc("POST /v1/admin/outside-feedback/{id}", s.requireStaff(admin.SellersReview, s.handleAdminReviewOutside))
	mux.HandleFunc("GET /v1/admin/staff", s.requireStaff(admin.StaffManage, s.handleAdminStaff))
	mux.HandleFunc("POST /v1/admin/staff", s.requireStaff(admin.StaffManage, s.handleAdminSetStaff))
	mux.HandleFunc("GET /v1/admin/sellers", s.requireStaff(admin.SellersReview, s.handleAdminSellers))
	mux.HandleFunc("POST /v1/admin/sellers/{id}/review", s.requireStaff(admin.SellersReview, s.handleAdminReviewSeller))
	mux.HandleFunc("GET /v1/admin/reports", s.requireStaff(admin.ReportsManage, s.handleAdminReports))
	mux.HandleFunc("POST /v1/admin/reports/{id}/resolve", s.requireStaff(admin.ReportsManage, s.handleAdminResolveReport))
	mux.HandleFunc("GET /v1/admin/listings", s.requireStaff(admin.UsersView, s.handleAdminListings))
	mux.HandleFunc("POST /v1/admin/listings/{id}/moderate", s.requireStaff(admin.ListingsModerate, s.handleAdminModerate))
	mux.HandleFunc("GET /v1/admin/catalog", s.requireStaff(admin.UsersView, s.handleAdminCatalog))
	mux.HandleFunc("POST /v1/admin/catalog/categories", s.requireStaff(admin.CatalogEdit, s.handleAdminAddCategory))
	mux.HandleFunc("PATCH /v1/admin/catalog/categories/{id}", s.requireStaff(admin.CatalogEdit, s.handleAdminEditCategory))
	mux.HandleFunc("DELETE /v1/admin/catalog/categories/{id}", s.requireStaff(admin.CatalogEdit, s.handleAdminDeleteCategory))
	mux.HandleFunc("PUT /v1/admin/catalog/features/{key}", s.requireStaff(admin.CatalogEdit, s.handleAdminSaveFeature))
	mux.HandleFunc("DELETE /v1/admin/catalog/features/{key}", s.requireStaff(admin.CatalogEdit, s.handleAdminDeleteFeature))
	mux.HandleFunc("GET /v1/admin/fees", s.requireStaff(admin.OrdersView, s.handleAdminFees))
	mux.HandleFunc("POST /v1/admin/fees", s.requireStaff(admin.FeesEdit, s.handleAdminAddFees))
	mux.HandleFunc("DELETE /v1/admin/fees/{id}", s.requireStaff(admin.FeesEdit, s.handleAdminCancelFees))
	mux.HandleFunc("PATCH /v1/admin/fees/upgrades/{key}", s.requireStaff(admin.FeesEdit, s.handleAdminUpgrade))
	mux.HandleFunc("GET /v1/admin/orders", s.requireStaff(admin.OrdersView, s.handleAdminOrders))
	mux.HandleFunc("GET /v1/admin/orders/{id}", s.requireStaff(admin.OrdersView, s.handleAdminOrder))
	mux.HandleFunc("GET /v1/admin/cases", s.requireStaff(admin.CasesManage, s.handleAdminCases))
	mux.HandleFunc("GET /v1/admin/cases/{id}", s.requireStaff(admin.CasesManage, s.handleAdminCase))
	mux.HandleFunc("POST /v1/admin/cases", s.requireStaff(admin.CasesManage, s.handleAdminOpenCase))
	mux.HandleFunc("PATCH /v1/admin/cases/{id}", s.requireStaff(admin.CasesManage, s.handleAdminUpdateCase))
	mux.HandleFunc("POST /v1/admin/cases/{id}/notes", s.requireStaff(admin.CasesManage, s.handleAdminCaseNote))
	mux.HandleFunc("GET /v1/admin/refunds", s.requireStaff(admin.OrdersView, s.handleAdminRefunds))
	mux.HandleFunc("POST /v1/admin/refunds", s.requireStaff(admin.CasesManage, s.handleAdminRecordRefund))
	mux.HandleFunc("GET /v1/admin/feedback", s.requireStaff(admin.FeedbackModerate, s.handleAdminFeedback))
	mux.HandleFunc("POST /v1/admin/feedback/{id}/moderate", s.requireStaff(admin.FeedbackModerate, s.handleAdminModerateFeedback))
	mux.HandleFunc("GET /v1/admin/messages", s.requireStaff(admin.MessagesView, s.handleAdminThreads))
	mux.HandleFunc("POST /v1/admin/messages/{id}/read", s.requireStaff(admin.MessagesView, s.handleAdminThreadLog))
	mux.HandleFunc("GET /v1/admin/fraud", s.requireStaff(admin.ReportsManage, s.handleAdminFraud))
	mux.HandleFunc("POST /v1/admin/fraud/{id}/review", s.requireStaff(admin.UsersRestrict, s.handleAdminReviewFraud))
	mux.HandleFunc("POST /v1/admin/fraud/run", s.requireStaff(admin.UsersRestrict, s.handleAdminRunFraud))
	mux.HandleFunc("GET /v1/admin/kpis", s.requireStaff(admin.OrdersView, s.handleAdminKPIs))
	mux.HandleFunc("POST /v1/reports", requireUser(s.handleReport))
	mux.HandleFunc("GET /v1/fees", s.handlePublicFees)
	mux.HandleFunc("POST /v1/admin/announcements", s.requireStaff(admin.Announce, s.handleAnnounce))
	mux.HandleFunc("GET /v1/announcements", s.handleAnnouncements)

	// Signed-in buyer features: header summary, watchlist, cart, messages, bell.
	mux.HandleFunc("GET /v1/me/summary", requireUser(s.handleSummary))
	mux.HandleFunc("GET /v1/me/standing", requireUser(s.handleMyStanding))
	mux.HandleFunc("POST /v1/me/appeals", requireUser(s.handleMyAppeal))
	mux.HandleFunc("POST /v1/appeals/account", s.handleAccountAppeal)
	mux.HandleFunc("GET /v1/watchlist", requireUser(s.handleWatchlist))
	mux.HandleFunc("GET /v1/watchlist/ids", requireUser(s.handleWatchIDs))
	mux.HandleFunc("GET /v1/saved-searches", requireUser(s.handleSavedSearches))
	mux.HandleFunc("POST /v1/saved-searches", requireUser(s.handleSaveSearch))
	mux.HandleFunc("PATCH /v1/saved-searches/{id}", requireUser(s.handleUpdateSaved))
	mux.HandleFunc("DELETE /v1/saved-searches/{id}", requireUser(s.handleDeleteSaved))
	mux.HandleFunc("PUT /v1/watchlist/{id}", requireUser(s.handleWatch))
	mux.HandleFunc("DELETE /v1/watchlist/{id}", requireUser(s.handleUnwatch))
	mux.HandleFunc("GET /v1/cart", requireUser(s.handleCart))
	mux.HandleFunc("PUT /v1/cart/{id}", requireUser(s.handleSetCart))
	mux.HandleFunc("DELETE /v1/cart/{id}", requireUser(s.handleRemoveCart))
	mux.HandleFunc("GET /v1/listings/{id}/me", requireUser(s.handleListingForMe))
	mux.HandleFunc("POST /v1/listings/{id}/messages", requireUser(s.handleAskSeller))
	mux.HandleFunc("GET /v1/messages", requireUser(s.handleThreads))
	mux.HandleFunc("GET /v1/messages/{id}", requireUser(s.handleConversation))
	mux.HandleFunc("POST /v1/messages/{id}", requireUser(s.handleReply))
	mux.HandleFunc("GET /v1/notifications", requireUser(s.handleNotifications))

	// Auctions and Best Offer.
	mux.HandleFunc("POST /v1/listings/{id}/bids", requireUser(s.handleBid))
	mux.HandleFunc("GET /v1/listings/{id}/bids", s.handleBids)
	mux.HandleFunc("POST /v1/listings/{id}/buy-now", requireUser(s.handleAuctionBuyNow))
	mux.HandleFunc("POST /v1/listings/{id}/offers", requireUser(s.handleMakeOffer))
	mux.HandleFunc("POST /v1/offers/{id}/respond", requireUser(s.handleRespondOffer))
	mux.HandleFunc("GET /v1/offers", requireUser(s.handleOffers))
	mux.HandleFunc("GET /v1/checkout/status", s.handleCheckoutStatus)
	mux.HandleFunc("POST /v1/checkout", requireUser(s.handleStartCheckout))
	mux.HandleFunc("POST /v1/orders/{id}/confirm", requireUser(s.handleConfirmOrder))
	mux.HandleFunc("POST /v1/orders/{id}/cancel", requireUser(s.handleAbandonOrder))
	mux.HandleFunc("GET /v1/orders", s.sellerAPI(s.handleOrders))
	mux.HandleFunc("POST /v1/orders/{id}/oversold", s.sellerAPI(s.handleOversold))
	// FireCardManager and other inventory tools (API key or session).
	mux.HandleFunc("POST /v1/listings", s.sellerAPI(s.handleAPICreateListing))
	mux.HandleFunc("POST /v1/listings/bulk", s.sellerAPI(s.handleAPIBulk))
	mux.HandleFunc("PATCH /v1/listings/{externalID}", s.sellerAPI(s.handleAPIPatchListing))
	mux.HandleFunc("DELETE /v1/listings/{externalID}", s.sellerAPI(s.handleAPIDeleteListing))
	mux.HandleFunc("GET /v1/seller/shipping", requireUser(s.handleShippingSettings))
	mux.HandleFunc("PUT /v1/seller/shipping/from", requireUser(s.handleShipFrom))
	mux.HandleFunc("POST /v1/seller/shipping/card", requireUser(s.handleLabelCard))
	mux.HandleFunc("POST /v1/seller/shipping/card/finish", requireUser(s.handleLabelCardFinish))
	mux.HandleFunc("POST /v1/orders/{id}/label/rates", requireUser(s.handleLabelRates))
	mux.HandleFunc("POST /v1/orders/{id}/label", requireUser(s.handleBuyLabel))
	mux.HandleFunc("POST /v1/orders/{id}/label/void", requireUser(s.handleVoidLabel))
	mux.HandleFunc("POST /v1/webhooks/shipping/{provider}", s.handleShippingWebhook)
	mux.HandleFunc("POST /v1/feedback/{id}/reply", requireUser(s.handleFeedbackReply))
	mux.HandleFunc("POST /v1/orders/{id}/messages", requireUser(s.handleOrderMessage))
	mux.HandleFunc("GET /v1/members/{username}/feedback", s.handleMemberFeedback)
	mux.HandleFunc("GET /v1/members/{username}/buyer-record", requireUser(s.handleBuyerRecord))
	mux.HandleFunc("GET /v1/seller/feedback", requireUser(s.handleSellerFeedback))
	mux.HandleFunc("PUT /v1/seller/feedback", requireUser(s.handleSetSellerFeedback))
	mux.HandleFunc("POST /v1/seller/outside-feedback", requireUser(s.handleClaimOutside))
	mux.HandleFunc("GET /v1/seller/connect", requireUser(s.handleConnect))
	mux.HandleFunc("PUT /v1/seller/connect", requireUser(s.handleSetConnect))
	mux.HandleFunc("POST /v1/seller/connect/test", requireUser(s.handleTestWebhook))
	mux.HandleFunc("POST /v1/seller/api-keys", requireUser(s.handleCreateKey))
	mux.HandleFunc("DELETE /v1/seller/api-keys/{id}", requireUser(s.handleRevokeKey))
	mux.HandleFunc("GET /v1/orders/{id}", requireUser(s.handleOrder))
	mux.HandleFunc("POST /v1/orders/{id}/ship", requireUser(s.handleShipOrder))
	mux.HandleFunc("POST /v1/orders/{id}/delivered", requireUser(s.handleOrderDelivered))
	mux.HandleFunc("POST /v1/orders/{id}/feedback", requireUser(s.handleOrderFeedback))
	mux.HandleFunc("POST /v1/orders/{id}/problem", requireUser(s.handleOrderProblem))
	mux.HandleFunc("GET /v1/holds", requireUser(s.handleHolds))
	mux.HandleFunc("POST /v1/notifications/read-all", requireUser(s.handleReadAll))
	mux.HandleFunc("POST /v1/notifications/{id}/read", requireUser(s.handleReadOne))

	// Selling: Stripe Connect onboarding and settings.
	mux.HandleFunc("POST /v1/seller/onboarding", requireUser(s.handleStartOnboarding))
	mux.HandleFunc("GET /v1/seller", requireUser(s.handleGetSeller))
	mux.HandleFunc("PATCH /v1/seller/settings", requireUser(s.handleSellerSettings))
	mux.HandleFunc("PATCH /v1/seller/tax", requireUser(s.handleSellerTax))
	mux.HandleFunc("GET /v1/seller/reports/sales.csv", requireUser(s.handleSalesCSV))
	mux.HandleFunc("GET /v1/seller/reports/summary", requireUser(s.handleSalesSummary))
	mux.HandleFunc("GET /v1/orders/{id}/evidence", requireUser(s.handleEvidence))
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
	mux.HandleFunc("GET /v1/catalog", handleCatalog)
	mux.HandleFunc("GET /demo/card/{name}", handleDemoCard)
	mux.HandleFunc("POST /v1/photos", s.sellerAPI(s.handleUploadPhoto))
	mux.HandleFunc("GET /photos/{file}", s.handlePhoto)
	mux.HandleFunc("GET /v1/listings/{id}", s.handleGetListing)

	var h http.Handler = mux
	h = frozenGuard(h)
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
	case errors.Is(err, blocklist.ErrBlocked):
		writeError(w, http.StatusForbidden, "email_blocked", err.Error(), map[string]string{"email": err.Error()})
		return
	case errors.Is(err, auth.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "that username is taken", map[string]string{"username": "already taken"})
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}

	ip := clientIP(r, s.trustProxy)
	if hit, banned, err := blocklist.Has(r.Context(), s.db, "ip", ip); err == nil && hit {
		if err := blocklist.Alert(r.Context(), s.db, u.ID, "sign-up address", banned, "IP "+ip); err != nil {
			s.log.Error("banned ip alert", "err", err)
		}
	}
	token, err := s.auth.StartSession(r.Context(), u.ID, ip, r.UserAgent())
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
		writeError(w, http.StatusForbidden, "account_disabled", "this account is suspended or banned; you can appeal once from the sign-in page", nil)
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
