// Package stores runs business storefronts: the store page and branding,
// the seller's own categories, store-wide sales, followers and updates.
package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
)

var (
	ErrNotFound  = errors.New("store not found")
	ErrNotSeller = errors.New("set up payments and start selling before opening a store")
	ErrNotActive = errors.New("your store isn't open yet")
	ErrSlugTaken = errors.New("that store address is taken")
	ErrSaleClash = errors.New("you already have a sale running or scheduled for that time")
	ErrTooSoon   = errors.New("you can send followers one update every 7 days")
	ErrOwnStore  = errors.New("you can't follow your own store")
)

// FieldError is a problem with one form field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Field + ": " + e.Msg }

// Store is a business storefront. Business details (legal name, type,
// website, note) are only shown to the owner and staff.
type Store struct {
	SellerID     string     `json:"-"`
	Username     string     `json:"username"`
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Tagline      string     `json:"tagline"`
	About        string     `json:"about"`
	Policies     string     `json:"policies"`
	LogoURL      string     `json:"logo_url"`
	BannerURL    string     `json:"banner_url"`
	Status       string     `json:"status"`
	LegalName    string     `json:"legal_name,omitempty"`
	BusinessType string     `json:"business_type,omitempty"`
	Website      string     `json:"website,omitempty"`
	BusinessNote string     `json:"business_note,omitempty"`
	ReviewNote   string     `json:"review_note,omitempty"`
	Followers    int        `json:"followers"`
	Listings     int        `json:"listings"`
	Categories   []Category `json:"categories"`
	Sale         *Sale      `json:"sale"` // running now
	Following    bool       `json:"following"`
	LastUpdateAt *time.Time `json:"last_update_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
}

// Public drops the business details.
func (s Store) Public() Store {
	s.LegalName, s.BusinessType, s.Website, s.BusinessNote, s.ReviewNote, s.LastUpdateAt = "", "", "", "", "", nil
	return s
}

type Category struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Listings int    `json:"listings"`
}

type Sale struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Percent    int        `json:"percent"`
	CategoryID *int       `json:"category_id"`
	Category   string     `json:"category,omitempty"`
	StartsAt   time.Time  `json:"starts_at"`
	EndsAt     time.Time  `json:"ends_at"`
	Cancelled  *time.Time `json:"cancelled_at,omitempty"`
}

type Service struct {
	DB  *sql.DB
	Now func() time.Time
	// Email queues an update email to a follower (nil: bell notices only).
	Email func(ctx context.Context, tx *sql.Tx, userID, subject, body string) error
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,28}[a-z0-9])$`)

var reserved = map[string]bool{"admin": true, "new": true, "edit": true, "settings": true, "help": true, "api": true,
	"store": true, "stores": true, "sell": true, "seller": true, "slabs": true, "slabsnsingles": true, "support": true}

// CleanSlug turns a store address into its stored form, or explains what's wrong.
func CleanSlug(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if !slugRe.MatchString(v) || strings.Contains(v, "--") {
		return "", &FieldError{"slug", "use 3 to 30 letters, numbers and single dashes"}
	}
	if reserved[v] {
		return "", &FieldError{"slug", "that address is reserved; choose another"}
	}
	return v, nil
}

// Settings are what the owner edits.
type Settings struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Tagline      string `json:"tagline"`
	About        string `json:"about"`
	Policies     string `json:"policies"`
	LogoURL      string `json:"logo_url"`
	BannerURL    string `json:"banner_url"`
	LegalName    string `json:"legal_name"`
	BusinessType string `json:"business_type"`
	Website      string `json:"website"`
	BusinessNote string `json:"business_note"`
}

