package shipping

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Fake is a provider for tests and for running without a label account.
type Fake struct {
	mu     sync.Mutex
	n      int
	rates  map[string]Rate
	labels map[string]Label
	Voided []string
}

func NewFake() *Fake { return &Fake{rates: map[string]Rate{}, labels: map[string]Label{}} }

func (f *Fake) Name() string { return "fake" }

func (f *Fake) Rates(_ context.Context, r RateRequest) ([]Rate, error) {
	if r.To.Zip == "" || r.From.Zip == "" {
		return nil, ErrBadAddress
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	base := 450 + int(r.Parcel.WeightOz)*20
	if r.Signature {
		base += 300
	}
	var out []Rate
	for i, svc := range []string{"Ground Advantage", "Priority Mail"} {
		f.n++
		rt := Rate{ID: fmt.Sprintf("rate_fake%d", f.n), Carrier: "USPS", Service: svc, AmountCents: base + i*400, Days: 4 - i*2}
		f.rates[rt.ID] = rt
		out = append(out, rt)
	}
	return out, nil
}

func (f *Fake) BuyLabel(_ context.Context, rateID string) (Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rates[rateID]; !ok {
		return Label{}, fmt.Errorf("fake: unknown rate %s", rateID)
	}
	f.n++
	l := Label{ID: fmt.Sprintf("lbl_fake%d", f.n), TrackingNumber: fmt.Sprintf("9400100000000000%06d", f.n),
		LabelURL: "https://labels.example.test/" + rateID + ".pdf", Carrier: "USPS"}
	f.labels[l.ID] = l
	return l, nil
}

func (f *Fake) VoidLabel(_ context.Context, labelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.labels[labelID]; !ok {
		return fmt.Errorf("fake: unknown label")
	}
	f.Voided = append(f.Voided, labelID)
	return nil
}

func (f *Fake) ValidateAddress(_ context.Context, a Address) (Address, error) {
	if a.Street1 == "" || a.City == "" || len(a.State) != 2 || len(a.Zip) < 5 || strings.Contains(strings.ToLower(a.Street1), "nowhere") {
		return Address{}, ErrBadAddress
	}
	a.State = strings.ToUpper(a.State)
	if a.Country == "" {
		a.Country = "US"
	}
	return a, nil
}

// ParseTrackingWebhook reads {"tracking_number", "status", "description"}.
func (f *Fake) ParseTrackingWebhook(r *http.Request) ([]TrackingEvent, error) {
	var e TrackingEvent
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil || e.TrackingNumber == "" {
		return nil, ErrBadWebhook
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	return []TrackingEvent{e}, nil
}
