// Package sellers manages seller accounts: Stripe Connect onboarding,
// account status, tiers and selling settings.
package sellers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/stripe"
)

var (
	ErrNotSeller           = errors.New("not a seller yet")
	ErrStripeNotConfigured = errors.New("stripe is not configured")
)

// TierCapCents is the highest price a seller in each tier may list a card at.
var TierCapCents = map[int]int{1: 2000, 2: 25000, 3: 100000}

type Profile struct {
	UserID             string    `json:"-"`
	StripeAccountID    string    `json:"stripe_account_id"`
	ChargesEnabled     bool      `json:"charges_enabled"`
	DetailsSubmitted   bool      `json:"details_submitted"`
	Tier               int       `json:"tier"`
	TierCapCents       int       `json:"tier_cap_cents"`
	BundleDiscountType *string   `json:"bundle_discount_type"`
	BundleDiscountVal  *int      `json:"bundle_discount_val"`
	FCMConnected       bool      `json:"fcm_connected"`
	CreatedAt          time.Time `json:"created_at"`
	ReviewStatus       string    `json:"review_status"` // pending, approved, rejected (staff review)
	CollectTax         bool      `json:"collect_tax"`   // Stripe Tax adds sales tax at checkout
	// StripeLive: the account was made with a live key (false: sandbox).
	StripeLive bool `json:"-"`
	// From Stripe on a refresh (not stored): what Stripe still needs before
	// the seller can take payments, and why charges are off.
	StripeNeeds     []string `json:"stripe_needs,omitempty"`
	StripeVerifying []string `json:"stripe_verifying,omitempty"`
	StripeReason    string   `json:"stripe_reason,omitempty"`
}

// CanSell reports whether the seller may publish listings.
func (p Profile) CanSell() bool { return p.ChargesEnabled }

type Service struct {
	DB     *sql.DB
	Stripe stripe.Connect // nil when STRIPE_SECRET_KEY is not set
	// Live: STRIPE_SECRET_KEY is a live key. Accounts made in the other mode
	// can't be used with it and are replaced when the seller sets up again.
	Live bool
}

// demoPrefix marks the sample shop's made-up account (see internal/demo).
const demoPrefix = "acct_demo_"

// otherMode reports a seller account this key can't use: a test seller, or
// one made in sandbox while the site is live (or the reverse).
func (s *Service) otherMode(accountID string, live bool) bool {
	if strings.HasPrefix(accountID, TestAccountPrefix) {
		return true
	}
	return !strings.HasPrefix(accountID, demoPrefix) && live != s.Live
}

const profileCols = `user_id, stripe_account_id, charges_enabled, details_submitted, tier,
	bundle_discount_type, bundle_discount_val, fcm_connected, created_at, review_status, collect_tax, stripe_livemode`

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	var bt sql.NullString
	var bv sql.NullInt64
	err := row.Scan(&p.UserID, &p.StripeAccountID, &p.ChargesEnabled, &p.DetailsSubmitted, &p.Tier,
		&bt, &bv, &p.FCMConnected, &p.CreatedAt, &p.ReviewStatus, &p.CollectTax, &p.StripeLive)
	if bt.Valid {
		p.BundleDiscountType = &bt.String
	}
	if bv.Valid {
		v := int(bv.Int64)
		p.BundleDiscountVal = &v
	}
	p.TierCapCents = TierCapCents[p.Tier]
	return p, err
}

