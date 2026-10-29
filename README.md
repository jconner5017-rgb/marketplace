# Card Marketplace

An eBay-style marketplace for sports cards and trading card games, US-only, cards up to $1,000. Sellers are paid directly through their own Stripe accounts; the platform earns a per-sale fee.

Full design: see the **Card Marketplace Build Spec** document.

## Status

**Phase 1 (done):** Go API skeleton, Postgres with migrations, accounts, login sessions, admin role, Docker, CI.

**Phase 2 (done):** Stripe Connect seller onboarding (Standard accounts), seller tiers and price caps, listings, CSV bulk import, FireCardManager-ready upsert by SKU, public search with filters.

**Website (in progress):** Next.js storefront in `web/` — home, search with filters, listing pages, sign in, register, seller hub, list-a-card, CSV upload. Sample listings via `SEED_DEMO=true`.

Next: phase 3 — cart, bundle discounts, checkout with Stripe direct charges, orders, ledger.

## Website (`web/`)

Next.js 15 (App Router) + Tailwind CSS 4. The browser only talks to the website; `/api/*` is proxied to the Go API, so the login cookie is first-party and the API needs no CORS.

| Setting | Where | Value |
| --- | --- | --- |
| `API_URL` | web service | `http://marketplace-api-xqq7:10000` (Render private network) |
| `NEXT_PUBLIC_SITE_NAME` | web service, optional | Defaults to "Slabs & Singles" |
| `PUBLIC_URL` | API service | The website's URL, so Stripe sends sellers back to it |
| `IMAGE_BASE_URL` | API service | The API's own URL, for sample card images |
| `SEED_DEMO` | API service | `true` loads sample listings; `remove` deletes them |

Run locally: start the API on :8080, then `cd web && npm install && API_URL=http://localhost:8080 npm run dev`.

## Run it locally

Requires Docker.

```bash
docker compose up --build
```

The API is at http://localhost:8080. Check it with:

```bash
curl http://localhost:8080/healthz
```

### Without Docker

Requires Go 1.24+ and Postgres 16.

```bash
cp .env.example .env          # then edit DATABASE_URL
export $(grep -v '^#' .env | xargs)
go run ./cmd/api
```

Migrations run automatically on startup.

### Deploy to Render

`render.yaml` sets up the API ($7/mo) and a Postgres database ($6/mo + storage) in the Virginia region.

1. Render dashboard → **New → Blueprint**.
2. Connect the `jconner5017-rgb/marketplace` repo and approve.
3. Render builds the Dockerfile, creates the database, and wires `DATABASE_URL` automatically.
4. When it shows **Deployed**, open `https://<your-service>.onrender.com/healthz`.

Every push to `main` redeploys.

Set these in the service's **Environment** tab (never commit them):

