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
	"math"
	"net/http"
	"strconv"
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
	ImageURL   string `json:"image_url,omitempty"` // the PNG
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

// Letter is a request for envelope postage.
type Letter struct {
	From, To      shipping.Address
	Package       string  // Letter, Flat or Card
	WeightOz      float64 // up to 3.5 (Letter, Card) or 13 (Flat)
	LabelSize     string  // 4x6, 7x3 or 6x4
	Format        string  // PDF (default) or PNG
	NonMachinable bool    // rigid / lumpy envelopes (toploaders) pay the surcharge
	Reference     string
}

// MaxOz is the heaviest First-Class piece of each kind.
func MaxOz(pkg string) float64 {
	if pkg == "Flat" {
		return 13
	}
	return 3.5
}

// LetterRates asks for rates on an envelope.
func (e *EasyPost) LetterRates(ctx context.Context, l Letter) (Shipment, error) {
	if !Packages[l.Package] {
		return Shipment{}, errors.New("package must be Letter, Flat or Card")
	}
	if !LabelSizes[l.LabelSize] {
		l.LabelSize = "4x6"
	}
	if l.Format != "PNG" {
		l.Format = "PDF"
	}
	if max := MaxOz(l.Package); l.WeightOz <= 0 || l.WeightOz > max {
		return Shipment{}, fmt.Errorf("a First-Class %s weighs up to %g oz", strings.ToLower(l.Package), max)
	}
	opts := map[string]any{"label_format": l.Format, "label_size": l.LabelSize}
	if l.NonMachinable {
		opts["machinable"] = false
	}
	sh := map[string]any{
		"from_address": addr(l.From), "to_address": addr(l.To),
		"parcel":  map[string]any{"predefined_package": l.Package, "weight": l.WeightOz},
		"options": opts,
	}
	if l.Reference != "" {
		sh["reference"] = l.Reference
	}
	var out Shipment
	err := e.call(ctx, http.MethodPost, "/shipments", map[string]any{"shipment": sh}, &out)
	return out, err
}

// FirstClass is the shipment's USPS First-Class rate, if it has one.
func (s Shipment) FirstClass() (Rate, bool) {
	for _, r := range s.Rates {
		if strings.EqualFold(r.Carrier, "USPS") && strings.EqualFold(r.Service, "First") {
			return r, true
		}
	}
	return Rate{}, false
}

// Cents is the rate in cents.
func (r Rate) Cents() (int, error) {
	f, err := strconv.ParseFloat(r.Rate, 64)
	if err != nil || f <= 0 {
		return 0, fmt.Errorf("bad rate %q", r.Rate)
	}
	return int(math.Round(f * 100)), nil
}

// Refund asks EasyPost to refund an unused label. USPS confirms it later.
func (e *EasyPost) Refund(ctx context.Context, shipmentID string) error {
	var out struct {
		RefundStatus string `json:"refund_status"`
	}
	if err := e.call(ctx, http.MethodPost, "/shipments/"+shipmentID+"/refund", map[string]any{}, &out); err != nil {
		return err
	}
	if out.RefundStatus == "rejected" || out.RefundStatus == "not_applicable" {
		return fmt.Errorf("USPS won't refund this postage (%s)", strings.ReplaceAll(out.RefundStatus, "_", " "))
	}
	return nil
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
	return Label{ShipmentID: out.ID, Tracking: out.TrackingCode, LabelURL: url, ImageURL: out.PostageLabel.LabelURL,
		Rate: out.SelectedRate, Size: out.PostageLabel.LabelSize}, nil
}
