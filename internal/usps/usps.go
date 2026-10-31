// Package usps talks to the two USPS services letter tracking needs: the
// Addresses API (ZIP+4 and delivery point for the barcode) and Informed
// Visibility (scan events for one barcode).
package usps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	apisBase   = "https://apis.usps.com"
	ivAuthURL  = "https://services.usps.com/oauth/authenticate"
	ivPieceURL = "https://iv.usps.com/ivws_api/informedvisapi/api/mt/get/piece/imb/"
	// ivClientID is the public client ID USPS issues for Informed Visibility
	// API sign-in with Business Customer Gateway credentials.
	ivClientID = "687b8a36-db61-42f7-83f7-11c79bf7785e"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// token caches an access token until shortly before it expires.
type token struct {
	mu      sync.Mutex
	value   string
	kind    string
	expires time.Time
}

func (t *token) get(fetch func() (value, kind string, ttl time.Duration, err error)) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.value != "" && time.Now().Before(t.expires) {
		return t.kind + " " + t.value, nil
	}
	v, k, ttl, err := fetch()
	if err != nil {
		return "", err
	}
	if k == "" {
		k = "Bearer"
	}
	t.value, t.kind, t.expires = v, k, time.Now().Add(ttl/2)
	return k + " " + v, nil
}

func (t *token) clear() {
	t.mu.Lock()
	t.value = ""
	t.mu.Unlock()
}

func postJSON(ctx context.Context, u string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return do(req, out)
}

func do(req *http.Request, out any) error {
	req.Header.Set("Accept", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode == http.StatusUnauthorized {
		return errUnauthorized
	}
	if res.StatusCode >= 300 {
		return &APIError{Status: res.StatusCode, Body: snippet(data)}
	}
	return json.Unmarshal(data, out)
}

var errUnauthorized = errors.New("usps: unauthorized")

// APIError is a non-2xx answer from USPS.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("usps: HTTP %d: %s", e.Status, e.Body) }

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// Addresses standardizes addresses with the USPS Addresses API v3, using a
// developers.usps.com app's consumer key and secret.
type Addresses struct {
	ClientID, Secret string
	tok              token
}

func (a *Addresses) auth(ctx context.Context) (string, error) {
	return a.tok.get(func() (string, string, time.Duration, error) {
		var r struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			ExpiresIn   any    `json:"expires_in"`
		}
		err := postJSON(ctx, apisBase+"/oauth2/v3/token", map[string]string{
			"client_id": a.ClientID, "client_secret": a.Secret, "grant_type": "client_credentials",
		}, &r)
		if err != nil {
			return "", "", 0, fmt.Errorf("usps addresses token: %w", err)
		}
		if r.AccessToken == "" {
			return "", "", 0, errors.New("usps addresses token: empty")
		}
		return r.AccessToken, r.TokenType, seconds(r.ExpiresIn, time.Hour), nil
	})
}

// seconds reads expires_in, which USPS sends as a number or a string.
func seconds(v any, def time.Duration) time.Duration {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case string:
		fmt.Sscan(x, &n)
	}
	if n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}

// Standard is the USPS form of an address.
type Standard struct {
	Street, Secondary, City, State string
	ZIP5, ZIP4, DeliveryPoint      string
}

// Routing is the barcode routing code: ZIP+4+delivery point when known.
func (s Standard) Routing() string {
	switch {
	case len(s.ZIP5) == 5 && len(s.ZIP4) == 4 && len(s.DeliveryPoint) == 2:
		return s.ZIP5 + s.ZIP4 + s.DeliveryPoint
	case len(s.ZIP5) == 5 && len(s.ZIP4) == 4:
		return s.ZIP5 + s.ZIP4
	}
	return s.ZIP5
}

// Standardize looks an address up. A bad address is an *APIError with 400.
func (a *Addresses) Standardize(ctx context.Context, street, secondary, city, state, zip string) (Standard, error) {
	q := url.Values{}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			q.Set(k, v)
		}
	}
	set("streetAddress", street)
	set("secondaryAddress", secondary)
	set("city", city)
	set("state", state)
	if len(zip) >= 5 {
		set("ZIPCode", zip[:5])
	}
	var r struct {
		Address struct {
			StreetAddress    string `json:"streetAddress"`
			SecondaryAddress string `json:"secondaryAddress"`
			City             string `json:"city"`
			State            string `json:"state"`
			ZIPCode          string `json:"ZIPCode"`
			ZIPPlus4         string `json:"ZIPPlus4"`
		} `json:"address"`
		AdditionalInfo struct {
			DeliveryPoint string `json:"deliveryPoint"`
		} `json:"additionalInfo"`
	}
	for try := 0; try < 2; try++ {
		h, err := a.auth(ctx)
		if err != nil {
			return Standard{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apisBase+"/addresses/v3/address?"+q.Encode(), nil)
		if err != nil {
			return Standard{}, err
		}
		req.Header.Set("Authorization", h)
		err = do(req, &r)
		if errors.Is(err, errUnauthorized) && try == 0 {
			a.tok.clear()
			continue
		}
		if err != nil {
			return Standard{}, err
		}
		break
	}
	ad := r.Address
	return Standard{Street: ad.StreetAddress, Secondary: ad.SecondaryAddress, City: ad.City, State: ad.State,
		ZIP5: ad.ZIPCode, ZIP4: ad.ZIPPlus4, DeliveryPoint: r.AdditionalInfo.DeliveryPoint}, nil
}

