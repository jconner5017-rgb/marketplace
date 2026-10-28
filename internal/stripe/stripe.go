// Package stripe is a small client for the handful of Stripe Connect calls
// the marketplace needs. It talks to the REST API directly (form-encoded
// requests, JSON responses), so there is no SDK dependency.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Connect is what the rest of the app uses. Tests swap in a fake.
type Connect interface {
	// CreateStandardAccount creates a Standard connected account.
	CreateStandardAccount(ctx context.Context, email string) (Account, error)
	// CreateOnboardingLink returns a one-time Stripe-hosted onboarding URL.
	CreateOnboardingLink(ctx context.Context, accountID, refreshURL, returnURL string) (string, error)
	// GetAccount fetches the account's current state.
	GetAccount(ctx context.Context, accountID string) (Account, error)
}

type Account struct {
	ID               string `json:"id"`
	ChargesEnabled   bool   `json:"charges_enabled"`
	DetailsSubmitted bool   `json:"details_submitted"`
	PayoutsEnabled   bool   `json:"payouts_enabled"`
}

// Error is an error returned by the Stripe API.
type Error struct {
	Status  int
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("stripe: %d %s: %s", e.Status, e.Type, e.Message)
}

// Client implements Connect against the real API.
type Client struct {
	SecretKey string
	BaseURL   string // defaults to https://api.stripe.com
	HTTP      *http.Client
}

func NewClient(secretKey string) *Client {
	return &Client{
		SecretKey: secretKey,
		BaseURL:   "https://api.stripe.com",
		HTTP:      &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.SecretKey, "")
	req.Header.Set("Stripe-Version", "2024-06-20")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("stripe request: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		var wrapper struct {
			Error Error `json:"error"`
		}
		json.Unmarshal(data, &wrapper)
		wrapper.Error.Status = res.StatusCode
		if wrapper.Error.Message == "" {
			wrapper.Error.Message = http.StatusText(res.StatusCode)
		}
		return &wrapper.Error
	}
	return json.Unmarshal(data, out)
}

func (c *Client) CreateStandardAccount(ctx context.Context, email string) (Account, error) {
	var a Account
	err := c.do(ctx, http.MethodPost, "/v1/accounts", url.Values{
		"type":    {"standard"},
		"country": {"US"},
		"email":   {email},
	}, &a)
	return a, err
}

func (c *Client) CreateOnboardingLink(ctx context.Context, accountID, refreshURL, returnURL string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/account_links", url.Values{
		"account":     {accountID},
		"refresh_url": {refreshURL},
		"return_url":  {returnURL},
		"type":        {"account_onboarding"},
	}, &out)
	return out.URL, err
}

func (c *Client) GetAccount(ctx context.Context, accountID string) (Account, error) {
	var a Account
	err := c.do(ctx, http.MethodGet, "/v1/accounts/"+url.PathEscape(accountID), nil, &a)
	return a, err
}

// Event is the part of a webhook event the app reads.
type Event struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Account string `json:"account"` // set for Connect events
	Data    struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

var ErrBadSignature = errors.New("stripe: invalid webhook signature")

// VerifyWebhook checks the Stripe-Signature header and parses the event.
// tolerance limits how old an event may be, to stop replays.
func VerifyWebhook(payload []byte, header, secret string, tolerance time.Duration, now time.Time) (Event, error) {
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return Event{}, ErrBadSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return Event{}, ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(payload)
	want := mac.Sum(nil)

	valid := false
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			valid = true
			break
		}
	}
	if !valid {
		return Event{}, ErrBadSignature
	}
	var ev Event
	if err := json.Unmarshal(payload, &ev); err != nil {
		return Event{}, err
	}
	return ev, nil
}

// SignForTest builds a valid Stripe-Signature header. Used only in tests.
func SignForTest(payload []byte, secret string, t time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", t.Unix())
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", t.Unix(), hex.EncodeToString(mac.Sum(nil)))
}
