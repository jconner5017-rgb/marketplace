package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPick(t *testing.T) {
	if s, err := Pick("", "", ""); s != nil || err != nil {
		t.Fatal("blank provider should be off")
	}
	if _, err := Pick("mailjet", "k", ""); err == nil {
		t.Fatal("mailjet without secret accepted")
	}
	if s, err := Pick("Mailjet", "k", "s"); err != nil || s.Name() != "mailjet" {
		t.Fatal(s, err)
	}
}

func TestMailjetRequest(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"Messages":[{"Status":"success"}]}`))
	}))
	defer srv.Close()
	old := httpClient.Transport
	httpClient.Transport = rewrite{srv.URL}
	defer func() { httpClient.Transport = old }()
	err := Mailjet{Key: "k", Secret: "s"}.Send(context.Background(), "Slabs & Singles <hello@slabsnsingles.com>",
		Message{To: "a@b.test", Subject: "Hi", Text: "Body", Headers: map[string]string{"List-Unsubscribe": "<x>"}})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Basic azpz" {
		t.Fatalf("auth %q", auth)
	}
	m := got["Messages"].([]any)[0].(map[string]any)
	if m["From"].(map[string]any)["Email"] != "hello@slabsnsingles.com" || m["From"].(map[string]any)["Name"] != "Slabs & Singles" ||
		m["TextPart"] != "Body" || m["Headers"].(map[string]any)["List-Unsubscribe"] != "<x>" {
		t.Fatalf("body %v", m)
	}
}

// rewrite sends every request to the test server.
type rewrite struct{ base string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u := r.base + req.URL.Path
	nr, _ := http.NewRequestWithContext(req.Context(), req.Method, u, req.Body)
	nr.Header = req.Header
	if !strings.HasPrefix(u, "http") {
		return nil, http.ErrNotSupported
	}
	return http.DefaultTransport.RoundTrip(nr)
}
