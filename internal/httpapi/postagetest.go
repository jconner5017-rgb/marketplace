package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/postage"
	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

// The admin "Test envelope postage" page: asks EasyPost for First-Class
// letter rates on an envelope, and with a TEST key buys one to see the
// label. Nothing here touches orders; buying is refused with a live key.

// GET /v1/admin/postage-test
func (s *Server) handlePostageTest(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	out := map[string]any{"configured": s.postage != nil}
	if s.postage != nil {
		out["test_mode"] = s.postage.TestMode()
	}
	var from sql.NullString
	if err := s.db.QueryRowContext(r.Context(), `SELECT ship_from FROM seller_profiles WHERE user_id = $1`, u.ID).Scan(&from); err == nil && from.Valid {
		var a shipping.Address
		if json.Unmarshal([]byte(from.String), &a) == nil {
			out["from"] = a
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) postageOff(w http.ResponseWriter) bool {
	if s.postage == nil {
		writeError(w, http.StatusServiceUnavailable, "postage_off", "Envelope postage isn't set up: add EASYPOST_API_KEY on marketplace-api.", nil)
		return true
	}
	return false
}

func postageError(w http.ResponseWriter, err error) {
	var api *postage.APIError
	if errors.As(err, &api) {
		writeError(w, http.StatusBadGateway, "easypost", api.Error(), nil)
		return
	}
	writeError(w, http.StatusUnprocessableEntity, "invalid", err.Error(), nil)
}

// POST /v1/admin/postage-test/rates {from, to, package, weight_oz, label_size}
func (s *Server) handlePostageTestRates(w http.ResponseWriter, r *http.Request) {
	if s.postageOff(w) {
		return
	}
	var in struct {
		From      shipping.Address `json:"from"`
		To        shipping.Address `json:"to"`
		Package   string           `json:"package"`
		WeightOz  float64          `json:"weight_oz"`
		LabelSize string           `json:"label_size"`
		NonMach   bool             `json:"non_machinable"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	sh, err := s.postage.LetterRates(r.Context(), postage.Letter{From: in.From, To: in.To, Package: in.Package, WeightOz: in.WeightOz,
		LabelSize: in.LabelSize, NonMachinable: in.NonMach})
	if err != nil {
		postageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sh)
}

// POST /v1/admin/postage-test/buy {shipment_id, rate_id} - test key only.
func (s *Server) handlePostageTestBuy(w http.ResponseWriter, r *http.Request) {
	if s.postageOff(w) {
		return
	}
	if !s.postage.TestMode() {
		writeError(w, http.StatusConflict, "live_key", "This is a live EasyPost key: test labels can only be bought with the Test key (EZTK...), so nothing is charged.", nil)
		return
	}
	var in struct {
		ShipmentID string `json:"shipment_id"`
		RateID     string `json:"rate_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ShipmentID == "" || in.RateID == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid", "Pick a rate first.", nil)
		return
	}
	l, err := s.postage.Buy(r.Context(), in.ShipmentID, in.RateID)
	if err != nil {
		postageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}