func (in *Settings) clean(photoOK func(string) bool) error {
	var err error
	if in.Slug, err = CleanSlug(in.Slug); err != nil {
		return err
	}
	trim := func(v *string, field string, min, max int) error {
		*v = strings.TrimSpace(*v)
		if n := utf8.RuneCountInString(*v); n < min || n > max {
			if min > 0 && n < min {
				return &FieldError{field, fmt.Sprintf("enter at least %d characters", min)}
			}
			return &FieldError{field, fmt.Sprintf("keep it under %d characters", max)}
		}
		return nil
	}
	for _, f := range []struct {
		v        *string
		field    string
		min, max int
	}{
		{&in.Name, "name", 2, 60}, {&in.Tagline, "tagline", 0, 120}, {&in.About, "about", 0, 4000},
		{&in.Policies, "policies", 0, 4000}, {&in.LegalName, "legal_name", 2, 120}, {&in.BusinessNote, "business_note", 0, 1000},
	} {
		if err := trim(f.v, f.field, f.min, f.max); err != nil {
			return err
		}
	}
	switch in.BusinessType {
	case "llc", "corporation", "partnership", "sole_proprietor", "other":
	default:
		return &FieldError{"business_type", "choose your business type"}
	}
	in.Website = strings.TrimSpace(in.Website)
	if in.Website != "" {
		if !strings.Contains(in.Website, "://") {
			in.Website = "https://" + in.Website
		}
		u, err := url.Parse(in.Website)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.Contains(u.Host, ".") || len(in.Website) > 200 {
			return &FieldError{"website", "enter a web address like example.com"}
		}
	}
	for _, p := range []struct {
		v     *string
		field string
	}{{&in.LogoURL, "logo_url"}, {&in.BannerURL, "banner_url"}} {
		*p.v = strings.TrimSpace(*p.v)
		if *p.v != "" && photoOK != nil && !photoOK(*p.v) {
			return &FieldError{p.field, "upload the image here first"}
		}
	}
	return nil
}

const storeCols = `st.seller_id, u.username, st.slug, st.name, st.tagline, st.about, st.policies, coalesce(st.logo_url, ''),
	coalesce(st.banner_url, ''), st.status, st.legal_name, st.business_type, st.website, st.business_note, st.review_note,
	st.last_update_at, st.created_at, st.reviewed_at,
	(SELECT count(*) FROM store_followers f WHERE f.seller_id = st.seller_id),
	(SELECT count(*) FROM listings l WHERE l.seller_id = st.seller_id AND l.status = 'active')`

func scanStore(row interface{ Scan(...any) error }) (Store, error) {
	var st Store
	var upd, rev sql.NullTime
	err := row.Scan(&st.SellerID, &st.Username, &st.Slug, &st.Name, &st.Tagline, &st.About, &st.Policies, &st.LogoURL,
		&st.BannerURL, &st.Status, &st.LegalName, &st.BusinessType, &st.Website, &st.BusinessNote, &st.ReviewNote,
		&upd, &st.CreatedAt, &rev, &st.Followers, &st.Listings)
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	if upd.Valid {
		st.LastUpdateAt = &upd.Time
	}
	if rev.Valid {
		st.ReviewedAt = &rev.Time
	}
	st.Categories = []Category{}
	return st, err
}

// Mine is the seller's own store (any status).
func (s *Service) Mine(ctx context.Context, sellerID string) (Store, error) {
	st, err := scanStore(s.DB.QueryRowContext(ctx, `SELECT `+storeCols+` FROM stores st JOIN users u ON u.id = st.seller_id
		WHERE st.seller_id = $1`, sellerID))
	if err != nil {
		return st, err
	}
	return st, s.fill(ctx, &st, "")
}

// BySlug is an open store's public page. viewerID marks whether they follow it.
func (s *Service) BySlug(ctx context.Context, slug, viewerID string) (Store, error) {
	st, err := scanStore(s.DB.QueryRowContext(ctx, `SELECT `+storeCols+` FROM stores st JOIN users u ON u.id = st.seller_id
		WHERE lower(st.slug) = lower($1) AND st.status = 'active' AND u.status = 'active'`, strings.TrimSpace(slug)))
	if err != nil {
		return st, err
	}
	return st, s.fill(ctx, &st, viewerID)
}

// ForSeller is the open store of a seller, if they have one (for linking
// from listings and profiles).
func (s *Service) ForSeller(ctx context.Context, sellerID string) (string, string, bool) {
	var slug, name string
	err := s.DB.QueryRowContext(ctx, `SELECT slug, name FROM stores WHERE seller_id = $1 AND status = 'active'`, sellerID).Scan(&slug, &name)
	return slug, name, err == nil
}

func (s *Service) fill(ctx context.Context, st *Store, viewerID string) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.name,
		(SELECT count(*) FROM listings l WHERE l.store_category_id = c.id AND l.status = 'active')
		FROM store_categories c WHERE c.seller_id = $1 ORDER BY c.position, c.id`, st.SellerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Listings); err != nil {
			return err
		}
		st.Categories = append(st.Categories, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if sale, err := s.current(ctx, st.SellerID); err != nil {
		return err
	} else {
		st.Sale = sale
	}
	if viewerID != "" {
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM store_followers WHERE seller_id = $1 AND user_id = $2)`,
			st.SellerID, viewerID).Scan(&st.Following); err != nil {
			return err
		}
	}
	return nil
}

