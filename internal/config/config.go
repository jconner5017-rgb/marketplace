// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env          string        // "dev" or "prod"
	Addr         string        // HTTP listen address, e.g. ":8080"
	DatabaseURL  string        // postgres://user:pass@host:5432/db?sslmode=disable
	CookieSecure bool          // send session cookie only over HTTPS
	SessionTTL   time.Duration // how long a login lasts
	TrustProxy   bool          // read the client IP from X-Forwarded-For (behind a load balancer)
}

func Load() (Config, error) {
	c := Config{
		Env:         getenv("APP_ENV", "dev"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	// ADDR wins; otherwise use PORT (set by hosts like Render); otherwise 8080.
	switch {
	case os.Getenv("ADDR") != "":
		c.Addr = os.Getenv("ADDR")
	case os.Getenv("PORT") != "":
		c.Addr = ":" + os.Getenv("PORT")
	default:
		c.Addr = ":8080"
	}

	if v := os.Getenv("TRUST_PROXY_HEADERS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, errors.New("TRUST_PROXY_HEADERS must be true or false")
		}
		c.TrustProxy = b
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}

	// Secure cookies by default everywhere except local development.
	c.CookieSecure = c.Env != "dev"
	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, errors.New("COOKIE_SECURE must be true or false")
		}
		c.CookieSecure = b
	}

	hours := 24 * 30
	if v := os.Getenv("SESSION_TTL_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, errors.New("SESSION_TTL_HOURS must be a positive integer")
		}
		hours = n
	}
	c.SessionTTL = time.Duration(hours) * time.Hour
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
