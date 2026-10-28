package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		remote     string
		xff        string
		trustProxy bool
		want       string
	}{
		{"direct", "203.0.113.5:4321", "", false, "203.0.113.5"},
		{"header ignored when not trusted", "203.0.113.5:4321", "198.51.100.7", false, "203.0.113.5"},
		{"behind proxy", "10.0.0.2:5000", "198.51.100.7", true, "198.51.100.7"},
		{"chain uses first hop", "10.0.0.2:5000", "198.51.100.7, 10.1.1.1", true, "198.51.100.7"},
		{"garbage header falls back", "10.0.0.2:5000", "not-an-ip", true, "10.0.0.2"},
		{"no header falls back", "10.0.0.2:5000", "", true, "10.0.0.2"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientIP(r, c.trustProxy); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