// Apply opens (or updates) the seller's store. A new store waits for staff
// to check the business details; a rejected one goes back for review when
// it's sent again. Branding changes on an open store go live at once.
func (s *Service) Apply(ctx context.Context, sellerID string, in Settings, photoOK func(string) bool) (Store, error) {
	if err := in.clean(photoOK); err != nil {
		return Store{}, err
	}
	var canSell bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM seller_profiles WHERE user_id = $1 AND charges_enabled
		AND review_status <> 'rejected')`, sellerID).Scan(&canSell); err != nil {
		return Store{}, err
	}
	if !canSell {
		return Store{}, ErrNotSeller
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO stores (seller_id, slug, name, tagline, about, policies, logo_url, banner_url, legal_name, business_type, website, business_note)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12)
		ON CONFLICT (seller_id) DO UPDATE SET slug = EXCLUDED.slug, name = EXCLUDED.name, tagline = EXCLUDED.tagline,
		  about = EXCLUDED.about, policies = EXCLUDED.policies, logo_url = EXCLUDED.logo_url, banner_url = EXCLUDED.banner_url,
		  legal_name = EXCLUDED.legal_name, business_type = EXCLUDED.business_type, website = EXCLUDED.website,
		  business_note = EXCLUDED.business_note,
		  status = CASE WHEN stores.status = 'rejected' THEN 'pending' ELSE stores.status END,
		  updated_at = now()`,
		sellerID, in.Slug, in.Name, in.Tagline, in.About, in.Policies, in.LogoURL, in.BannerURL, in.LegalName, in.BusinessType, in.Website, in.BusinessNote)
	if isUnique(err) {
		return Store{}, &FieldError{"slug", ErrSlugTaken.Error()}
	}
	if err != nil {
		return Store{}, err
	}
	return s.Mine(ctx, sellerID)
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "stores_slug")
}

func (s *Service) requireActive(ctx context.Context, sellerID string) error {
	var status string
	err := s.DB.QueryRowContext(ctx, `SELECT status FROM stores WHERE seller_id = $1`, sellerID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "active") {
		return ErrNotActive
	}
	return err
}

