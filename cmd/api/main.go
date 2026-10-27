// Command api runs the marketplace HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/config"
	"github.com/jconner5017-rgb/marketplace/internal/db"
	"github.com/jconner5017-rgb/marketplace/internal/httpapi"
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

	authSvc := &auth.Service{DB: database, SessionTTL: cfg.SessionTTL}
	api := httpapi.New(httpapi.Options{
		DB:           database,
		Auth:         authSvc,
		Log:          log,
		CookieSecure: cfg.CookieSecure,
		SessionTTL:   cfg.SessionTTL,
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
