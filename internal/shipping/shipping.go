// Package shipping hides which label company is in use. Everything outside
// this package works with these types; switching providers means writing
// one adapter and changing SHIPPING_PROVIDER.
package shipping

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type Address struct {
	Name    string `json:"name"`
	Street1 string `json:"line1"`
	Street2 string `json:"line2,omitempty"`
	City    string `json:"city"`
	State   string `json:"state"`
	Zip     string `json:"zip"`
	Country string `json:"country"`
	Phone   string `json:"phone,omitempty"`
	Email   string `json:"email,omitempty"`
}

// Parcel dimensions in inches, weight in ounces.
type Parcel struct {
	Length, Width, Height float64
	WeightOz              float64
}

type RateRequest struct {
	From, To  Address
	Parcel    Parcel
	Signature bool // signature confirmation (orders over $750)
	Reference string
}

type Rate struct {
	ID          string `json:"id"`
	Carrier     string `json:"carrier"`
	Service     string `json:"service"`
	AmountCents int    `json:"amount_cents"`
	Days        int    `json:"days"`
}

type Label struct {
	ID             string `json:"id"`
	TrackingNumber string `json:"tracking_number"`
	TrackingURL    string `json:"tracking_url"`
	LabelURL       string `json:"label_url"`
	Carrier        string `json:"carrier"`
}

// TrackingEvent statuses.
const (
	PreTransit = "pre_transit"
	InTransit  = "in_transit"
	Delivered  = "delivered"
	Returned   = "returned"
	Exception  = "exception"
)

type TrackingEvent struct {
	TrackingNumber string    `json:"tracking_number"`
	Carrier        string    `json:"carrier"`
	Status         string    `json:"status"`
	Description    string    `json:"description"`
	Location       string    `json:"location"`
	At             time.Time `json:"at"`
}

var (
	ErrBadAddress = errors.New("the carrier couldn't verify that address")
	ErrNoRates    = errors.New("no shipping rates are available for this package")
	ErrBadWebhook = errors.New("unrecognized tracking webhook")
)

type Provider interface {
	Name() string
	Rates(ctx context.Context, req RateRequest) ([]Rate, error)
	BuyLabel(ctx context.Context, rateID string) (Label, error)
	VoidLabel(ctx context.Context, labelID string) error
	ValidateAddress(ctx context.Context, a Address) (Address, error)
	// ParseTrackingWebhook turns the provider's raw webhook into our events.
	ParseTrackingWebhook(r *http.Request) ([]TrackingEvent, error)
}

// LabelChargeCents is what to charge the seller's card so the platform nets
// exactly labelCents after Stripe's 2.9% + 30¢. LabelChargeCents(500) == 546.
func LabelChargeCents(labelCents int) int {
	const fixed, pctBps = 30, 290
	num := (labelCents + fixed) * 10000
	den := 10000 - pctBps
	return (num + den - 1) / den
}

// Item is one line of an order for packing.
type Item struct {
	Preset   string // envelope, bubble, box
	WeightOz float64
	Quantity int
	Graded   bool
}

// ParcelFor packs an order's cards into one package: a bubble mailer, or a
// box when any card needs one (graded slabs, box preset, or many cards).
func ParcelFor(items []Item) Parcel {
	box := false
	w, cards := 0.0, 0
	for _, it := range items {
		if it.Preset == "box" || it.Graded {
			box = true
		}
		w += it.WeightOz * float64(max(it.Quantity, 1))
		cards += max(it.Quantity, 1)
	}
	if cards > 10 {
		box = true
	}
	if box {
		return Parcel{Length: 8, Width: 6, Height: 2, WeightOz: w + 4}
	}
	return Parcel{Length: 7.25, Width: 5.25, Height: 0.75, WeightOz: w + 1}
}

// Pick builds the configured provider by name.
func Pick(name string, cfg map[string]string) (Provider, error) {
	switch name {
	case "", "none":
		return nil, nil
	case "shippo":
		if cfg["SHIPPO_API_KEY"] == "" {
			return nil, errors.New("SHIPPING_PROVIDER=shippo needs SHIPPO_API_KEY")
		}
		return NewShippo(cfg["SHIPPO_API_KEY"]), nil
	case "fake":
		return NewFake(), nil
	}
	return nil, errors.New("unknown SHIPPING_PROVIDER " + name)
}