// SetCategories replaces the store's category list, in order. Categories
// keep their ID when the name stays (by ID); missing ones are deleted and
// their listings become uncategorized.
func (s *Service) SetCategories(ctx context.Context, sellerID string, cats []Category) error {
	if err := s.requireActive(ctx, sellerID); err != nil {
		return err
	}
	if len(cats) > 50 {
		return &FieldError{"categories", "up to 50 categories"}
	}
	seen := map[string]bool{}
	for i := range cats {
		cats[i].Name = strings.TrimSpace(cats[i].Name)
		n := utf8.RuneCountInString(cats[i].Name)
		if n < 1 || n > 40 {
			return &FieldError{"categories", "each category needs a name up to 40 characters"}
		}
		k := strings.ToLower(cats[i].Name)
		if seen[k] {
			return &FieldError{"categories", "\"" + cats[i].Name + "\" is listed twice"}
		}
		seen[k] = true
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := []int{}
	for _, c := range cats {
		if c.ID > 0 {
			keep = append(keep, c.ID)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM store_categories WHERE seller_id = $1 AND NOT (id = ANY($2::int[]))`,
		sellerID, intArray(keep)); err != nil {
		return err
	}
	// Free the names first so renames that swap names don't collide.
	if _, err := tx.ExecContext(ctx, `UPDATE store_categories SET name = '~' || id::text WHERE seller_id = $1`, sellerID); err != nil {
		return err
	}
	for i, c := range cats {
		if c.ID > 0 {
			res, err := tx.ExecContext(ctx, `UPDATE store_categories SET name = $3, position = $4 WHERE id = $1 AND seller_id = $2`,
				c.ID, sellerID, c.Name, i)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 1 {
				continue
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO store_categories (seller_id, name, position) VALUES ($1, $2, $3)`,
			sellerID, c.Name, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Categorize puts the seller's listings in a store category (0 = none).
func (s *Service) Categorize(ctx context.Context, sellerID string, categoryID int, listingIDs []string) (int, error) {
	if err := s.requireActive(ctx, sellerID); err != nil {
		return 0, err
	}
	if len(listingIDs) == 0 || len(listingIDs) > 500 {
		return 0, &FieldError{"listing_ids", "choose 1 to 500 listings"}
	}
	var cat any
	if categoryID > 0 {
		var ok bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM store_categories WHERE id = $1 AND seller_id = $2)`,
			categoryID, sellerID).Scan(&ok); err != nil {
			return 0, err
		}
		if !ok {
			return 0, &FieldError{"category_id", "choose one of your store categories"}
		}
		cat = categoryID
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE listings SET store_category_id = $3, updated_at = now()
		WHERE seller_id = $1 AND id::text = ANY($2::text[]) AND status <> 'removed'`, sellerID, textArray(listingIDs), cat)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SaleInput schedules a store sale.
type SaleInput struct {
	Name       string     `json:"name"`
	Percent    int        `json:"percent"`
	CategoryID *int       `json:"category_id"`
	StartsAt   *time.Time `json:"starts_at"` // nil = now
	EndsAt     time.Time  `json:"ends_at"`
}

// CreateSale schedules a sale. One sale at a time: it may not overlap
// another that isn't cancelled.
func (s *Service) CreateSale(ctx context.Context, sellerID string, in SaleInput) (Sale, error) {
	if err := s.requireActive(ctx, sellerID); err != nil {
		return Sale{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 2 || n > 60 {
		return Sale{}, &FieldError{"name", "name the sale (2 to 60 characters)"}
	}
	if in.Percent < 5 || in.Percent > 50 {
		return Sale{}, &FieldError{"percent", "choose 5% to 50% off"}
	}
	now := s.now()
	start := now
	if in.StartsAt != nil && in.StartsAt.After(now) {
		start = *in.StartsAt
	}
	if start.After(now.AddDate(0, 3, 0)) {
		return Sale{}, &FieldError{"starts_at", "start within the next 3 months"}
	}
	if !in.EndsAt.After(start.Add(time.Hour)) {
		return Sale{}, &FieldError{"ends_at", "end at least an hour after it starts"}
	}
	if in.EndsAt.After(start.AddDate(0, 0, 31)) {
		return Sale{}, &FieldError{"ends_at", "a sale can run up to 31 days"}
	}
	if in.CategoryID != nil && *in.CategoryID > 0 {
		var ok bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM store_categories WHERE id = $1 AND seller_id = $2)`,
			*in.CategoryID, sellerID).Scan(&ok); err != nil {
			return Sale{}, err
		}
		if !ok {
			return Sale{}, &FieldError{"category_id", "choose one of your store categories, or the whole store"}
		}
	} else {
		in.CategoryID = nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Sale{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM stores WHERE seller_id = $1 FOR UPDATE`, sellerID); err != nil {
		return Sale{}, err
	}
	var clash bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM store_sales WHERE seller_id = $1 AND cancelled_at IS NULL
		AND starts_at < $3 AND ends_at > $2)`, sellerID, start, in.EndsAt).Scan(&clash); err != nil {
		return Sale{}, err
	}
	if clash {
		return Sale{}, ErrSaleClash
	}
	var sale Sale
	if err := tx.QueryRowContext(ctx, `INSERT INTO store_sales (seller_id, name, percent, category_id, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, sellerID, in.Name, in.Percent, in.CategoryID, start, in.EndsAt).Scan(&sale.ID); err != nil {
		return Sale{}, err
	}
	if err := tx.Commit(); err != nil {
		return Sale{}, err
	}
	sale.Name, sale.Percent, sale.CategoryID, sale.StartsAt, sale.EndsAt = in.Name, in.Percent, in.CategoryID, start, in.EndsAt
	return sale, nil
}

// EndSale cancels a scheduled sale or ends a running one now.
func (s *Service) EndSale(ctx context.Context, sellerID string, id int) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE store_sales SET cancelled_at = now() WHERE id = $1 AND seller_id = $2
		AND cancelled_at IS NULL AND ends_at > now()`, id, sellerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const saleCols = `ss.id, ss.name, ss.percent, ss.category_id, coalesce(c.name, ''), ss.starts_at, ss.ends_at, ss.cancelled_at`

func scanSale(row interface{ Scan(...any) error }) (Sale, error) {
	var x Sale
	var cat sql.NullInt64
	var canc sql.NullTime
	err := row.Scan(&x.ID, &x.Name, &x.Percent, &cat, &x.Category, &x.StartsAt, &x.EndsAt, &canc)
	if cat.Valid {
		v := int(cat.Int64)
		x.CategoryID = &v
	}
	if canc.Valid {
		x.Cancelled = &canc.Time
	}
	return x, err
}

func (s *Service) current(ctx context.Context, sellerID string) (*Sale, error) {
	x, err := scanSale(s.DB.QueryRowContext(ctx, `SELECT `+saleCols+` FROM store_sales ss
		LEFT JOIN store_categories c ON c.id = ss.category_id
		WHERE ss.seller_id = $1 AND ss.cancelled_at IS NULL AND now() >= ss.starts_at AND now() < ss.ends_at
		ORDER BY ss.percent DESC LIMIT 1`, sellerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &x, nil
}

// Sales lists the store's sales: running and scheduled first, then the last 20 past ones.
func (s *Service) Sales(ctx context.Context, sellerID string) ([]Sale, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+saleCols+` FROM store_sales ss
		LEFT JOIN store_categories c ON c.id = ss.category_id
		WHERE ss.seller_id = $1 ORDER BY (ss.cancelled_at IS NULL AND ss.ends_at > now()) DESC, ss.starts_at DESC LIMIT 40`, sellerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Sale{}
	for rows.Next() {
		x, err := scanSale(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Follow adds (on) or removes a follower.
func (s *Service) Follow(ctx context.Context, userID, slug string, on bool) (Store, error) {
	st, err := s.BySlug(ctx, slug, userID)
	if err != nil {
		return st, err
	}
	if st.SellerID == userID {
		return st, ErrOwnStore
	}
	if on {
		_, err = s.DB.ExecContext(ctx, `INSERT INTO store_followers (seller_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, st.SellerID, userID)
	} else {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM store_followers WHERE seller_id = $1 AND user_id = $2`, st.SellerID, userID)
	}
	if err != nil {
		return st, err
	}
	return s.BySlug(ctx, slug, userID)
}

// Following lists the stores a member follows.
func (s *Service) Following(ctx context.Context, userID string) ([]Store, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+storeCols+` FROM store_followers f JOIN stores st ON st.seller_id = f.seller_id
		JOIN users u ON u.id = st.seller_id WHERE f.user_id = $1 AND st.status = 'active' ORDER BY st.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Store{}
	for rows.Next() {
		st, err := scanStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st.Public())
	}
	return out, rows.Err()
}

// SendUpdate sends followers a short update: a bell notice for everyone,
// and an email to those who get announcement emails. Once every 7 days.
func (s *Service) SendUpdate(ctx context.Context, sellerID, subject, body string) (recipients, emailed int, err error) {
	subject, body = strings.TrimSpace(subject), strings.TrimSpace(body)
	if n := utf8.RuneCountInString(subject); n < 3 || n > 100 {
		return 0, 0, &FieldError{"subject", "a subject of 3 to 100 characters"}
	}
	if n := utf8.RuneCountInString(body); n < 10 || n > 2000 {
		return 0, 0, &FieldError{"body", "write 10 to 2,000 characters"}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var status, slug, name string
	var last sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT status, slug, name, last_update_at FROM stores WHERE seller_id = $1 FOR UPDATE`, sellerID).
		Scan(&status, &slug, &name, &last)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "active") {
		return 0, 0, ErrNotActive
	}
	if err != nil {
		return 0, 0, err
	}
	if last.Valid && s.now().Sub(last.Time) < 7*24*time.Hour {
		return 0, 0, ErrTooSoon
	}
	rows, err := tx.QueryContext(ctx, `SELECT f.user_id, NOT u.email_opt_out AND (u.email_verified_at IS NOT NULL OR NOT u.verify_required)
		FROM store_followers f JOIN users u ON u.id = f.user_id WHERE f.seller_id = $1 AND u.status = 'active'`, sellerID)
	if err != nil {
		return 0, 0, err
	}
	type follower struct {
		id    string
		email bool
	}
	var fs []follower
	for rows.Next() {
		var f follower
		if err := rows.Scan(&f.id, &f.email); err != nil {
			rows.Close()
			return 0, 0, err
		}
		fs = append(fs, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	link := "/store/" + slug
	for _, f := range fs {
		if err := inbox.Notify(ctx, tx, f.id, "store_update", name+": "+subject, trimTo(body, 280), link); err != nil {
			return 0, 0, err
		}
		if f.email && s.Email != nil {
			if err := s.Email(ctx, tx, f.id, name+": "+subject, body+"\n\nVisit the store: "+link); err != nil {
				return 0, 0, err
			}
			emailed++
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO store_updates (seller_id, subject, body, recipients, emailed) VALUES ($1, $2, $3, $4, $5)`,
		sellerID, subject, body, len(fs), emailed); err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE stores SET last_update_at = $2 WHERE seller_id = $1`, sellerID, s.now()); err != nil {
		return 0, 0, err
	}
	return len(fs), emailed, tx.Commit()
}

// NotifyNewListings tells followers when a store lists new cards: one bell
// notice per store per run (run hourly). Returns how many notices were sent.
func (s *Service) NotifyNewListings(ctx context.Context) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT st.seller_id, st.slug, st.name, count(l.id), max(l.created_at)
		FROM stores st JOIN listings l ON l.seller_id = st.seller_id AND l.status = 'active' AND l.created_at > st.last_notified_at
		WHERE st.status = 'active' AND EXISTS (SELECT 1 FROM store_followers f WHERE f.seller_id = st.seller_id)
		GROUP BY st.seller_id, st.slug, st.name`)
	if err != nil {
		return 0, err
	}
	type batch struct {
		seller, slug, name string
		n                  int
		upTo               time.Time
	}
	var bs []batch
	for rows.Next() {
		var b batch
		if err := rows.Scan(&b.seller, &b.slug, &b.name, &b.n, &b.upTo); err != nil {
			rows.Close()
			return 0, err
		}
		bs = append(bs, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	sent := 0
	for _, b := range bs {
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return sent, err
		}
		cards := "cards"
		if b.n == 1 {
			cards = "card"
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO notifications (user_id, kind, title, body, link)
			SELECT f.user_id, 'store_new_listings', $2, $3, $4 FROM store_followers f JOIN users u ON u.id = f.user_id
			WHERE f.seller_id = $1 AND u.status = 'active'`,
			b.seller, b.name+" listed new cards", fmt.Sprintf("%d new %s in %s.", b.n, cards, b.name), "/store/"+b.slug+"?sort=new")
		if err != nil {
			tx.Rollback()
			return sent, err
		}
		n, _ := res.RowsAffected()
		if _, err := tx.ExecContext(ctx, `UPDATE stores SET last_notified_at = $2 WHERE seller_id = $1`, b.seller, b.upTo); err != nil {
			tx.Rollback()
			return sent, err
		}
		if err := tx.Commit(); err != nil {
			return sent, err
		}
		sent += int(n)
	}
	return sent, nil
}

// List is the back office's store list (pending first).
func (s *Service) List(ctx context.Context, status string) ([]Store, error) {
	q := `SELECT ` + storeCols + ` FROM stores st JOIN users u ON u.id = st.seller_id`
	args := []any{}
	if status != "" {
		q += ` WHERE st.status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY (st.status = 'pending') DESC, st.created_at DESC LIMIT 200`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Store{}
	for rows.Next() {
		st, err := scanStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Review approves, rejects, suspends or reopens a store, and tells the seller.
func (s *Service) Review(ctx context.Context, tx *sql.Tx, sellerID, status, note, staffID string) error {
	switch status {
	case "active", "rejected", "suspended":
	default:
		return &FieldError{"status", "choose approve, reject or suspend"}
	}
	note = strings.TrimSpace(note)
	if status != "active" && note == "" {
		return &FieldError{"note", "tell the seller why"}
	}
	var name, slug string
	err := tx.QueryRowContext(ctx, `UPDATE stores SET status = $2, review_note = $3, reviewed_by = $4, reviewed_at = now(), updated_at = now(),
		last_notified_at = CASE WHEN $2 = 'active' AND status <> 'active' THEN now() ELSE last_notified_at END
		WHERE seller_id = $1 RETURNING name, slug`, sellerID, status, note, staffID).Scan(&name, &slug)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	title, body, link := "Your store is open", name+" is live at slabsnsingles.com/store/"+slug+".", "/store/"+slug
	switch status {
	case "rejected":
		title, body, link = "Your store application needs changes", note, "/sell/store"
	case "suspended":
		title, body, link = "Your store is paused", note, "/sell/store"
	}
	return inbox.Notify(ctx, tx, sellerID, "store_review", title, body, link)
}

func trimTo(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func intArray(v []int) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprint(x)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func textArray(v []string) string {
	parts := make([]string, 0, len(v))
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x == "" || strings.ContainsAny(x, `{},"\`) {
			continue
		}
		parts = append(parts, x)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
