// Command api runs the marketplace HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/admin"
	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/checkout"
	"github.com/jconner5017-rgb/marketplace/internal/config"
	"github.com/jconner5017-rgb/marketplace/internal/db"
	"github.com/jconner5017-rgb/marketplace/internal/demo"
	"github.com/jconner5017-rgb/marketplace/internal/fcm"
	"github.com/jconner5017-rgb/marketplace/internal/feedback"
	"github.com/jconner5017-rgb/marketplace/internal/fees"
	"github.com/jconner5017-rgb/marketplace/internal/httpapi"
	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/photos"
	"github.com/jconner5017-rgb/marketplace/internal/sales"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
	"github.com/jconner5017-rgb/marketplace/internal/shopping"
	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		return err
	}
	log.Info("migrations applied")

	authSvc := &auth.Service{DB: database, SessionTTL: cfg.SessionTTL, AdminEmails: cfg.AdminEmails}
	if err := authSvc.PromoteAdmins(ctx); err != nil {
		log.Error("promote admins", "err", err)
	}

	sellerSvc := &sellers.Service{DB: database}
	if cfg.StripeSecretKey != "" {
		sellerSvc.Stripe = stripe.NewClient(cfg.StripeSecretKey)
		mode := "test"
		if strings.HasPrefix(cfg.StripeSecretKey, "sk_live_") || strings.HasPrefix(cfg.StripeSecretKey, "rk_live_") {
			mode = "live"
		}
		log.Info("stripe enabled", "mode", mode, "webhooks", cfg.StripeWebhookSecret != "")
	} else {
		log.Warn("STRIPE_SECRET_KEY not set; seller onboarding disabled")
	}
	listingSvc := &listings.Service{DB: database, Sellers: sellerSvc}

	switch cfg.SeedDemo {
	case "true", "1", "yes":
		n, err := demo.Seed(ctx, database, listingSvc, cfg.ImageBaseURL)
		if err != nil {
			log.Error("demo seed failed", "err", err)
		} else {
			log.Info("demo listings loaded", "count", n)
		}
	case "remove":
		if err := demo.Remove(ctx, database); err != nil {
			log.Error("demo remove failed", "err", err)
		} else {
			log.Info("demo listings removed")
		}
	}
	salesSvc := &sales.Service{DB: database}
	catalog := &listings.Catalog{DB: database}
	if err := catalog.Load(ctx); err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	mfaKey, err := admin.ParseKey(cfg.MFAKey)
	if err != nil {
		return err
	}
	if mfaKey == nil {
		log.Warn("MFA_ENCRYPTION_KEY not set: the staff back office can't be used")
	}
	feeSvc := &fees.Service{DB: database}
	reputation := &sellers.Reputation{DB: database}
	checkoutSvc := &checkout.Service{DB: database, Sellers: sellerSvc, Fees: feeSvc, Rep: reputation,
		Enabled: cfg.CheckoutEnabled, WebURL: cfg.WebURL}
	if cfg.StripeSecretKey != "" {
		sc := stripe.NewClient(cfg.StripeSecretKey)
		checkoutSvc.Pay, checkoutSvc.Platform = sc, sc
	}
	ship, err := shipping.Pick(cfg.ShippingProvider, map[string]string{"SHIPPO_API_KEY": cfg.ShippoAPIKey})
	if err != nil {
		return err
	}
	if ship != nil {
		checkoutSvc.Ship = ship
		log.Info("shipping labels", "provider", ship.Name(), "card_payments", checkoutSvc.Platform != nil,
			"tracking_webhook", cfg.ShippingWebhookToken != "")
	}
	log.Info("checkout", "enabled", checkoutSvc.Ready(), "return_to", cfg.WebURL)
	fcmSvc := &fcm.Service{DB: database}
	feedbackSvc := &feedback.Service{DB: database, Rep: reputation}
	photoSvc := &photos.Service{DB: database, BaseURL: cfg.ImageBaseURL}
	if cfg.PhotoStore == "s3" {
		photoSvc.S3 = &photos.S3{Endpoint: cfg.S3Endpoint, Bucket: cfg.S3Bucket, Region: cfg.S3Region,
			AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PublicURL: cfg.PhotoPublicURL}
	}
	log.Info("photo storage", "store", photoSvc.Store())
	api := httpapi.New(httpapi.Options{
		DB:           database,
		Auth:         authSvc,
		Log:          log,
		CookieSecure: cfg.CookieSecure,
		SessionTTL:   cfg.SessionTTL,
		TrustProxy:   cfg.TrustProxy,

		Sellers:       sellerSvc,
		Listings:      listingSvc,
		PublicURL:     cfg.PublicURL,
		WebhookSecret: cfg.StripeWebhookSecret,

		Shopping: &shopping.Service{DB: database, Listings: listingSvc, Sellers: sellerSvc},
		Inbox:    &inbox.Service{DB: database, Listings: listingSvc},
		Sales:    salesSvc,
		MFA:      &admin.MFA{DB: database, Key: mfaKey},
		Audit:    &admin.Audit{DB: database},
		Catalog:  catalog,
		Fees:     feeSvc,
		Checkout: checkoutSvc,
		FCM:      fcmSvc,
		Feedback: feedbackSvc,
		Photos:   photoSvc,

		ShippingWebhookToken: cfg.ShippingWebhookToken,

		AdminIPAllowlistOff: cfg.AdminIPAllowlistOff,
	})

	// Fraud checks every 10 minutes.
	fraud := &admin.Fraud{DB: database}
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := fraud.Run(ctx); err != nil {
					log.Error("fraud checks", "err", err)
				} else if n > 0 {
					log.Info("fraud alerts raised", "count", n)
				}
			}
		}
	}()

	// Close finished auctions, warn watchers and expire offers every 30 seconds.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := salesSvc.RunJobs(ctx); err != nil {
					log.Error("sales jobs", "err", err)
				}
				if err := checkoutSvc.RunJobs(ctx); err != nil {
					log.Error("checkout jobs", "err", err)
				}
				if err := feedbackSvc.RunJobs(ctx); err != nil {
					log.Error("feedback jobs", "err", err)
				}
				if _, err := fcmSvc.RunDeliveries(ctx); err != nil {
					log.Error("webhook deliveries", "err", err)
				}
				if err := catalog.Load(ctx); err != nil {
					log.Error("reload catalog", "err", err)
				}
			}
		}
	}()

	// Seller tiers: each seller is re-checked about once a day.
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := fcmSvc.RunReconcile(ctx, checkoutSvc.ReleaseListing, func(m string, kv ...any) { log.Info(m, kv...) }); err != nil {
					log.Error("inventory reconcile", "err", err)
				}
				if n, err := photoSvc.Sweep(ctx); err != nil {
					log.Error("sweep unused photos", "err", err)
				} else if n > 0 {
					log.Info("unused photos removed", "count", n)
				}
				if err := fcmSvc.PurgeIdempotency(ctx); err != nil {
					log.Error("purge idempotency keys", "err", err)
				}
				if n, err := reputation.RunNightly(ctx); err != nil {
					log.Error("reputation job", "err", err)
				} else if n > 0 {
					log.Info("seller tiers changed", "count", n)
				}
			}
		}
	}()

	// Clear out expired sessions once an hour.
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := authSvc.PurgeExpiredSessions(ctx); err != nil {
					log.Error("purge sessions", "err", err)
				} else if n > 0 {
					log.Info("purged expired sessions", "count", n)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}
