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
	return c.doAs(ctx, "", method, path, form, out)
}

// doAs makes a request on behalf of a connected account (Stripe-Account header).
func (c *Client) doAs(ctx context.Context, account, method, path string, form url.Values, out any) error {
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
	if account != "" {
		req.Header.Set("Stripe-Account", account)
	}
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

// Payments creates and reads Checkout Sessions on a seller's own account
// (direct charges), with our application fee.
type Payments interface {
	CreateCheckoutSession(ctx context.Context, account string, p CheckoutParams) (CheckoutSession, error)
	GetCheckoutSession(ctx context.Context, account, id string) (CheckoutSession, error)
	// ExpireCheckoutSession closes an open session so it can no longer be paid.
	ExpireCheckoutSession(ctx context.Context, account, id string) (CheckoutSession, error)
	// Refund returns a payment in full (and our application fee with it).
	Refund(ctx context.Context, account, paymentIntent string, metadata map[string]string) (Refund, error)
}

type Refund struct {
	ID     string `json:"id"`
	Amount int    `json:"amount"`
	Status string `json:"status"`
}

func (c *Client) Refund(ctx context.Context, account, paymentIntent string, metadata map[string]string) (Refund, error) {
	f := url.Values{"payment_intent": {paymentIntent}, "refund_application_fee": {"true"}}
	for k, v := range metadata {
		f.Set("metadata["+k+"]", v)
	}
	var r Refund
	err := c.doAs(ctx, account, http.MethodPost, "/v1/refunds", f, &r)
	return r, err
}

type LineItem struct {
	Name      string
	UnitCents int
	Quantity  int
	ImageURL  string
}

type CheckoutParams struct {
	OrderID       string
	Lines         []LineItem
	FeeCents      int
	CustomerEmail string
	SuccessURL    string // may contain {CHECKOUT_SESSION_ID}
	CancelURL     string
	ExpiresAt     time.Time // 30 minutes to 24 hours from now
	AutomaticTax  bool      // Stripe Tax on the seller's account adds sales tax
}

type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type CheckoutSession struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	Status          string `json:"status"`         // open, complete, expired
	PaymentStatus   string `json:"payment_status"` // paid, unpaid, no_payment_required
	PaymentIntent   string `json:"payment_intent"`
	AmountTotal     int    `json:"amount_total"`
	ClientReference string `json:"client_reference_id"`
	ShippingDetails *struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	} `json:"shipping_details"`
	// Newer API versions (2025-03-31 onward, which webhook endpoints may use)
	// move the address here; Normalize copies it to ShippingDetails.
	CollectedInformation *struct {
		ShippingDetails *struct {
			Name    string  `json:"name"`
			Address Address `json:"address"`
		} `json:"shipping_details"`
	} `json:"collected_information,omitempty"`
}

// Normalize fills ShippingDetails from collected_information when a newer
// API version sent it there.
func (c *CheckoutSession) Normalize() {
	if c.ShippingDetails == nil && c.CollectedInformation != nil && c.CollectedInformation.ShippingDetails != nil {
		sd := c.CollectedInformation.ShippingDetails
		c.ShippingDetails = &struct {
			Name    string  `json:"name"`
			Address Address `json:"address"`
		}{Name: sd.Name, Address: sd.Address}
	}
}

func (c *Client) CreateCheckoutSession(ctx context.Context, account string, p CheckoutParams) (CheckoutSession, error) {
	f := url.Values{
		"mode":                {"payment"},
		"client_reference_id": {p.OrderID},
		"metadata[order_id]":  {p.OrderID},
		"success_url":         {p.SuccessURL},
		"cancel_url":          {p.CancelURL},
		"payment_intent_data[application_fee_amount]":       {strconv.Itoa(p.FeeCents)},
		"payment_intent_data[metadata][order_id]":           {p.OrderID},
		"shipping_address_collection[allowed_countries][0]": {"US"},
	}
	if !p.ExpiresAt.IsZero() {
		f.Set("expires_at", strconv.FormatInt(p.ExpiresAt.Unix(), 10))
	}
	if p.CustomerEmail != "" {
		f.Set("customer_email", p.CustomerEmail)
	}
	if p.AutomaticTax {
		f.Set("automatic_tax[enabled]", "true")
	}
	for i, l := range p.Lines {
		k := fmt.Sprintf("line_items[%d]", i)
		f.Set(k+"[quantity]", strconv.Itoa(l.Quantity))
		f.Set(k+"[price_data][currency]", "usd")
		f.Set(k+"[price_data][unit_amount]", strconv.Itoa(l.UnitCents))
		f.Set(k+"[price_data][product_data][name]", l.Name)
		if strings.HasPrefix(l.ImageURL, "https://") && len(l.ImageURL) < 2000 {
			f.Set(k+"[price_data][product_data][images][0]", l.ImageURL)
		}
	}
	var s CheckoutSession
	err := c.doAs(ctx, account, http.MethodPost, "/v1/checkout/sessions", f, &s)
	return s, err
}