// Get returns the user's seller profile, or ErrNotSeller.
func (s *Service) Get(ctx context.Context, userID string) (Profile, error) {
	p, err := scanProfile(s.DB.QueryRowContext(ctx,
		`SELECT `+profileCols+` FROM seller_profiles WHERE user_id = $1`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotSeller
	}
	return p, err
}

// StartOnboarding creates the user's Stripe Standard account on first use,
// then returns a fresh Stripe-hosted onboarding link.
func (s *Service) StartOnboarding(ctx context.Context, userID, email, refreshURL, returnURL string) (string, error) {
	if s.Stripe == nil {
		return "", ErrStripeNotConfigured
	}
	accountID, err := s.ensureAccount(ctx, userID, email)
	if err != nil {
		return "", err
	}
	return s.Stripe.CreateOnboardingLink(ctx, accountID, refreshURL, returnURL)
}

// TestAccountPrefix marks seller accounts set up without Stripe, for admin
// testing before Stripe Connect is switched on.
const TestAccountPrefix = "acct_test_"

// StartTestSeller makes userID a seller who can list right away, with no
// Stripe account. Only for admins while checkout is off (the caller checks).
func (s *Service) StartTestSeller(ctx context.Context, userID string) error {
	var b [8]byte
	rand.Read(b[:])
	acct := TestAccountPrefix + hex.EncodeToString(b[:])
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO seller_profiles (user_id, stripe_account_id, charges_enabled, details_submitted, review_status)
		VALUES ($1, $2, true, true, 'approved')
		ON CONFLICT (user_id) DO UPDATE SET stripe_account_id = EXCLUDED.stripe_account_id, charges_enabled = true,
			details_submitted = true, review_status = 'approved', updated_at = now()
		WHERE NOT seller_profiles.charges_enabled`, userID, acct)
	return err
}

// RetireTestSellers runs once checkout is live: test sellers (set up
// without Stripe) can't take payments, so they go back to "set up payments"
// and are sent through real Stripe onboarding, keeping their listings.
func (s *Service) RetireTestSellers(ctx context.Context) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE seller_profiles SET charges_enabled = false, details_submitted = false, updated_at = now()
		WHERE starts_with(stripe_account_id, $1) AND (charges_enabled OR details_submitted)`, TestAccountPrefix)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RetireOtherModeSellers runs at startup: sellers whose Stripe account was
// made in the other mode (sandbox accounts once the live key is in) go back
// to "set up payments". Their listings stay; setting up makes a new account.
func (s *Service) RetireOtherModeSellers(ctx context.Context) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE seller_profiles SET charges_enabled = false, details_submitted = false, updated_at = now()
		WHERE stripe_livemode <> $1 AND NOT starts_with(stripe_account_id, $2) AND NOT starts_with(stripe_account_id, $3)
		  AND (charges_enabled OR details_submitted)`, s.Live, TestAccountPrefix, demoPrefix)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ClearOtherModeLabelCards runs at startup: a saved label card made under the
// other Stripe key (the sandbox, once the live key is in) can't be charged or
// added to, so it's dropped; the seller saves a card again in Shipping.
func (s *Service) ClearOtherModeLabelCards(ctx context.Context) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE seller_profiles SET stripe_customer_id = NULL, label_payment_method = NULL, label_card = NULL,
			stripe_customer_livemode = $1, updated_at = now()
		WHERE stripe_customer_id IS NOT NULL AND stripe_customer_livemode <> $1`, s.Live)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Service) ensureAccount(ctx context.Context, userID, email string) (string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// Lock the user row so two clicks can't create two Stripe accounts.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
		return "", err
	}
	var existing string
	var live bool
	err = tx.QueryRowContext(ctx,
		`SELECT stripe_account_id, stripe_livemode FROM seller_profiles WHERE user_id = $1`, userID).Scan(&existing, &live)
	if err == nil && s.otherMode(existing, live) {
		// A test seller, or a sandbox account now the site is live, gets a
		// new Stripe account; listings stay, selling waits for onboarding.
		acct, err := s.Stripe.CreateStandardAccount(ctx, email)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE seller_profiles SET stripe_account_id = $2, charges_enabled = $3, details_submitted = $4,
				stripe_livemode = $5, updated_at = now()
			WHERE user_id = $1`, userID, acct.ID, acct.ChargesEnabled, acct.DetailsSubmitted, s.Live); err != nil {
			return "", err
		}
		return acct.ID, tx.Commit()
	}
	if err == nil {
		return existing, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	acct, err := s.Stripe.CreateStandardAccount(ctx, email)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO seller_profiles (user_id, stripe_account_id, charges_enabled, details_submitted, stripe_livemode)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, acct.ID, acct.ChargesEnabled, acct.DetailsSubmitted, s.Live); err != nil {
		return "", err
	}
	return acct.ID, tx.Commit()
}

// Refresh pulls the account's current state from Stripe.
func (s *Service) Refresh(ctx context.Context, userID string) (Profile, error) {
	p, err := s.Get(ctx, userID)
	if err != nil {
		return p, err
	}
	if s.otherMode(p.StripeAccountID, p.StripeLive) {
		return p, nil // no usable Stripe account behind it yet
	}
	if s.Stripe == nil {
		return p, ErrStripeNotConfigured
	}
	acct, err := s.Stripe.GetAccount(ctx, p.StripeAccountID)
	if err != nil {
		return p, err
	}
	if err := s.ApplyAccount(ctx, acct); err != nil {
		return p, err
	}
	fresh, err := s.Get(ctx, userID)
	if err == nil && !acct.ChargesEnabled {
		fresh.StripeNeeds = append(append([]string{}, acct.Requirements.PastDue...), acct.Requirements.CurrentlyDue...)
		fresh.StripeVerifying = acct.Requirements.PendingVerification
		fresh.StripeReason = acct.Requirements.DisabledReason
	}
	return fresh, err
}

// ApplyAccount stores Stripe's view of an account (from a refresh or webhook).
func (s *Service) ApplyAccount(ctx context.Context, acct stripe.Account) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE seller_profiles
		SET charges_enabled = $2, details_submitted = $3, updated_at = now()
		WHERE stripe_account_id = $1`,
		acct.ID, acct.ChargesEnabled, acct.DetailsSubmitted)
	return err
}

