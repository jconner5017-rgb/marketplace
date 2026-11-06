package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/identify"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

// IdentifyDailyLimit is how many photo identifications a seller gets a day.
const IdentifyDailyLimit = 50

func identifyChoices() identify.Choices {
	cats, feats := listings.CatalogView()
	ch := identify.Choices{Kinds: map[string][]identify.Option{}, Graders: listings.Graders, Conditions: listings.Conditions}
	for _, c := range cats {
		ch.Kinds[c.Kind] = append(ch.Kinds[c.Kind], identify.Option{Value: c.Value, Label: c.Label})
	}
	for _, f := range feats {
		ch.Features = append(ch.Features, identify.Option{Value: f.Key, Label: f.Label})
	}
	return ch
}

// photoBytes reads one of the seller's own uploads. Only photos in the
// photos table owned by this seller are accepted, so nothing else is fetched.
func (s *Server) photoBytes(ctx context.Context, ownerID, url string) ([]byte, error) {
	var store, stored string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT store, url, data FROM photos WHERE url = $1 AND owner_id = $2`, url, ownerID).Scan(&store, &stored, &data)
	if err != nil {
		return nil, err
	}
	if store == "db" {
		return data, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stored, nil)
	if err != nil {
		return nil, err
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("photo store returned " + res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 10<<20))
}

// POST /v1/seller/identify {"photo_urls": [front, back]} — reads the card
// from the seller's own photos and returns details for the listing form.
func (s *Server) handleIdentify(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if !s.identify.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "identify_off", identify.ErrOff.Error(), nil)
		return
	}
	var in struct {
		PhotoURLs []string `json:"photo_urls"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.PhotoURLs) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no_photos", "add a photo of the card first", nil)
		return
	}
	if len(in.PhotoURLs) > 2 {
		in.PhotoURLs = in.PhotoURLs[:2] // the front and back are enough
	}
	var seller bool
	var used int
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM seller_profiles WHERE user_id = $1),
		(SELECT count(*) FROM identify_usage WHERE user_id = $1 AND created_at > now() - interval '1 day')`, u.ID).Scan(&seller, &used); err != nil {
		s.internalError(w, r, err)
		return
	}
	if !seller {
		writeError(w, http.StatusForbidden, "not_a_seller", "set up selling before identifying cards", nil)
		return
	}
	if used >= IdentifyDailyLimit && !u.IsStaff() {
		writeError(w, http.StatusTooManyRequests, "identify_limit", "you've used today's 50 photo identifications; fill in the details by hand or try again tomorrow", nil)
		return
	}
	images := make([][]byte, 0, len(in.PhotoURLs))
	for _, url := range in.PhotoURLs {
		b, err := s.photoBytes(r.Context(), u.ID, url)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnprocessableEntity, "bad_photo", "use photos you uploaded for this listing", nil)
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		images = append(images, b)
	}
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO identify_usage (user_id) VALUES ($1)`, u.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res, err := s.identify.Identify(ctx, images, identifyChoices())
	switch {
	case errors.Is(err, identify.ErrUnreadable):
		writeError(w, http.StatusUnprocessableEntity, "unreadable", err.Error(), nil)
		return
	case err != nil:
		s.log.Error("identify", "err", err)
		writeError(w, http.StatusBadGateway, "identify_failed", "identification didn't work this time; try again in a moment", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res, "left_today": max(0, IdentifyDailyLimit-used-1)})
}