| Variable | Value |
| --- | --- |
| `DATABASE_URL` | The database's Internal Database URL |
| `PUBLIC_URL` | The service URL, e.g. `https://marketplace-api-xqq7.onrender.com` |
| `STRIPE_SECRET_KEY` | `sk_test_…` from Stripe → Developers → API keys |
| `STRIPE_WEBHOOK_SECRET` | `whsec_…` from the Stripe webhook endpoint (below) |
| `CHECKOUT_ENABLED` | `true` lets buyers pay (needs `STRIPE_SECRET_KEY`); off by default |
| `SHIPPING_PROVIDER` | `shippo` turns on label buying (needs `STRIPE_SECRET_KEY` for sellers' saved cards) |
| `SHIPPO_API_KEY` | `shippo_test_…` or `shippo_live_…` |
| `SHIPPING_WEBHOOK_TOKEN` | a long random string; in Shippo, add a `track_updated` webhook to `<PUBLIC_URL>/v1/webhooks/shipping/shippo?token=<this>` |
| `ADMIN_IP_ALLOWLIST` | `off` ignores the admin network allowlist (break-glass if staff are locked out) |
| `WEB_URL` | the public website Stripe sends buyers back to (default `https://slabsnsingles.com`) |

**Stripe webhook:** in Stripe → Developers → Webhooks, add an endpoint at `<PUBLIC_URL>/v1/webhooks/stripe`, choose **Events on Connected accounts**, and select `account.updated`, `account.application.deauthorized`, `checkout.session.completed`, `checkout.session.async_payment_succeeded`, `checkout.session.expired`, `charge.refunded`, `charge.dispute.created` and `charge.dispute.closed`. Optionally add a second endpoint at the same URL for **Events on your account** with `application_fee.refunded` (fee refunds in the ledger); put both signing secrets in `STRIPE_WEBHOOK_SECRET`, comma-separated. Copy its signing secret into `STRIPE_WEBHOOK_SECRET`.

To make yourself an admin on Render, sign up through the API, then open the database in Render → **Connect** → copy the **PSQL Command**, run it on your computer, and enter:

```sql
UPDATE users SET role = 'admin' WHERE lower(email) = lower('you@example.com');
```

(The API image has no shell, by design, so Render's Shell tab can't run commands in it.)

### Make yourself an admin

Sign up through the API first, then:

```bash
go run ./cmd/admin promote you@example.com
# or inside Docker:
docker compose exec api /app/admin promote you@example.com
```

## API

All write requests must send `Content-Type: application/json` (the CSV import uses `text/csv`). Login uses an HttpOnly session cookie (`mp_session`).

**Accounts**

| Method | Path | Who | What |
| --- | --- | --- | --- |
| GET | `/healthz` | anyone | Server and database check |
| POST | `/v1/auth/signup` | anyone | `{email, username, password}` → creates account and logs in |
| POST | `/v1/auth/login` | anyone | `{email, password}` → logs in |
| POST | `/v1/auth/logout` | anyone | Ends the current session |
| GET | `/v1/me` | logged in | Current user |
| GET | `/v1/admin/users` | admin | Newest 50 accounts |

**Selling**

| Method | Path | What |
| --- | --- | --- |
| POST | `/v1/seller/onboarding` | Creates the seller's Stripe Standard account (first time) and returns `{url}` to Stripe's onboarding |
| GET | `/v1/seller` | Seller status: `charges_enabled`, `tier`, `tier_cap_cents`, bundle discount |
| PATCH | `/v1/seller/settings` | `{bundle_discount_type: "per_extra_item"\|"percent_3plus", bundle_discount_val}`; send `{}` to clear |
| POST | `/v1/seller/listings` | Create a listing |
| GET | `/v1/seller/listings` | The seller's own listings (`limit`, `offset`) |
| GET / PATCH / DELETE | `/v1/seller/listings/{id}` | Read, change or remove one listing |
| PUT | `/v1/seller/listings/external/{sku}` | Create or replace by the seller's own SKU (used by FireCardManager) |
| POST | `/v1/seller/listings/import` | Bulk CSV import (`Content-Type: text/csv`, up to 5,000 rows) |
| GET | `/v1/seller/listings/import/template` | Download a sample CSV |
| POST | `/v1/webhooks/stripe` | Stripe Connect events (accounts and Checkout sessions) |
| GET | `/v1/checkout/status` | whether buyers can pay yet |
| POST | `/v1/checkout` | start Stripe Checkout for one seller's cart cards (`seller`) or held cards (`hold_ids`) |
| POST | `/v1/orders/{id}/confirm` · `/cancel` | buyer returned from Stripe: record payment, or release the cards |
| GET | `/v1/orders?side=buying\|selling&status=` · `/v1/orders/{id}` | orders for either side |
| POST | `/v1/orders/{id}/ship` · `/delivered` · `/feedback` · `/problem` | seller ships; buyer confirms, rates, or opens a case |

Sellers start at tier 1 (cards up to $20), then $250 and $1,000. Listings need at least one `https://` photo; raw cards need a `condition` (NM, LP, MP, HP, DMG); graded cards need `grader` and `grade`.

**Buying**

| Method | Path | What |
| --- | --- | --- |
| GET | `/v1/listings` | Search. Filters: `q`, `category` (sports/tcg), `game`, `set`, `year`, `grader` (PSA… or `raw`), `min_grade`, `condition`, `attribute`, `seller`, `min_price`, `max_price` (dollars), `sort` (newest, price_asc, price_desc, relevance), `limit`, `offset` |
| GET | `/v1/listings/{id}` | One listing |

Errors always look like:

```json
{"error": {"code": "invalid_input", "message": "check the highlighted fields", "fields": {"email": "enter a valid email address"}}}
```

Example session with curl:

```bash
curl -c jar -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","username":"card_guy","password":"pikachu-1999"}' \
  http://localhost:8080/v1/auth/signup
curl -b jar http://localhost:8080/v1/me
```

## Tests

Integration tests need a throwaway Postgres database (they wipe it):

```bash
TEST_DATABASE_URL=postgres://marketplace:marketplace@localhost:5432/marketplace_test?sslmode=disable \
  go test -count=1 -p 1 ./...
```

GitHub Actions runs the same tests on every push.

## Layout

```
cmd/api          HTTP server entry point
cmd/admin        one-off admin tasks (promote/demote)
internal/config  environment settings
internal/db      Postgres connection + embedded SQL migrations
internal/auth    accounts, password hashing, sessions
internal/httpapi routes, handlers, middleware
```

## Security notes

- Passwords: PBKDF2-SHA256, 600,000 iterations, per-user salt (Go standard library).
- Sessions: random 256-bit tokens; only a SHA-256 hash is stored in the database.
- Cookies: HttpOnly, SameSite=Lax, Secure outside dev.
- CSRF: writes require `application/json`, which browsers can't send cross-site without a CORS preflight.
- Login and signup are rate limited per IP (10 per 5 minutes, in memory).
- Banned or suspended users lose their sessions immediately.
