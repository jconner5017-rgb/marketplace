// Package listings stores card listings and runs search.
package listings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"

	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

var (
	ErrNotFound        = errors.New("listing not found")
	ErrCannotSell      = errors.New("seller must finish Stripe onboarding before listing")
	ErrDuplicateExtern = errors.New("external_id already used by another listing")
)

// ValidationError lists problems with submitted fields, keyed by field name.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	slices.Sort(parts)
	return strings.Join(parts, "; ")
}

var (
	Conditions = []string{"NM", "LP", "MP", "HP", "DMG"}
	Graders    = []string{"PSA", "BGS", "SGC", "CGC", "TAG", "OTHER"}
	Attributes = []string{"holo", "reverse_holo", "first_edition", "shadowless", "foil", "full_art",
		"secret_rare", "rookie", "auto", "patch", "numbered", "parallel", "short_print", "error"}
	Presets  = []string{"envelope", "bubble", "box"}
	Statuses = []string{"active", "hidden"} // statuses a seller may set
)

// Input is a full listing as submitted by a seller, the CSV importer or
// FireCardManager.
type Input struct {
	ExternalID    *string  `json:"external_id"`
	Category      string   `json:"category"`
	GameOrSport   string   `json:"game_or_sport"`
	SetName       string   `json:"set_name"`
	Year          *int     `json:"year"`
	CardNumber    *string  `json:"card_number"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Condition     *string  `json:"condition"`
	Grader        *string  `json:"grader"`
	Grade         *float64 `json:"grade"`
	CertNumber    *string  `json:"cert_number"`
	Attributes    []string `json:"attributes"`
	PhotoURLs     []string `json:"photo_urls"`
	PriceCents    int      `json:"price_cents"`
	Quantity      int      `json:"quantity"`
	PackagePreset string   `json:"package_preset"`
	WeightOz      float64  `json:"weight_oz"`
	Status        string   `json:"status"`
}

// Patch changes only the fields that are present.
type Patch struct {
	Category      *string   `json:"category"`
	GameOrSport   *string   `json:"game_or_sport"`
	SetName       *string   `json:"set_name"`
	Year          *int      `json:"year"`
	CardNumber    *string   `json:"card_number"`
	Title         *string   `json:"title"`
	Description   *string   `json:"description"`
	Condition     *string   `json:"condition"`
	Grader        *string   `json:"grader"`
	Grade         *float64  `json:"grade"`
	CertNumber    *string   `json:"cert_number"`
	Attributes    *[]string `json:"attributes"`
	PhotoURLs     *[]string `json:"photo_urls"`
	PriceCents    *int      `json:"price_cents"`
	Quantity      *int      `json:"quantity"`
	PackagePreset *string   `json:"package_preset"`
	WeightOz      *float64  `json:"weight_oz"`
	Status        *string   `json:"status"`
	// Set true to make a graded card raw (clears grader/grade/cert).
	ClearGrading bool `json:"clear_grading"`
}

type Listing struct {
	ID             string    `json:"id"`
	SellerID       string    `json:"-"`
	SellerUsername string    `json:"seller_username"`
	ExternalID     *string   `json:"external_id,omitempty"`
	Category       string    `json:"category"`
	GameOrSport    string    `json:"game_or_sport"`
	SetName        string    `json:"set_name"`
	Year           *int      `json:"year"`
	CardNumber     *string   `json:"card_number"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Condition      *string   `json:"condition"`
	Grader         *string   `json:"grader"`
	Grade          *float64  `json:"grade"`
	CertNumber     *string   `json:"cert_number"`
	Attributes     []string  `json:"attributes"`
	PhotoURLs      []string  `json:"photo_urls"`
	PriceCents     int       `json:"price_cents"`
	Quantity       int       `json:"quantity"`
	PackagePreset  string    `json:"package_preset"`
	WeightOz       float64   `json:"weight_oz"`
	Status         string    `json:"status"`
	Version        int       `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (in *Input) normalize() {
	trim := func(p **string) {
		if *p != nil {
			v := strings.TrimSpace(**p)
			if v == "" {
				*p = nil
			} else {
				*p = &v
			}
		}
	}
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	in.GameOrSport = normalizeGameOrSport(in.Category, strings.TrimSpace(in.GameOrSport))
	in.SetName = strings.TrimSpace(in.SetName)
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	trim(&in.ExternalID)
	trim(&in.CardNumber)
	trim(&in.CertNumber)
	trim(&in.Condition)
	trim(&in.Grader)
	if in.Condition != nil {
		v := strings.ToUpper(*in.Condition)
		in.Condition = &v
	}
	if in.Grader != nil {
		v := strings.ToUpper(*in.Grader)
		in.Grader = &v
	}
	attrs := []string{}
	for _, a := range in.Attributes {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && !slices.Contains(attrs, a) {
			attrs = append(attrs, a)
		}
	}
	in.Attributes = attrs
	photos := []string{}
	for _, p := range in.PhotoURLs {
		if p = strings.TrimSpace(p); p != "" {
			photos = append(photos, p)
		}
	}
	in.PhotoURLs = photos
	if in.PackagePreset == "" {
		in.PackagePreset = "envelope"
	}
	if in.WeightOz == 0 {
		in.WeightOz = 1
	}
	if in.Status == "" {
		in.Status = "active"
	}
}

// validate checks a listing against the rules and the seller's price cap.
func (in *Input) validate(capCents int) error {
	f := map[string]string{}
	maxLen := func(field, v string, n int) {
		if utf8.RuneCountInString(v) > n {
			f[field] = fmt.Sprintf("at most %d characters", n)
		}
	}
	if in.Category != "sports" && in.Category != "tcg" {
		f["category"] = "sports or tcg"
	}
	switch {
	case in.GameOrSport == "":
		f["game_or_sport"] = "required (e.g. Pokemon, Baseball)"
	case in.Category == "sports" && !slices.Contains(Sports, in.GameOrSport):
		f["game_or_sport"] = "choose one of " + strings.Join(Sports, ", ")
	case in.Category == "tcg" && !slices.Contains(Games, in.GameOrSport):
		f["game_or_sport"] = "choose one of " + strings.Join(Games, ", ")
	}
	maxLen("game_or_sport", in.GameOrSport, 60)
	if in.SetName == "" {
		f["set_name"] = "required"
	}
	maxLen("set_name", in.SetName, 120)
	if in.Title == "" {
		f["title"] = "required"
	}
	maxLen("title", in.Title, 200)
	maxLen("description", in.Description, 5000)
	if in.ExternalID != nil {
		maxLen("external_id", *in.ExternalID, 100)
	}
	if in.CardNumber != nil {
		maxLen("card_number", *in.CardNumber, 30)
	}
	if in.Year != nil && (*in.Year < 1860 || *in.Year > time.Now().Year()+1) {
		f["year"] = "not a valid card year"
	}

	graded := in.Grader != nil || in.Grade != nil || in.CertNumber != nil
	if graded {
		if in.Grader == nil || !slices.Contains(Graders, *in.Grader) {
			f["grader"] = "one of " + strings.Join(Graders, ", ")
		}
		if in.Grade == nil || *in.Grade < 1 || *in.Grade > 10 || math.Mod(*in.Grade*2, 1) != 0 {
			f["grade"] = "1 to 10 in steps of 0.5"
		}
		if in.CertNumber != nil {
			maxLen("cert_number", *in.CertNumber, 30)
		}
		if in.Condition != nil && !slices.Contains(Conditions, *in.Condition) {
			f["condition"] = "one of " + strings.Join(Conditions, ", ")
		}
	} else if in.Condition == nil || !slices.Contains(Conditions, *in.Condition) {
		f["condition"] = "raw cards need one of " + strings.Join(Conditions, ", ")
	}

	for _, a := range in.Attributes {
		if !slices.Contains(Attributes, a) {
			f["attributes"] = "unknown attribute " + a
			break
		}
	}
	if len(in.PhotoURLs) == 0 {
		f["photo_urls"] = "at least one photo is required"
	} else if len(in.PhotoURLs) > 12 {
		f["photo_urls"] = "at most 12 photos"
	} else {
		for _, p := range in.PhotoURLs {
			u, err := url.Parse(p)
			if err != nil || u.Scheme != "https" || u.Host == "" || len(p) > 2048 {
				f["photo_urls"] = "each photo must be an https:// URL"
				break
			}
		}
	}

	switch {
	case in.PriceCents < 1:
		f["price_cents"] = "must be at least 1"
	case in.PriceCents > 100000:
		f["price_cents"] = "the marketplace limit is $1,000.00"
	case in.PriceCents > capCents:
		f["price_cents"] = fmt.Sprintf("your seller tier allows up to $%d.%02d per card", capCents/100, capCents%100)
	}
	if in.Quantity < 0 || in.Quantity > 10000 {
		f["quantity"] = "0 to 10000"
	}
	if !slices.Contains(Presets, in.PackagePreset) {
		f["package_preset"] = "one of " + strings.Join(Presets, ", ")
	}
	if in.WeightOz <= 0 || in.WeightOz > 1120 {
		f["weight_oz"] = "0.1 to 1120"
	}
	if !slices.Contains(Statuses, in.Status) {
		f["status"] = "active or hidden"
	}
	if len(f) > 0 {
		return &ValidationError{Fields: f}
	}
	return nil
}

type Service struct {
	DB      *sql.DB
	Sellers *sellers.Service
}

const listingCols = `l.id, l.seller_id, u.username, l.external_id, l.category, l.game_or_sport, l.set_name,
	l.year, l.card_number, l.title, l.description, l.condition, l.grader, l.grade, l.cert_number,
	l.attributes, l.photo_urls, l.price_cents, l.quantity, l.package_preset, l.weight_oz,
	l.status, l.version, l.created_at, l.updated_at`

func scanListing(row interface{ Scan(...any) error }) (Listing, error) {
	var l Listing
	var ext, cardNo, cond, grader, cert sql.NullString
	var year sql.NullInt64
	var grade sql.NullFloat64
	err := row.Scan(&l.ID, &l.SellerID, &l.SellerUsername, &ext, &l.Category, &l.GameOrSport, &l.SetName,
		&year, &cardNo, &l.Title, &l.Description, &cond, &grader, &grade, &cert,
		pq.Array(&l.Attributes), pq.Array(&l.PhotoURLs), &l.PriceCents, &l.Quantity, &l.PackagePreset,
		&l.WeightOz, &l.Status, &l.Version, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return l, err
	}
	str := func(n sql.NullString) *string {
		if !n.Valid {
			return nil
		}
		return &n.String
	}
	l.ExternalID, l.CardNumber, l.Condition, l.Grader, l.CertNumber = str(ext), str(cardNo), str(cond), str(grader), str(cert)
	if year.Valid {
		y := int(year.Int64)
		l.Year = &y
	}
	if grade.Valid {
		l.Grade = &grade.Float64
	}
	if l.Attributes == nil {
		l.Attributes = []string{}
	}
	if l.PhotoURLs == nil {
		l.PhotoURLs = []string{}
	}
	return l, nil
}

// sellerFor returns the seller profile and checks they can list.
func (s *Service) sellerFor(ctx context.Context, sellerID string) (sellers.Profile, error) {
	p, err := s.Sellers.Get(ctx, sellerID)
	if err != nil {
		return p, err
	}
	if !p.CanSell() {
		return p, ErrCannotSell
	}
	return p, nil
}

// Create adds a listing for a seller.
func (s *Service) Create(ctx context.Context, sellerID string, in Input) (Listing, error) {
	p, err := s.sellerFor(ctx, sellerID)
	if err != nil {
		return Listing{}, err
	}
	in.normalize()
	if err := in.validate(p.TierCapCents); err != nil {
		return Listing{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Listing{}, err
	}
	defer tx.Rollback()
	l, err := insertListing(ctx, tx, sellerID, in)
	if err != nil {
		return Listing{}, err
	}
	if err := snapshot(ctx, tx, l); err != nil {
		return Listing{}, err
	}
	return l, tx.Commit()
}

func status(in Input) string {
	if in.Status == "active" && in.Quantity == 0 {
		return "sold_out"
	}
	return in.Status
}

func insertListing(ctx context.Context, tx *sql.Tx, sellerID string, in Input) (Listing, error) {
	var id string
	err := tx.QueryRowContext(ctx, `
		INSERT INTO listings (seller_id, external_id, category, game_or_sport, set_name, year, card_number,
			title, description, condition, grader, grade, cert_number, attributes, photo_urls,
			price_cents, quantity, package_preset, weight_oz, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING id`,
		sellerID, in.ExternalID, in.Category, in.GameOrSport, in.SetName, in.Year, in.CardNumber,
		in.Title, in.Description, in.Condition, in.Grader, in.Grade, in.CertNumber,
		pq.Array(in.Attributes), pq.Array(in.PhotoURLs), in.PriceCents, in.Quantity,
		in.PackagePreset, in.WeightOz, status(in)).Scan(&id)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" && pqErr.Constraint == "listings_seller_external_key" {
			return Listing{}, ErrDuplicateExtern
		}
		return Listing{}, err
	}
	return getTx(ctx, tx, id)
}

func getTx(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (Listing, error) {
	l, err := scanListing(q.QueryRowContext(ctx, `
		SELECT `+listingCols+` FROM listings l JOIN users u ON u.id = l.seller_id WHERE l.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

func snapshot(ctx context.Context, tx *sql.Tx, l Listing) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO listing_versions (listing_id, version, snapshot) VALUES ($1, $2, $3)`, l.ID, l.Version, b)
	return err
}

// GetForSeller returns any of the seller's own listings (including hidden).
func (s *Service) GetForSeller(ctx context.Context, sellerID, id string) (Listing, error) {
	l, err := getTx(ctx, s.DB, id)
	if err != nil {
		return l, err
	}
	if l.SellerID != sellerID || l.Status == "removed" {
		return Listing{}, ErrNotFound
	}
	return l, nil
}

// GetPublic returns a listing buyers may see.
func (s *Service) GetPublic(ctx context.Context, id string) (Listing, error) {
	l, err := scanListing(s.DB.QueryRowContext(ctx, `
		SELECT `+listingCols+` FROM listings l `+publicJoin+` AND l.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// publicJoin limits results to listings buyers may see: active, from an
// active seller who can take payments, and priced within the seller's
// current tier (a demoted seller's pricier cards are hidden, not deleted).
const publicJoin = `
	JOIN users u ON u.id = l.seller_id AND u.status = 'active'
	JOIN seller_profiles sp ON sp.user_id = l.seller_id AND sp.charges_enabled
	WHERE l.status = 'active' AND l.quantity > 0
	  AND l.price_cents <= CASE sp.tier WHEN 1 THEN 2000 WHEN 2 THEN 25000 ELSE 100000 END`

// Update applies a patch to the seller's listing.
func (s *Service) Update(ctx context.Context, sellerID, id string, p Patch) (Listing, error) {
	prof, err := s.sellerFor(ctx, sellerID)
	if err != nil {
		return Listing{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Listing{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM listings WHERE id = $1 FOR UPDATE`, id); err != nil {
		return Listing{}, err
	}
	cur, err := getTx(ctx, tx, id)
	if errors.Is(err, ErrNotFound) || (err == nil && (cur.SellerID != sellerID || cur.Status == "removed")) {
		return Listing{}, ErrNotFound
	}
	if err != nil {
		return Listing{}, err
	}

	in := inputFrom(cur)
	applyPatch(&in, p)
	in.normalize()
	if err := in.validate(prof.TierCapCents); err != nil {
		return Listing{}, err
	}
	l, err := updateListing(ctx, tx, id, in)
	if err != nil {
		return Listing{}, err
	}
	if err := snapshot(ctx, tx, l); err != nil {
		return Listing{}, err
	}
	return l, tx.Commit()
}

func updateListing(ctx context.Context, tx *sql.Tx, id string, in Input) (Listing, error) {
	if _, err := tx.ExecContext(ctx, `
		UPDATE listings SET category=$2, game_or_sport=$3, set_name=$4, year=$5, card_number=$6,
			title=$7, description=$8, condition=$9, grader=$10, grade=$11, cert_number=$12,
			attributes=$13, photo_urls=$14, price_cents=$15, quantity=$16, package_preset=$17,
			weight_oz=$18, status=$19, version = version + 1, updated_at = now()
		WHERE id = $1`,
		id, in.Category, in.GameOrSport, in.SetName, in.Year, in.CardNumber, in.Title, in.Description,
		in.Condition, in.Grader, in.Grade, in.CertNumber, pq.Array(in.Attributes), pq.Array(in.PhotoURLs),
		in.PriceCents, in.Quantity, in.PackagePreset, in.WeightOz, status(in)); err != nil {
		return Listing{}, err
	}
	return getTx(ctx, tx, id)
}

func inputFrom(l Listing) Input {
	st := l.Status
	if st == "sold_out" {
		st = "active"
	}
	return Input{
		ExternalID: l.ExternalID, Category: l.Category, GameOrSport: l.GameOrSport, SetName: l.SetName,
		Year: l.Year, CardNumber: l.CardNumber, Title: l.Title, Description: l.Description,
		Condition: l.Condition, Grader: l.Grader, Grade: l.Grade, CertNumber: l.CertNumber,
		Attributes: l.Attributes, PhotoURLs: l.PhotoURLs, PriceCents: l.PriceCents, Quantity: l.Quantity,
		PackagePreset: l.PackagePreset, WeightOz: l.WeightOz, Status: st,
	}
}

func applyPatch(in *Input, p Patch) {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&in.Category, p.Category)
	set(&in.GameOrSport, p.GameOrSport)
	set(&in.SetName, p.SetName)
	set(&in.Title, p.Title)
	set(&in.Description, p.Description)
	set(&in.PackagePreset, p.PackagePreset)
	set(&in.Status, p.Status)
	if p.Year != nil {
		in.Year = p.Year
	}
	if p.CardNumber != nil {
		in.CardNumber = p.CardNumber
	}
	if p.ClearGrading {
		in.Grader, in.Grade, in.CertNumber = nil, nil, nil
	}
	if p.Condition != nil {
		in.Condition = p.Condition
	}
	if p.Grader != nil {
		in.Grader = p.Grader
	}
	if p.Grade != nil {
		in.Grade = p.Grade
	}
	if p.CertNumber != nil {
		in.CertNumber = p.CertNumber
	}
	if p.Attributes != nil {
		in.Attributes = *p.Attributes
	}
	if p.PhotoURLs != nil {
		in.PhotoURLs = *p.PhotoURLs
	}
	if p.PriceCents != nil {
		in.PriceCents = *p.PriceCents
	}
	if p.Quantity != nil {
		in.Quantity = *p.Quantity
	}
	if p.WeightOz != nil {
		in.WeightOz = *p.WeightOz
	}
}

// Remove takes a listing down. The row is kept for order history.
func (s *Service) Remove(ctx context.Context, sellerID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE listings SET status = 'removed', version = version + 1, updated_at = now()
		WHERE id = $1 AND seller_id = $2 AND status <> 'removed'`, id, sellerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertByExternalID creates or replaces the seller's listing with this
// external ID. Used by the CSV importer and FireCardManager. It returns
// whether a new listing was created.
func (s *Service) UpsertByExternalID(ctx context.Context, sellerID string, in Input) (Listing, bool, error) {
	p, err := s.sellerFor(ctx, sellerID)
	if err != nil {
		return Listing{}, false, err
	}
	in.normalize()
	if in.ExternalID == nil {
		return Listing{}, false, &ValidationError{map[string]string{"external_id": "required"}}
	}
	if err := in.validate(p.TierCapCents); err != nil {
		return Listing{}, false, err
	}
	return s.upsert(ctx, sellerID, in)
}

func (s *Service) upsert(ctx context.Context, sellerID string, in Input) (Listing, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Listing{}, false, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM listings WHERE seller_id = $1 AND external_id = $2 FOR UPDATE`,
		sellerID, *in.ExternalID).Scan(&id)
	var l Listing
	created := false
	switch {
	case errors.Is(err, sql.ErrNoRows):
		l, err = insertListing(ctx, tx, sellerID, in)
		created = true
	case err == nil:
		l, err = updateListing(ctx, tx, id, in)
	}
	if err != nil {
		return Listing{}, false, err
	}
	if err := snapshot(ctx, tx, l); err != nil {
		return Listing{}, false, err
	}
	return l, created, tx.Commit()
}

// ListForSeller returns the seller's own listings, newest first.
func (s *Service) ListForSeller(ctx context.Context, sellerID string, limit, offset int) ([]Listing, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM listings WHERE seller_id = $1 AND status <> 'removed'`, sellerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+listingCols+` FROM listings l JOIN users u ON u.id = l.seller_id
		WHERE l.seller_id = $1 AND l.status <> 'removed'
		ORDER BY l.created_at DESC, l.id LIMIT $2 OFFSET $3`, sellerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Listing{}
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// GetMany returns listings by ID in any status (for carts and watchlists,
// which must show cards that have since sold or been removed).
func (s *Service) GetMany(ctx context.Context, ids []string) (map[string]Listing, error) {
	out := map[string]Listing{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+listingCols+` FROM listings l JOIN users u ON u.id = l.seller_id
		WHERE l.id = ANY($1::uuid[])`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return nil, err
		}
		out[l.ID] = l
	}
	return out, rows.Err()
}

// PublicIDs reports which of ids buyers can currently purchase.
func (s *Service) PublicIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT l.id FROM listings l `+publicJoin+` AND l.id = ANY($1::uuid[])`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
