package checkout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

// StampCents is a USPS Forever stamp (1 oz letter).
const StampCents = 82

// NonmachinableStampCents is the stamp for a letter USPS can't run through
// its machines, such as a thick card or a very rigid toploader: the Forever rate
// plus the 49¢ nonmachinable surcharge. USPS sells $1.31 stamps for this.
const NonmachinableStampCents = 131

// Stripe's US card pricing, charged on each order and taken from the
// seller's Stripe balance.
const (
	StripePercentBps = 290
	StripeFixedCents = 30
)

// StripeFeeCents is Stripe's fee on a card payment of the given size.
func StripeFeeCents(amount int) int {
	if amount <= 0 {
		return 0
	}
	return int(math.Round(float64(amount)*StripePercentBps/10000)) + StripeFixedCents
}

// ShippingEstimate is what shipping one card will cost the seller.
type ShippingEstimate struct {
	Method     string `json:"method"`      // "stamp" or "label"
	CostCents  int    `json:"cost_cents"`  // what the seller pays, label card processing included
	LabelCents int    `json:"label_cents"` // the carrier's price for the label
	Carrier    string `json:"carrier,omitempty"`
	Service    string `json:"service,omitempty"`
	Live       bool   `json:"live"`     // a real quote from the label company
	FromSet    bool   `json:"from_set"` // quoted from the seller's own ship-from address
	Signature  bool   `json:"signature"`
}

// Quotes go to the far side of the country, so the estimate is close to the
// most a label will cost; nearer buyers cost less.
var (
	eastDest    = shipping.Address{Name: "Estimate", Street1: "350 5th Ave", City: "New York", State: "NY", Zip: "10118", Country: "US"}
	westDest    = shipping.Address{Name: "Estimate", Street1: "400 Broad St", City: "Seattle", State: "WA", Zip: "98109", Country: "US"}
	centralFrom = shipping.Address{Name: "Estimate", Street1: "30 W Pershing Rd", City: "Kansas City", State: "MO", Zip: "64108", Country: "US"}
)

// Typical labels when no live quote is available.
var fallbackLabel = map[string]int{"bubble": 480, "box": 650}

type estimateKey struct {
	zip       string
	parcel    shipping.Parcel
	signature bool
}

type cachedRate struct {
	rate shipping.Rate
	at   time.Time
}

var (
	estimateMu    sync.Mutex
	estimateCache = map[estimateKey]cachedRate{}
)

// EstimateShipping prices shipping one card listed with this package. An
// envelope is a Forever stamp; anything else is a live label quote from the
// seller's ship-from address (or the middle of the country when it isn't set
// yet), falling back to a typical price when no quote can be had.
func (s *Service) EstimateShipping(ctx context.Context, sellerID, preset string, weightOz float64, graded, nonmachinable bool, priceCents int) (ShippingEstimate, error) {
	if preset == "envelope" && !graded {
		if nonmachinable {
			return ShippingEstimate{Method: "stamp", CostCents: NonmachinableStampCents, LabelCents: NonmachinableStampCents, Service: "Nonmachinable stamp", Live: true}, nil
		}
		return ShippingEstimate{Method: "stamp", CostCents: StampCents, LabelCents: StampCents, Service: "Forever stamp", Live: true}, nil
	}
	if preset != "bubble" && preset != "box" {
		preset = "bubble"
	}
	if weightOz <= 0 || weightOz > 160 {
		weightOz = 1
	}
	parcel := shipping.ParcelFor([]shipping.Item{{Preset: preset, WeightOz: weightOz, Quantity: 1, Graded: graded}})
	sig := priceCents > SignatureOver
	est := ShippingEstimate{Method: "label", Signature: sig}
	if graded {
		preset = "box"
	}

	from := centralFrom
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT ship_from FROM seller_profiles WHERE user_id = $1`, sellerID).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return est, err
	}
	if len(raw) > 0 {
		var a shipping.Address
		if json.Unmarshal(raw, &a) == nil && a.Zip != "" {
			from, est.FromSet = a, true
		}
	}
	to := westDest
	if z := strings.TrimSpace(from.Zip); z != "" && z[0] >= '5' {
		to = eastDest
	}

	if s.Ship != nil {
		if r, ok := s.quote(ctx, from, to, parcel, sig); ok {
			est.LabelCents, est.Carrier, est.Service, est.Live = r.AmountCents, r.Carrier, r.Service, true
		}
	}
	if !est.Live {
		est.LabelCents = fallbackLabel[preset]
		est.Service = "USPS Ground Advantage (typical)"
	}
	est.CostCents = shipping.LabelChargeCents(est.LabelCents)
	return est, nil
}

// quote returns the cheapest USPS rate (or the cheapest of any carrier),
// cached for 12 hours per origin ZIP and parcel.
func (s *Service) quote(ctx context.Context, from, to shipping.Address, p shipping.Parcel, sig bool) (shipping.Rate, bool) {
	p.WeightOz = math.Ceil(p.WeightOz)
	key := estimateKey{zip: from.Zip + ">" + to.Zip, parcel: p, signature: sig}
	estimateMu.Lock()
	if c, ok := estimateCache[key]; ok && time.Since(c.at) < 12*time.Hour {
		estimateMu.Unlock()
		return c.rate, true
	}
	estimateMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	rates, err := s.Ship.Rates(ctx, shipping.RateRequest{From: from, To: to, Parcel: p, Signature: sig,
		Reference: fmt.Sprintf("Estimate %s", from.Zip)})
	if err != nil || len(rates) == 0 {
		return shipping.Rate{}, false
	}
	best, bestUSPS := -1, -1
	for i, r := range rates {
		if best < 0 || r.AmountCents < rates[best].AmountCents {
			best = i
		}
		if strings.EqualFold(r.Carrier, "USPS") && (bestUSPS < 0 || r.AmountCents < rates[bestUSPS].AmountCents) {
			bestUSPS = i
		}
	}
	if bestUSPS >= 0 {
		best = bestUSPS
	}
	r := rates[best]
	estimateMu.Lock()
	if len(estimateCache) > 5000 {
		estimateCache = map[estimateKey]cachedRate{}
	}
	estimateCache[key] = cachedRate{rate: r, at: time.Now()}
	estimateMu.Unlock()
	return r, true
}
