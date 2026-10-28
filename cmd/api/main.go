// Command api runs the marketplace HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/config"
	"github.com/jconner5017-rgb/marketplace/internal/db"
	"github.com/jconner5017-rgb/marketplace/internal/demo"
	"github.com/jconner5017-rgb/marketplace/internal/httpapi"
	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
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
	})

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
