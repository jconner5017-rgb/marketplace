package stripe

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
)

// Accounts v2. New Stripe platforms can't create v1 connected accounts in
// live mode ("Accounts v1 support" is test-mode only), so account creation,
// onboarding links and status fall back to the v2 API when v1 is refused.
// Everything else (Checkout with the Stripe-Account header, application
// fees, refunds) works the same for v2 accounts.

// v2Versions are tried in order: the first that Stripe accepts is used.
var v2Versions = []string{"2026-10-28.endive", "2026-10-28.preview"}

func (c *Client) doV2(ctx context.Context, method, path string, body, out any) error {
	var last error
	for _, ver := range v2Versions {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.SecretKey)
		req.Header.Set("Stripe-Version", ver)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("stripe request: %w", err)
		}
		data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if res.StatusCode < 300 {
			return json.Unmarshal(data, out)
		}
		var wrapper struct {
			Error Error `json:"error"`
		}
		json.Unmarshal(data, &wrapper)
		wrapper.Error.Status = res.StatusCode
		if wrapper.Error.Message == "" {
			wrapper.Error.Message = strings.TrimSpace(string(data))
		}
		last = &wrapper.Error
		// Only a version problem is worth retrying with the next version.
		if !strings.Contains(strings.ToLower(wrapper.Error.Message), "version") {
			return last
		}
	}
	return last
}

// v1Refused reports Stripe declining a v1 Accounts call for this platform.
func v1Refused(err error) bool {
	var se *Error
	if !errors.As(err, &se) || se.Status < 400 || se.Status >= 500 {
		return false
	}
	m := strings.ToLower(se.Message + " " + se.Code)
	return strings.Contains(m, "v1") || strings.Contains(m, "v2") || strings.Contains(m, "accounts_v")
}

type v2Account struct {
	ID            string `json:"id"`
	Configuration struct {
		Merchant *struct {
			Capabilities struct {
				CardPayments *struct {
					Status string `json:"status"`
				} `json:"card_payments"`
			} `json:"capabilities"`
		} `json:"merchant"`
	} `json:"configuration"`
	Requirements *struct {
		Entries []struct {
			Description     string `json:"description"`
			MinimumDeadline struct {
				Status string `json:"status"`
			} `json:"minimum_deadline"`
		} `json:"entries"`
	} `json:"requirements"`
}

func (a v2Account) toAccount() Account {
	out := Account{ID: a.ID}
	if m := a.Configuration.Merchant; m != nil && m.Capabilities.CardPayments != nil {
		out.ChargesEnabled = m.Capabilities.CardPayments.Status == "active"
	}
	due := 0
	if a.Requirements != nil {
		for _, e := range a.Requirements.Entries {
			switch e.MinimumDeadline.Status {
			case "past_due":
				out.Requirements.PastDue = append(out.Requirements.PastDue, e.Description)
				due++
			case "currently_due":
				out.Requirements.CurrentlyDue = append(out.Requirements.CurrentlyDue, e.Description)
				due++
			}
		}
	}
	out.DetailsSubmitted = out.ChargesEnabled || (a.Requirements != nil && due == 0)
	return out
}

// createAccountV2 makes a connected account like a v1 Standard account:
// full Stripe Dashboard, Stripe-hosted onboarding, Stripe collects its fees
// from the seller and carries negative-balance losses.
func (c *Client) createAccountV2(ctx context.Context, email string) (Account, error) {
	var a v2Account
	err := c.doV2(ctx, http.MethodPost, "/v2/core/accounts", map[string]any{
		"contact_email": email,
		"identity":      map[string]any{"country": "us"},
		"dashboard":     "full",
		"defaults": map[string]any{
			"responsibilities": map[string]string{"fees_collector": "stripe", "losses_collector": "stripe"},
		},
		"configuration": map[string]any{
			"merchant": map[string]any{
				"capabilities": map[string]any{"card_payments": map[string]bool{"requested": true}},
			},
		},
		"include": []string{"configuration.merchant", "requirements"},
	}, &a)
	return a.toAccount(), err
}

func (c *Client) onboardingLinkV2(ctx context.Context, accountID, refreshURL, returnURL string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	err := c.doV2(ctx, http.MethodPost, "/v2/core/account_links", map[string]any{
		"account": accountID,
		"use_case": map[string]any{
			"type": "account_onboarding",
			"account_onboarding": map[string]any{
				"configurations":     []string{"merchant"},
				"collection_options": map[string]string{"fields": "eventually_due"},
				"refresh_url":        refreshURL,
				"return_url":         returnURL,
			},
		},
	}, &out)
	return out.URL, err
}

func (c *Client) getAccountV2(ctx context.Context, accountID string) (Account, error) {
	var a v2Account
	q := url.Values{"include": {"configuration.merchant", "requirements"}}
	err := c.doV2(ctx, http.MethodGet, "/v2/core/accounts/"+url.PathEscape(accountID)+"?"+q.Encode(), nil, &a)
	return a.toAccount(), err
}
