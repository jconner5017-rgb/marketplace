// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env          string        // "dev" or "prod"
	Addr         string        // HTTP listen address, e.g. ":8080"
	DatabaseURL  string        // postgres://user:pass@host:5432/db?sslmode=disable
	CookieSecure bool          // send session cookie only over HTTPS
	SessionTTL   time.Duration // how long a login lasts
	TrustProxy   bool          // read the client IP from X-Forwarded-For (behind a load balancer)

	PublicURL           string // e.g. https://marketplace-api-xqq7.onrender.com (Stripe return links)
	StripeSecretKey     string // sk_test_... or sk_live_...; selling is disabled without it
	StripeWebhookSecret string // whsec_...; the webhook endpoint is disabled without it

	// SeedDemo loads sample listings on start; "remove" deletes them.
	SeedDemo string // "", "true" or "remove"
	// ImageBaseURL is where demo card images are served (the API's own URL).
	ImageBaseURL string
	// AdminEmails are promoted to admin on start and when they register.
	AdminEmails []string
	// CheckoutEnabled lets buyers pay (CHECKOUT_ENABLED=true); needs Stripe.
	CheckoutEnabled bool
	// AdminIPAllowlistOff (ADMIN_IP_ALLOWLIST=off) ignores the admin IP allowlist.
	AdminIPAllowlistOff bool
	// Shipping labels: SHIPPING_PROVIDER (shippo), its key, and the token
	// that must appear in the tracking webhook URL.
	ShippingProvider     string
	ShippoAPIKey         string
	ShippingWebhookToken string
	// WebURL is the public website, where Stripe sends buyers back.
	WebURL string
	// MFAKey encrypts staff authenticator secrets (MFA_ENCRYPTION_KEY, 32 bytes).
	MFAKey string
	// Photo storage: PHOTO_STORE=db (default) keeps uploads in Postgres;
	// PHOTO_STORE=s3 uses S3-compatible storage such as Cloudflare R2.
	PhotoStore     string
	S3Endpoint     string // S3_ENDPOINT, e.g. https://<account>.r2.cloudflarestorage.com
	S3Bucket       string // S3_BUCKET
	S3Region       string // S3_REGION ("auto" for R2)
	S3AccessKey    string // S3_ACCESS_KEY_ID
	S3SecretKey    string // S3_SECRET_ACCESS_KEY
	PhotoPublicURL string // PHOTO_PUBLIC_URL: the bucket's public address
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

	c.PublicURL = strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	c.StripeSecretKey = strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	c.StripeWebhookSecret = strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET"))
	c.SeedDemo = strings.ToLower(strings.TrimSpace(os.Getenv("SEED_DEMO")))
	c.MFAKey = strings.TrimSpace(os.Getenv("MFA_ENCRYPTION_KEY"))
	if v := os.Getenv("CHECKOUT_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, errors.New("CHECKOUT_ENABLED must be true or false")
		}
		c.CheckoutEnabled = b
	}
	c.AdminIPAllowlistOff = strings.EqualFold(strings.TrimSpace(os.Getenv("ADMIN_IP_ALLOWLIST")), "off")
	c.ShippingProvider = strings.ToLower(strings.TrimSpace(os.Getenv("SHIPPING_PROVIDER")))
	c.ShippoAPIKey = strings.TrimSpace(os.Getenv("SHIPPO_API_KEY"))
	c.ShippingWebhookToken = strings.TrimSpace(os.Getenv("SHIPPING_WEBHOOK_TOKEN"))
	if c.Env == "prod" && c.ShippingProvider == "fake" {
		return c, errors.New("SHIPPING_PROVIDER=fake is for development only")
	}
	c.WebURL = strings.TrimRight(getenv("WEB_URL", "https://slabsnsingles.com"), "/")
	for _, e := range strings.Split(os.Getenv("ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			c.AdminEmails = append(c.AdminEmails, e)
		}
	}
	c.ImageBaseURL = strings.TrimRight(getenv("IMAGE_BASE_URL", c.PublicURL), "/")
	c.PhotoStore = strings.ToLower(strings.TrimSpace(getenv("PHOTO_STORE", "db")))
	c.S3Endpoint = strings.TrimRight(strings.TrimSpace(os.Getenv("S3_ENDPOINT")), "/")
	c.S3Bucket = strings.TrimSpace(os.Getenv("S3_BUCKET"))
	c.S3Region = strings.TrimSpace(getenv("S3_REGION", "auto"))
	c.S3AccessKey = strings.TrimSpace(os.Getenv("S3_ACCESS_KEY_ID"))
	c.S3SecretKey = strings.TrimSpace(os.Getenv("S3_SECRET_ACCESS_KEY"))
	c.PhotoPublicURL = strings.TrimRight(strings.TrimSpace(os.Getenv("PHOTO_PUBLIC_URL")), "/")
	switch c.PhotoStore {
	case "db":
	case "s3":
		if c.S3Endpoint == "" || c.S3Bucket == "" || c.S3AccessKey == "" || c.S3SecretKey == "" || c.PhotoPublicURL == "" {
			return c, errors.New("PHOTO_STORE=s3 needs S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY and PHOTO_PUBLIC_URL")
		}
		if !strings.HasPrefix(c.S3Endpoint, "https://") || !strings.HasPrefix(c.PhotoPublicURL, "https://") {
			return c, errors.New("S3_ENDPOINT and PHOTO_PUBLIC_URL must be https:// addresses")
		}
	default:
		return c, errors.New("PHOTO_STORE must be db or s3")
	}
	if k := c.StripeSecretKey; k != "" && !strings.HasPrefix(k, "sk_") && !strings.HasPrefix(k, "rk_") {
		return c, errors.New("STRIPE_SECRET_KEY should start with sk_ or rk_")
	}
	if c.Env != "prod" && strings.HasPrefix(c.StripeSecretKey, "sk_live_") {
		return c, errors.New("refusing to use a live Stripe key outside APP_ENV=prod")
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