// Disconnect marks a seller whose Stripe account was disconnected from the
// platform as unable to sell.
func (s *Service) Disconnect(ctx context.Context, stripeAccountID string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE seller_profiles SET charges_enabled = false, updated_at = now()
		WHERE stripe_account_id = $1`, stripeAccountID)
	return err
}

// MarkEventProcessed records a webhook event ID. It returns false when the
// event was already handled (Stripe retries deliveries).
func (s *Service) MarkEventProcessed(ctx context.Context, id, typ string) (bool, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO stripe_events (id, type) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, id, typ)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ValidationError lists problems with submitted fields.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid input" }

// UpdateBundleDiscount sets or clears the seller's multi-card discount.
// per_extra_item: val = cents off each card after the first (max $5).
// percent_3plus:  val = basis points off when 3+ cards (max 50%).
func (s *Service) UpdateBundleDiscount(ctx context.Context, userID string, typ *string, val *int) (Profile, error) {
	if _, err := s.Get(ctx, userID); err != nil {
		return Profile{}, err
	}
	switch {
	case typ == nil && val == nil:
	case typ == nil || val == nil:
		return Profile{}, &ValidationError{map[string]string{"bundle_discount": "set both type and value, or neither"}}
	case *typ == "per_extra_item":
		if *val < 1 || *val > 500 {
			return Profile{}, &ValidationError{map[string]string{"bundle_discount_val": "1 to 500 cents"}}
		}
	case *typ == "percent_3plus":
		if *val < 1 || *val > 5000 {
			return Profile{}, &ValidationError{map[string]string{"bundle_discount_val": "1 to 5000 basis points (0.01% to 50%)"}}
		}
	default:
		return Profile{}, &ValidationError{map[string]string{"bundle_discount_type": "per_extra_item or percent_3plus"}}
	}
	if _, err := s.DB.ExecContext(ctx, `
		UPDATE seller_profiles SET bundle_discount_type = $2, bundle_discount_val = $3, updated_at = now()
		WHERE user_id = $1`, userID, typ, val); err != nil {
		return Profile{}, err
	}
	return s.Get(ctx, userID)
}

// SetCollectTax turns Stripe Tax on or off for the seller's checkouts. The
// seller must have Stripe Tax set up on their own Stripe account first.
func (s *Service) SetCollectTax(ctx context.Context, userID string, on bool) (Profile, error) {
	if _, err := s.Get(ctx, userID); err != nil {
		return Profile{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE seller_profiles SET collect_tax = $2, updated_at = now() WHERE user_id = $1`,
		userID, on); err != nil {
		return Profile{}, err
	}
	return s.Get(ctx, userID)
}

// RefreshPending asks Stripe about sellers still finishing setup, so they're
// switched on even if no account webhook arrives (Accounts v2 sends its own
// event types). It returns how many it checked.
func (s *Service) RefreshPending(ctx context.Context) (int, error) {
	if s.Stripe == nil {
		return 0, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM seller_profiles
		WHERE NOT charges_enabled AND stripe_livemode = $1
		  AND NOT starts_with(stripe_account_id, $2) AND NOT starts_with(stripe_account_id, $3)
		  AND updated_at > now() - interval '30 days'
		ORDER BY updated_at DESC LIMIT 50`, s.Live, TestAccountPrefix, demoPrefix)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := s.Refresh(ctx, id); err != nil {
			return len(ids), fmt.Errorf("refresh seller %s: %w", id, err)
		}
	}
	return len(ids), nil
}
