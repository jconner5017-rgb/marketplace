package postage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

func TestLetter(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, _, _ := r.BasicAuth(); u != "EZTKtest" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/shipments":
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Write([]byte(`{"id":"shp_1","rates":[{"id":"rate_1","carrier":"USPS","service":"First","rate":"0.78","currency":"USD"}],"messages":[]}`))
		case "/shipments/shp_1/buy":
			w.Write([]byte(`{"id":"shp_1","tracking_code":"9400","selected_rate":{"id":"rate_1","carrier":"USPS","service":"First","rate":"0.78"},"postage_label":{"label_url":"https://x/l.png","label_pdf_url":"https://x/l.pdf","label_size":"7x3"}}`))
		}
	}))
	defer srv.Close()
	e := New("EZTKtest")
	e.Base = srv.URL
	if !e.TestMode() {
		t.Fatal("test mode")
	}
	a := shipping.Address{Name: "A", Street1: "1 Main", City: "X", State: "NY", Zip: "10001"}
	sh, err := e.LetterRates(context.Background(), Letter{From: a, To: a, Package: "Letter", WeightOz: 1, LabelSize: "7x3"})
	if err != nil || len(sh.Rates) != 1 || sh.Rates[0].Service != "First" {
		t.Fatalf("%v %v", sh, err)
	}
	parcel := got["shipment"].(map[string]any)["parcel"].(map[string]any)
	if parcel["predefined_package"] != "Letter" || got["shipment"].(map[string]any)["to_address"].(map[string]any)["country"] != "US" {
		t.Fatalf("request: %v", got)
	}
	l, err := e.Buy(context.Background(), "shp_1", "rate_1")
	if err != nil || l.LabelURL != "https://x/l.pdf" || l.Rate.Rate != "0.78" {
		t.Fatalf("%v %v", l, err)
	}
	if _, err := New("bad").LetterRates(context.Background(), Letter{From: a, To: a, Package: "Box", WeightOz: 1}); err == nil {
		t.Fatal("box allowed")
	}
}

func TestTrack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"shp_1","tracking_code":"9400","tracker":{"status":"out_for_delivery","tracking_details":[
			{"status":"pre_transit","message":"Shipping Label Created","datetime":"2026-11-18T10:00:00Z"},
			{"status":"in_transit","message":"Accepted at USPS Origin Facility","datetime":"2026-11-18T20:00:00Z","tracking_location":{"city":"AUSTIN","state":"TX"}},
			{"status":"out_for_delivery","message":"Out for Delivery","datetime":"2026-11-19T08:00:00Z","tracking_location":{"city":"PLANO","state":"TX"}}]}}`))
	}))
	defer srv.Close()
	e := New("EZTKtest")
	e.Base = srv.URL
	tr, err := e.Track(context.Background(), "shp_1")
	if err != nil || tr.Status != "out_for_delivery" || len(tr.Events) != 2 {
		t.Fatalf("%+v %v", tr, err)
	}
	if ev := tr.Events[1]; ev.Status != shipping.InTransit || ev.Description != "Out for Delivery" || ev.Location != "PLANO TX" || ev.TrackingNumber != "9400" {
		t.Fatalf("%+v", ev)
	}
}