// Visibility reads Informed Visibility piece scans, signed in as a Business
// Customer Gateway user with IV-MTR access.
type Visibility struct {
	Username, Password string
	tok                token
}

func (v *Visibility) auth(ctx context.Context) (string, error) {
	return v.tok.get(func() (string, string, time.Duration, error) {
		var r struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			ExpiresIn   any    `json:"expires_in"`
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		err := postJSON(ctx, ivAuthURL, map[string]string{
			"username": v.Username, "password": v.Password, "grant_type": "authorization",
			"response_type": "token", "scope": "user.info.ereg,iv1.apis", "client_id": ivClientID,
		}, &r)
		if err != nil {
			return "", "", 0, fmt.Errorf("informed visibility sign-in: %w", err)
		}
		if r.AccessToken == "" {
			return "", "", 0, fmt.Errorf("informed visibility sign-in: %s %s", r.Error, r.Description)
		}
		return r.AccessToken, r.TokenType, seconds(r.ExpiresIn, 30*time.Minute), nil
	})
}

// Check signs in, to confirm the credentials work.
func (v *Visibility) Check(ctx context.Context) error {
	_, err := v.auth(ctx)
	return err
}

// Scan is one USPS handling event for a letter.
type Scan struct {
	DateTime     string `json:"scan_date_time"`
	EventCode    string `json:"scan_event_code"`
	HandlingType string `json:"handling_event_type"`
	MailPhase    string `json:"mail_phase"`
	Machine      string `json:"machine_name"`
	Facility     string `json:"scan_facility_name"`
	City         string `json:"scan_facility_city"`
	State        string `json:"scan_facility_state"`
	ZIP          string `json:"scan_facility_zip"`
}

// Piece is what Informed Visibility knows about one barcode.
type Piece struct {
	ExpectedDelivery string `json:"expected_delivery_date"`
	Scans            []Scan `json:"scans"`
}

// Piece fetches the scans for a 20-, 25-, 29- or 31-digit IMb.
func (v *Visibility) Piece(ctx context.Context, imb string) (Piece, error) {
	var r struct {
		Data        *Piece `json:"data"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
		Message     string `json:"message"`
	}
	for try := 0; try < 2; try++ {
		h, err := v.auth(ctx)
		if err != nil {
			return Piece{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ivPieceURL+url.PathEscape(imb), nil)
		if err != nil {
			return Piece{}, err
		}
		req.Header.Set("Authorization", h)
		err = do(req, &r)
		if errors.Is(err, errUnauthorized) && try == 0 {
			v.tok.clear()
			continue
		}
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return Piece{}, nil // no scans yet
		}
		if err != nil {
			return Piece{}, err
		}
		break
	}
	if r.Data == nil {
		if r.Error != "" {
			return Piece{}, fmt.Errorf("informed visibility: %s %s", r.Error, r.Description)
		}
		return Piece{}, nil
	}
	return *r.Data, nil
}

// Describe turns a scan into a short line for buyers, e.g.
// "Processed at KANSAS CITY MO" or "Out for delivery area".
func (s Scan) Describe() string {
	place := strings.TrimSpace(strings.TrimSpace(s.City) + " " + strings.TrimSpace(s.State))
	if place == "" {
		place = strings.TrimSpace(s.Facility)
	}
	phase := strings.ToLower(s.MailPhase)
	switch {
	case strings.Contains(phase, "destination") && (strings.Contains(phase, "sequenc") || strings.Contains(phase, "delivery")):
		return joinAt("Sorted for delivery", place)
	case strings.Contains(phase, "destination"):
		return joinAt("Arrived at destination facility", place)
	case strings.Contains(phase, "origin"):
		return joinAt("Processed at origin facility", place)
	}
	return joinAt("Scanned by USPS", place)
}

func joinAt(what, place string) string {
	if place == "" {
		return what
	}
	return what + " in " + place
}

// ParseTime reads a scan time ("2026-11-02 04:31:12" or RFC 3339). USPS
// reports facility local time without a zone; it's treated as US Central,
// which is close enough for a timeline.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		loc = time.UTC
	}
	for _, f := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "01/02/2006 15:04:05"} {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
