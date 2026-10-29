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
			"query", clip(r.URL.RawQuery, 200),
			"fwd", clip(r.Header.Get("X-Forwarded-For"), 60),
			"ua", clip(r.Header.Get("User-Agent"), 120),
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
			// text/csv (bulk import) also forces a preflight; form posts don't.
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" && mt != "text/csv" {
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
		// Proxies append to X-Forwarded-For, so anything a visitor sends
		// sits at the left. Read from the right, skipping our own hops
		// (Render's private network and its Cloudflare edge): the first
		// other address is the one that connected to the edge.
		hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(hops[i]))
			if ip == nil || trustedHop(ip) {
				continue
			}
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Cloudflare's published edge ranges (cloudflare.com/ips), which front Render.
var edgeNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{
		"103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "104.16.0.0/13", "104.24.0.0/14",
		"108.162.192.0/18", "131.0.72.0/22", "141.101.64.0/18", "162.158.0.0/15", "172.64.0.0/13",
		"173.245.48.0/20", "188.114.96.0/20", "190.93.240.0/20", "197.234.240.0/22", "198.41.128.0/17",
		"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32",
		"2a06:98c0::/29", "2c0f:f248::/32",
	} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

func trustedHop(ip net.IP) bool {
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	for _, n := range edgeNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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
