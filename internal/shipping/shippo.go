package shipping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Shippo is the Shippo adapter (https://docs.goshippo.com). A test token
// (shippo_test_...) buys free test labels.
type Shippo struct {
	Token   string
	BaseURL string // defaults to https://api.goshippo.com
	HTTP    *http.Client
}

func NewShippo(token string) *Shippo {
	return &Shippo{Token: token, BaseURL: "https://api.goshippo.com", HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (s *Shippo) Name() string { return "shippo" }

type shippoAddress struct {
	Name    string `json:"name"`
	Street1 string `json:"street1"`
	Street2 string `json:"street2,omitempty"`
	City    string `json:"city"`
	State   string `json:"state"`
	Zip     string `json:"zip"`
	Country string `json:"country"`
	Phone   string `json:"phone,omitempty"`
	Email   string `json:"email,omitempty"`
}

func toShippo(a Address) shippoAddress {
	c := a.Country
	if c == "" {
		c = "US"
	}
	return shippoAddress{a.Name, a.Street1, a.Street2, a.City, a.State, a.Zip, c, a.Phone, a.Email}
}

func (s *Shippo) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "ShippoToken "+s.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("shippo: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("shippo %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(b[:min(len(b), 300)])))
	}
	return json.Unmarshal(b, out)
}

func num(f float64) string { return strconv.FormatFloat(math.Round(f*100)/100, 'f', -1, 64) }

func (s *Shippo) Rates(ctx context.Context, r RateRequest) ([]Rate, error) {
	in := map[string]any{
		"address_from": toShippo(r.From),
		"address_to":   toShippo(r.To),
		"parcels": []map[string]string{{
			"length": num(r.Parcel.Length), "width": num(r.Parcel.Width), "height": num(r.Parcel.Height),
			"distance_unit": "in", "weight": num(r.Parcel.WeightOz), "mass_unit": "oz",
		}},
		"async": false,
	}
	if r.Signature {
		in["extra"] = map[string]any{"signature_confirmation": "STANDARD"}
	}
	if r.Reference != "" {
		in["metadata"] = r.Reference
	}
	var out struct {
		Rates []struct {
			ObjectID      string `json:"object_id"`
			Amount        string `json:"amount"`
			Currency      string `json:"currency"`
			Provider      string `json:"provider"`
			EstimatedDays *int   `json:"estimated_days"`
			Servicelevel  struct {
				Name string `json:"name"`
			} `json:"servicelevel"`
		} `json:"rates"`
		Messages []struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := s.call(ctx, http.MethodPost, "/shipments/", in, &out); err != nil {
		return nil, err
	}
	var rates []Rate
	for _, x := range out.Rates {
		if x.Currency != "" && x.Currency != "USD" {
			continue
		}
		f, err := strconv.ParseFloat(x.Amount, 64)
		if err != nil || f <= 0 {
			continue
		}
		rt := Rate{ID: x.ObjectID, Carrier: x.Provider, Service: x.Servicelevel.Name, AmountCents: int(math.Round(f * 100))}
		if x.EstimatedDays != nil {
			rt.Days = *x.EstimatedDays
		}
		rates = append(rates, rt)
	}
	if len(rates) == 0 {
		return nil, ErrNoRates
	}
	return rates, nil
}

func (s *Shippo) BuyLabel(ctx context.Context, rateID string) (Label, error) {
	var out struct {
		ObjectID            string `json:"object_id"`
		Status              string `json:"status"`
		TrackingNumber      string `json:"tracking_number"`
		TrackingURLProvider string `json:"tracking_url_provider"`
		LabelURL            string `json:"label_url"`
		Messages            []struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := s.call(ctx, http.MethodPost, "/transactions/", map[string]any{"rate": rateID, "label_file_type": "PDF_4x6", "async": false}, &out); err != nil {
		return Label{}, err
	}
	if out.Status != "SUCCESS" {
		msg := out.Status
		if len(out.Messages) > 0 {
			msg = out.Messages[0].Text
		}
		return Label{}, fmt.Errorf("shippo couldn't buy the label: %s", msg)
	}
	return Label{ID: out.ObjectID, TrackingNumber: out.TrackingNumber, TrackingURL: out.TrackingURLProvider, LabelURL: out.LabelURL}, nil
}

func (s *Shippo) VoidLabel(ctx context.Context, labelID string) error {
	var out struct {
		Status string `json:"status"`
	}
	if err := s.call(ctx, http.MethodPost, "/refunds/", map[string]any{"transaction": labelID, "async": false}, &out); err != nil {
		return err
	}
	if out.Status == "ERROR" {
		return fmt.Errorf("shippo refused to void the label")
	}
	return nil
}

func (s *Shippo) ValidateAddress(ctx context.Context, a Address) (Address, error) {
	in := toShippo(a)
	body := map[string]any{"name": in.Name, "street1": in.Street1, "street2": in.Street2, "city": in.City, "state": in.State,
		"zip": in.Zip, "country": in.Country, "phone": in.Phone, "email": in.Email, "validate": true}
	var out struct {
		shippoAddress
		ValidationResults struct {
			IsValid  bool `json:"is_valid"`
			Messages []struct {
				Text string `json:"text"`
			} `json:"messages"`
		} `json:"validation_results"`
	}
	if err := s.call(ctx, http.MethodPost, "/addresses/", body, &out); err != nil {
		return Address{}, err
	}
	if !out.ValidationResults.IsValid {
		return Address{}, ErrBadAddress
	}
	return Address{Name: out.Name, Street1: out.Street1, Street2: out.Street2, City: out.City, State: out.State,
		Zip: out.Zip, Country: out.Country, Phone: a.Phone, Email: a.Email}, nil
}

var shippoStatus = map[string]string{
	"PRE_TRANSIT": PreTransit, "TRANSIT": InTransit, "DELIVERED": Delivered, "RETURNED": Returned, "FAILURE": Exception,
}

// ParseTrackingWebhook reads a Shippo track_updated event.
func (s *Shippo) ParseTrackingWebhook(r *http.Request) ([]TrackingEvent, error) {
	var in struct {
		Event string `json:"event"`
		Data  struct {
			TrackingNumber string `json:"tracking_number"`
			Carrier        string `json:"carrier"`
			TrackingStatus struct {
				Status        string    `json:"status"`
				StatusDetails string    `json:"status_details"`
				StatusDate    time.Time `json:"status_date"`
				Location      *struct {
					City  string `json:"city"`
					State string `json:"state"`
				} `json:"location"`
			} `json:"tracking_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		return nil, ErrBadWebhook
	}
	if in.Event != "track_updated" || in.Data.TrackingNumber == "" {
		return nil, ErrBadWebhook
	}
	st, ok := shippoStatus[in.Data.TrackingStatus.Status]
	if !ok {
		return nil, nil // UNKNOWN: nothing to record
	}
	loc := ""
	if l := in.Data.TrackingStatus.Location; l != nil && l.City != "" {
		loc = l.City + ", " + l.State
	}
	at := in.Data.TrackingStatus.StatusDate
	if at.IsZero() {
		at = time.Now()
	}
	return []TrackingEvent{{TrackingNumber: in.Data.TrackingNumber, Carrier: in.Data.Carrier, Status: st,
		Description: in.Data.TrackingStatus.StatusDetails, Location: loc, At: at}}, nil
}
