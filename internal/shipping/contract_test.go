package shipping

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// contract is the suite every adapter must pass (rate, buy, void, address,
// webhook parsing). webhook is a raw delivered-event body in the provider's format.
func contract(t *testing.T, p Provider, webhook string) {
	ctx := context.Background()
	from := Address{Name: "Seller", Street1: "1 Market St", City: "San Francisco", State: "CA", Zip: "94105", Country: "US"}
	to := Address{Name: "Buyer", Street1: "200 Congress Ave", City: "Austin", State: "TX", Zip: "78701", Country: "US"}
	if _, err := p.ValidateAddress(ctx, to); err != nil {
		t.Fatalf("%s validate: %v", p.Name(), err)
	}
	rates, err := p.Rates(ctx, RateRequest{From: from, To: to, Parcel: ParcelFor([]Item{{Preset: "bubble", WeightOz: 1, Quantity: 2}})})
	if err != nil || len(rates) == 0 || rates[0].AmountCents <= 0 || rates[0].ID == "" {
		t.Fatalf("%s rates: %v %v", p.Name(), rates, err)
	}
	l, err := p.BuyLabel(ctx, rates[0].ID)
	if err != nil || l.ID == "" || l.TrackingNumber == "" || l.LabelURL == "" {
		t.Fatalf("%s buy: %+v %v", p.Name(), l, err)
	}
	if err := p.VoidLabel(ctx, l.ID); err != nil {
		t.Fatalf("%s void: %v", p.Name(), err)
	}
	req := httptest.NewRequest("POST", "/hook", strings.NewReader(webhook))
	evs, err := p.ParseTrackingWebhook(req)
	if err != nil || len(evs) != 1 || evs[0].Status != Delivered || evs[0].TrackingNumber == "" {
		t.Fatalf("%s webhook: %+v %v", p.Name(), evs, err)
	}
}

func TestFakeContract(t *testing.T) {
	contract(t, NewFake(), `{"tracking_number":"9400","status":"delivered","description":"Delivered, mailbox"}`)
}

// A stand-in for Shippo's API with the documented shapes.
func TestShippoContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ShippoToken shippo_test_x" {
			w.WriteHeader(401)
			return
		}
		var in map[string]any
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &in)
		switch r.URL.Path {
		case "/addresses/":
			if in["validate"] != true {
				w.WriteHeader(400)
				return
			}
			in["validation_results"] = map[string]any{"is_valid": true}
			json.NewEncoder(w).Encode(in)
		case "/shipments/":
			p := in["parcels"].([]any)[0].(map[string]any)
			if p["mass_unit"] != "oz" || p["distance_unit"] != "in" {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(`{"object_id":"shp_1","status":"SUCCESS","rates":[
				{"object_id":"rate_1","amount":"5.12","currency":"USD","provider":"USPS","estimated_days":3,"servicelevel":{"name":"Ground Advantage","token":"usps_ground_advantage"}},
				{"object_id":"rate_eur","amount":"4.00","currency":"EUR","provider":"X","servicelevel":{"name":"Y"}}]}`))
		case "/transactions/":
			if in["rate"] != "rate_1" {
				w.Write([]byte(`{"status":"ERROR","messages":[{"text":"bad rate"}]}`))
				return
			}
			w.Write([]byte(`{"object_id":"txn_1","status":"SUCCESS","tracking_number":"9205590164917312751089","label_url":"https://shippo.test/label.pdf","tracking_url_provider":"https://tools.usps.com/x"}`))
		case "/refunds/":
			w.Write([]byte(`{"object_id":"ref_1","status":"QUEUED"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	s := NewShippo("shippo_test_x")
	s.BaseURL = srv.URL
	contract(t, s, `{"event":"track_updated","test":true,"data":{"tracking_number":"9205590164917312751089","carrier":"usps",
		"tracking_status":{"status":"DELIVERED","status_details":"Delivered, in mailbox","status_date":"2026-10-29T15:04:05Z","location":{"city":"Austin","state":"TX"}}}}`)
	if rates, _ := s.Rates(context.Background(), RateRequest{From: Address{Zip: "1"}, To: Address{Zip: "2"}}); len(rates) != 1 || rates[0].AmountCents != 512 {
		t.Fatalf("rate parsing: %+v", rates)
	}
}

func TestLabelCharge(t *testing.T) {
	if LabelChargeCents(500) != 546 {
		t.Fatalf("got %d", LabelChargeCents(500))
	}
	if p := ParcelFor([]Item{{Preset: "bubble", WeightOz: 1, Quantity: 1}, {Graded: true, WeightOz: 4, Quantity: 1}}); p.Height != 2 || p.WeightOz != 9 {
		t.Fatalf("parcel: %+v", p)
	}
}
