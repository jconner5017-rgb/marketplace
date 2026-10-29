package httpapi

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/sellers"
	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

// baseURL is where Stripe sends sellers back after onboarding.
func (s *Server) baseURL(r *http.Request) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	scheme := "http"
	if r.TLS != nil || (s.trustProxy && r.Header.Get("X-Forwarded-Proto") == "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) sellerError(w http.ResponseWriter, r *http.Request, err error) {
	var se *stripe.Error
	switch {
	case errors.Is(err, sellers.ErrNotSeller):
		writeError(w, http.StatusNotFound, "not_a_seller", "start seller onboarding first", nil)
	case errors.Is(err, sellers.ErrStripeNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "stripe_not_configured", "selling is not available yet", nil)
	case errors.As(err, &se):
		s.log.Error("stripe error", "path", r.URL.Path, "status", se.Status, "type", se.Type, "msg", se.Message)
		writeError(w, http.StatusBadGateway, "stripe_error", "Stripe could not complete the request; try again", nil)
	default:
		s.internalError(w, r, err)
	}
}

// POST /v1/seller/onboarding → {"url": "https://connect.stripe.com/..."}
// The client sends the browser to that URL.
func (s *Server) handleStartOnboarding(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	// Admin test mode: no Stripe until checkout opens.
	if u.IsAdmin() && s.checkout != nil && s.checkout.TestMode() {
		if err := s.sellers.StartTestSeller(r.Context(), u.ID); err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"url": "/sell?test_seller=1", "test": true})
		return
	}
	base := s.baseURL(r)
	link, err := s.sellers.StartOnboarding(r.Context(), u.ID, u.Email,
		base+"/seller/onboarding/refresh", base+"/seller/onboarding/return")
	if err != nil {
		s.sellerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": link})
}

// GET /v1/seller → seller profile, refreshed from Stripe while onboarding
// is unfinished.
func (s *Server) handleGetSeller(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	p, err := s.sellers.Get(r.Context(), u.ID)
	if err != nil {
		s.sellerError(w, r, err)
		return
	}
	if !p.ChargesEnabled && s.sellers.Stripe != nil {
		if fresh, err := s.sellers.Refresh(r.Context(), u.ID); err == nil {
			p = fresh
		} else {
			s.log.Warn("seller refresh failed", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"seller": p})
}

type sellerSettingsRequest struct {
	BundleDiscountType *string `json:"bundle_discount_type"`
	BundleDiscountVal  *int    `json:"bundle_discount_val"`
}

func (s *Server) handleSellerSettings(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req sellerSettingsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := s.sellers.UpdateBundleDiscount(r.Context(), u.ID, req.BundleDiscountType, req.BundleDiscountVal)
	var ve *sellers.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "check the highlighted fields", ve.Fields)
		return
	}
	if err != nil {
		s.sellerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"seller": p})
}

// PATCH /v1/seller/tax {"collect_tax": true}
func (s *Server) handleSellerTax(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var req struct {
		CollectTax bool `json:"collect_tax"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := s.sellers.SetCollectTax(r.Context(), u.ID, req.CollectTax)
	if err != nil {
		s.sellerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"seller": p})
}

var onboardingPage = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#222}
h1{font-size:1.4rem}</style></head>
<body><h1>{{.Title}}</h1><p>{{.Body}}</p></body></html>`))

func renderPage(w http.ResponseWriter, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	onboardingPage.Execute(w, map[string]string{"Title": title, "Body": body})
}

// GET /seller/onboarding/return — Stripe sends the seller here when they
// finish (or leave) onboarding.
func (s *Server) handleOnboardingReturn(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(r)
	if !ok {
		renderPage(w, http.StatusOK, "Back from Stripe", "Log in to the marketplace to see your seller status.")
		return
	}
	p, err := s.sellers.Refresh(r.Context(), u.ID)
	switch {
	case err != nil:
		s.log.Error("onboarding return refresh", "err", err)
		renderPage(w, http.StatusOK, "Back from Stripe", "We couldn't check your Stripe status just now. It will update shortly.")
	case p.ChargesEnabled:
		renderPage(w, http.StatusOK, "You're ready to sell", "Your Stripe account is connected. You can start listing cards.")
	case p.DetailsSubmitted:
		renderPage(w, http.StatusOK, "Almost there", "Stripe is reviewing your details. Selling unlocks as soon as they approve.")
	default:
		renderPage(w, http.StatusOK, "Setup not finished", "Your Stripe setup isn't complete yet. Start seller onboarding again to finish it.")
	}
}

// GET /seller/onboarding/refresh — Stripe sends the seller here when an
// onboarding link expires; we make a new one and send them back.
func (s *Server) handleOnboardingRefresh(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(r)
	if !ok {
		renderPage(w, http.StatusUnauthorized, "Please log in", "Log in to the marketplace, then start seller onboarding again.")
		return
	}
	base := s.baseURL(r)
	link, err := s.sellers.StartOnboarding(r.Context(), u.ID, u.Email,
		base+"/seller/onboarding/refresh", base+"/seller/onboarding/return")
	if err != nil {
		s.log.Error("onboarding refresh", "err", err)
		renderPage(w, http.StatusBadGateway, "Something went wrong", "We couldn't reach Stripe. Please try again in a minute.")
		return
	}
	http.Redirect(w, r, link, http.StatusSeeOther)
}

// POST /v1/webhooks/stripe — Connect events from Stripe.
func (s *Server) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if s.webhookSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "webhooks_disabled", "STRIPE_WEBHOOK_SECRET is not set", nil)
		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "payload too large", nil)
		return
	}
	// Two Stripe endpoints can point here (connected-account events and our
	// own account's events), each with its own secret: STRIPE_WEBHOOK_SECRET
	// may hold both, comma-separated.
	var ev stripe.Event
	err = stripe.ErrBadSignature
	for _, secret := range strings.Split(s.webhookSecret, ",") {
		if secret = strings.TrimSpace(secret); secret == "" {
			continue
		}
		if ev, err = stripe.VerifyWebhook(payload, r.Header.Get("Stripe-Signature"), secret, 5*time.Minute, time.Now()); err == nil {
			break
		}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_signature", "invalid signature", nil)
		return
	}
	fresh, err := s.sellers.MarkEventProcessed(r.Context(), ev.ID, ev.Type)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !fresh {
		writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}

	switch ev.Type {
	case "account.updated":
		var acct stripe.Account
		if err := json.Unmarshal(ev.Data.Object, &acct); err != nil || !strings.HasPrefix(acct.ID, "acct_") {
			writeError(w, http.StatusBadRequest, "bad_event", "could not read account", nil)
			return
		}
		if err := s.sellers.ApplyAccount(r.Context(), acct); err != nil {
			s.internalError(w, r, err)
			return
		}
	case "account.application.deauthorized":
		if ev.Account != "" {
			if err := s.sellers.Disconnect(r.Context(), ev.Account); err != nil {
				s.internalError(w, r, err)
				return
			}
		}
	default:
		if s.checkout != nil {
			handled, err := s.checkout.HandleEvent(r.Context(), ev)
			if err == nil && !handled {
				_, err = s.checkout.HandleMoneyEvent(r.Context(), ev.Type, ev.Account, ev.Data.Object)
			}
			if err != nil {
				s.internalError(w, r, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
