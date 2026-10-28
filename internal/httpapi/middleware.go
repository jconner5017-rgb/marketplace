package httpapi

import (
	"context"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/auth"
)

type ctxKey int

const userKey ctxKey = 1

// currentUser returns the logged-in user, if any.
func currentUser(r *http.Request) (auth.User, bool) {
	u, ok := r.Context().Value(userKey).(auth.User)
	return u, ok
}

// recoverer turns a panic into a 500 instead of crashing the server.
func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic", "err", rec, "path", r.URL.Path, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "something went wrong", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// requestLogger logs one line per request.
func requestLogger(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}

// requireJSONForWrites rejects state-changing requests that aren't
// application/json. Browsers can't send a cross-site JSON request without
// a CORS preflight, so together with SameSite cookies this blocks CSRF.
func requireJSONForWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "json_required",
					"Content-Type must be application/json", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// loadUser attaches the logged-in user (if the session cookie is valid).
func (s *Server) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			if u, err := s.auth.UserForSession(r.Context(), c.Value); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), userKey, u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireUser allows only logged-in users.
func requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(r); !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "log in first", nil)
			return
		}
		next(w, r)
	}
}

// requireAdmin allows only admins.
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return requireUser(func(w http.ResponseWriter, r *http.Request) {
		if u, _ := currentUser(r); !u.IsAdmin() {
			writeError(w, http.StatusForbidden, "forbidden", "admins only", nil)
			return
		}
		next(w, r)
	})
}

// clientIP returns the caller's IP. Behind a load balancer (Render, etc.)
// every request arrives from the proxy, so with trustProxy set the first
// address in X-Forwarded-For is used instead. That header can be forged by
// a client, so it is used only for rate limiting and logging, never for
// access decisions.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter allows `limit` events per key per window (fixed window).
// In-memory, so it is per-instance; fine for a single test server.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*windowCount
}

type windowCount struct {
	start time.Time
	n     int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, hits: map[string]*windowCount{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	wc, ok := l.hits[key]
	if !ok || now.Sub(wc.start) >= l.window {
		if len(l.hits) > 100_000 { // crude cap on memory
			l.hits = map[string]*windowCount{}
		}
		l.hits[key] = &windowCount{start: now, n: 1}
		return true
	}
	if wc.n >= l.limit {
		return false
	}
	wc.n++
	return true
}
