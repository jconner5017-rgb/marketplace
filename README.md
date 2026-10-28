# Card Marketplace

An eBay-style marketplace for sports cards and trading card games, US-only, cards up to $1,000. Sellers are paid directly through their own Stripe accounts; the platform earns a per-sale fee.

Full design: see the **Card Marketplace Build Spec** document.

## Status

**Phase 1 (done):** Go API skeleton, Postgres with migrations, accounts, login sessions, admin role, Docker, CI.

Next: phase 2 — seller onboarding (Stripe Connect, test mode), listings, CSV import, search.

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

## API (phase 1)

All write requests must send `Content-Type: application/json`. Login uses an HttpOnly session cookie (`mp_session`).

| Method | Path | Who | What |
| --- | --- | --- | --- |
| GET | `/healthz` | anyone | Server and database check |
| POST | `/v1/auth/signup` | anyone | `{email, username, password}` → creates account and logs in |
| POST | `/v1/auth/login` | anyone | `{email, password}` → logs in |
| POST | `/v1/auth/logout` | anyone | Ends the current session |
| GET | `/v1/me` | logged in | Current user |
| GET | `/v1/admin/users` | admin | Newest 50 accounts |

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