func (c *Client) GetCheckoutSession(ctx context.Context, account, id string) (CheckoutSession, error) {
	var s CheckoutSession
	err := c.doAs(ctx, account, http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(id), nil, &s)
	s.Normalize()
	return s, err
}

func (c *Client) ExpireCheckoutSession(ctx context.Context, account, id string) (CheckoutSession, error) {
	var s CheckoutSession
	err := c.doAs(ctx, account, http.MethodPost, "/v1/checkout/sessions/"+url.PathEscape(id)+"/expire", url.Values{}, &s)
	return s, err
}

// Platform covers charges on the platform's own Stripe account: sellers'
// saved cards for shipping labels. Separate from buyers' payments.
type Platform interface {
	CreateCustomer(ctx context.Context, email, userID string) (string, error)
	// CreateSetupSession opens Stripe Checkout in setup mode to save a card.
	CreateSetupSession(ctx context.Context, customer, successURL, cancelURL string) (CheckoutSession, error)
	// SetupResult returns the saved payment method and its "Visa ending 4242" label.
	SetupResult(ctx context.Context, sessionID string) (paymentMethod, label, customer string, err error)
	// Charge charges a saved card off-session and returns the PaymentIntent id.
	Charge(ctx context.Context, customer, paymentMethod string, cents int, description string, metadata map[string]string) (string, error)
	RefundPlatform(ctx context.Context, paymentIntent string) error
}

func (c *Client) CreateCustomer(ctx context.Context, email, userID string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/customers", url.Values{"email": {email}, "metadata[user_id]": {userID}}, &out)
	return out.ID, err
}

func (c *Client) CreateSetupSession(ctx context.Context, customer, successURL, cancelURL string) (CheckoutSession, error) {
	var s CheckoutSession
	err := c.do(ctx, http.MethodPost, "/v1/checkout/sessions", url.Values{
		"mode": {"setup"}, "customer": {customer}, "currency": {"usd"}, "payment_method_types[0]": {"card"},
		"success_url": {successURL}, "cancel_url": {cancelURL},
	}, &s)
	return s, err
}

func (c *Client) SetupResult(ctx context.Context, sessionID string) (string, string, string, error) {
	var s struct {
		Status      string `json:"status"`
		Customer    string `json:"customer"`
		SetupIntent struct {
			PaymentMethod struct {
				ID   string `json:"id"`
				Card struct {
					Brand string `json:"brand"`
					Last4 string `json:"last4"`
				} `json:"card"`
			} `json:"payment_method"`
		} `json:"setup_intent"`
	}
	path := "/v1/checkout/sessions/" + url.PathEscape(sessionID) + "?expand[]=setup_intent.payment_method"
	if err := c.do(ctx, http.MethodGet, path, nil, &s); err != nil {
		return "", "", "", err
	}
	pm := s.SetupIntent.PaymentMethod
	if s.Status != "complete" || pm.ID == "" {
		return "", "", "", errors.New("the card wasn't saved")
	}
	brand := strings.ToUpper(pm.Card.Brand[:min(1, len(pm.Card.Brand))]) + pm.Card.Brand[min(1, len(pm.Card.Brand)):]
	return pm.ID, brand + " ending " + pm.Card.Last4, s.Customer, nil
}

func (c *Client) Charge(ctx context.Context, customer, pm string, cents int, desc string, md map[string]string) (string, error) {
	f := url.Values{"amount": {strconv.Itoa(cents)}, "currency": {"usd"}, "customer": {customer}, "payment_method": {pm},
		"off_session": {"true"}, "confirm": {"true"}, "description": {desc}}
	for k, v := range md {
		f.Set("metadata["+k+"]", v)
	}
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/payment_intents", f, &out); err != nil {
		return "", err
	}
	if out.Status != "succeeded" {
		return out.ID, fmt.Errorf("card payment %s", out.Status)
	}
	return out.ID, nil
}

func (c *Client) RefundPlatform(ctx context.Context, pi string) error {
	var out struct{}
	return c.do(ctx, http.MethodPost, "/v1/refunds", url.Values{"payment_intent": {pi}}, &out)
}
