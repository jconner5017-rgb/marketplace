// Package postage buys USPS First-Class letter postage (envelopes) through
// EasyPost, the one shipping API that sells letter and flat postage. Shippo
// (internal/shipping) only does packages.
package postage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

// Packages USPS First-Class Mail takes (EasyPost predefined packages).
var Packages = map[string]bool{"Letter": true, "Flat": true, "Card": true}

// LabelSizes EasyPost offers for letters.
var LabelSizes = map[string]bool{"4x6": true, "7x3": true, "6x4": true}

type EasyPost struct {
	Key  string
	Base string // https://api.easypost.com/v2 (tests point it elsewhere)
	HTTP *http.Client
}

func New(key string) *EasyPost {
	return &EasyPost{Key: strings.TrimSpace(key), Base: "https://api.easypost.com/v2", HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// TestMode is true for a test key (EZTK...): labels are free and not mailable.
func (e *EasyPost) TestMode() bool { return strings.HasPrefix(e.Key, "EZTK") }

type Rate struct {
	ID       string `json:"id"`
	Carrier  string `json:"carrier"`
	Service  string `json:"service"`
	Rate     string `json:"rate"`
	Currency string `json:"currency"`
	Days     *int   `json:"delivery_days"`
}

type Message struct {
	Carrier string `json:"carrier"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type Shipment struct {
	ID       string    `json:"id"`
	Rates    []Rate    `json:"rates"`
	Messages []Message `json:"messages"`
}

type Label struct {
	ShipmentID string `json:"shipment_id"`
	Tracking   string `json:"tracking_code"`
	LabelURL   string `json:"label_url"`
	Rate       Rate   `json:"rate"`
	Size       string `json:"label_size"`
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return "EasyPost: " + e.Message
	}
	return fmt.Sprintf("EasyPost error %d", e.Status)
}

func addr(a shipping.Address) map[string]string {
	country := a.Country
	if country == "" {
		country = "US"
	}
	return map[string]string{"name": a.Name, "street1": a.Street1, "street2": a.Street2, "city": a.City,
		"state": a.State, "zip": a.Zip, "country": country, "phone": a.Phone}
}

func (e *EasyPost) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.Base+path, rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth(e.Key, "")
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var eb struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &eb)
		if resp.StatusCode == http.StatusUnauthorized {
			eb.Error.Message = "the API key was refused"
		}
		return &APIError{Status: resp.StatusCode, Message: eb.Error.Message}
	}
	return json.Unmarshal(raw, out)
}

// LetterRates asks for rates on an envelope (Letter, Flat or Card).
func (e *EasyPost) LetterRates(ctx context.Context, from, to shipping.Address, pkg string, weightOz float64, labelSize string, nonMachinable bool) (Shipment, error) {
	if !Packages[pkg] {
		return Shipment{}, errors.New("package must be Letter, Flat or Card")
	}
	if !LabelSizes[labelSize] {
		labelSize = "4x6"
	}
	max := 13.0 // Flat (large envelope)
	if pkg != "Flat" {
		max = 3.5 // Letter and Card
	}
	if weightOz <= 0 || weightOz > max {
		return Shipment{}, fmt.Errorf("a First-Class %s weighs up to %g oz", strings.ToLower(pkg), max)
	}
	opts := map[string]any{"label_format": "PDF", "label_size": labelSize}
	if nonMachinable {
		opts["machinable"] = false // rigid / lumpy envelopes (toploaders) pay the surcharge
	}
	var out Shipment
	err := e.call(ctx, http.MethodPost, "/shipments", map[string]any{"shipment": map[string]any{
		"from_address": addr(from), "to_address": addr(to),
		"parcel":  map[string]any{"predefined_package": pkg, "weight": weightOz},
		"options": opts,
	}}, &out)
	return out, err
}

// Buy buys one of a shipment's rates and returns its label.
func (e *EasyPost) Buy(ctx context.Context, shipmentID, rateID string) (Label, error) {
	var out struct {
		ID           string `json:"id"`
		TrackingCode string `json:"tracking_code"`
		SelectedRate Rate   `json:"selected_rate"`
		PostageLabel struct {
			LabelURL    string `json:"label_url"`
			LabelPDFURL string `json:"label_pdf_url"`
			LabelSize   string `json:"label_size"`
		} `json:"postage_label"`
	}
	if err := e.call(ctx, http.MethodPost, "/shipments/"+shipmentID+"/buy", map[string]any{"rate": map[string]string{"id": rateID}}, &out); err != nil {
		return Label{}, err
	}
	url := out.PostageLabel.LabelPDFURL
	if url == "" {
		url = out.PostageLabel.LabelURL
	}
	return Label{ShipmentID: out.ID, Tracking: out.TrackingCode, LabelURL: url, Rate: out.SelectedRate, Size: out.PostageLabel.LabelSize}, nil
}
