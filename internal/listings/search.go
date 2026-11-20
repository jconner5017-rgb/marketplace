package listings

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

// SearchParams are the buyer-facing filters. Zero values mean "any".
// List filters (Games, Graders, Conditions) match any of their values;
// Attributes must all be present.
type SearchParams struct {
	Query      string
	QueryMode  string // all (default): every word; any: at least one word; exact: the exact phrase
	Exclude    string // words that must not appear
	TitleOnly  bool   // match keywords against the title only, not the description
	Category   string // sports | tcg
	Games      []string
	SetName    string
	Year       int
	YearMin    int
	YearMax    int
	CardNumber string
	Graders    []string // PSA, BGS, ... or "RAW" for ungraded
	MinGrade   float64
	CertNumber string
	Conditions []string
	Attributes []string
	Seller     string // username
	// StoreCategory keeps one of the seller's store categories (with Seller).
	StoreCategory int
	OnSale        bool // in a store sale right now
	MinCents      int
	MaxCents      int
	Discount      bool   // seller offers a multi-card discount
	Lots          string // "only" keeps lots, "none" hides them; empty shows both
	Format        string // auction, fixed (Buy It Now) or offers (accepts offers)
	Sort          string // newest (default), price_asc, price_desc, relevance
	// ListedAfter keeps cards listed after this time (saved-search alerts).
	ListedAfter time.Time
	// ExactTotal counts every match (sitemap, feeds); otherwise the count
	// stops at TotalCap and TotalCapped says there are more.
	ExactTotal bool
	Limit      int
	Offset     int
}

// tsQueryText turns keywords, mode and exclusions into websearch syntax.
func tsQueryText(p SearchParams) string {
	q := strings.TrimSpace(p.Query)
	words := strings.Fields(strings.ReplaceAll(q, `"`, " "))
	switch p.QueryMode {
	case "any":
		q = strings.Join(words, " or ")
	case "exact":
		if len(words) > 0 {
			q = `"` + strings.Join(words, " ") + `"`
		}
	}
	for _, w := range strings.Fields(strings.ReplaceAll(p.Exclude, `"`, " ")) {
		w = strings.TrimLeft(w, "-")
		if w != "" {
			q += " -" + w
		}
	}
	return strings.TrimSpace(q)
}

// TotalCap is where a search stops counting matches: "10,000+ results" is as
// useful to a buyer as an exact figure, and counting stays fast as the
// catalog grows.
var TotalCap = 10000

type SearchResult struct {
	Listings []Listing `json:"listings"`
	Total    int       `json:"total"`
	// TotalCapped: there are more than Total matches.
	TotalCapped bool `json:"total_capped,omitempty"`
	Limit       int  `json:"limit"`
	Offset      int  `json:"offset"`
}

// Search runs a buyer search over public listings.
func (s *Service) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 48
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	f := buildFilter(p)
	args, arg, filter, tsq := f.args, f.arg, f.sql, f.tsq

	order := "l.created_at DESC, l.id"
	switch p.Sort {
	case "price_asc":
		order = "l.price_cents ASC, l.created_at DESC, l.id"
	case "price_desc":
		order = "l.price_cents DESC, l.created_at DESC, l.id"
	case "ending":
		order = "(l.format = 'auction') DESC, l.ends_at ASC NULLS LAST, l.created_at DESC, l.id"
	case "random":
		// Home page shelves: no card gets a fixed spot.
		order = "random()"
	case "relevance":
		if tsq != "" {
			order = "ts_rank(l.search, " + tsq + ") DESC, l.created_at DESC, l.id"
		}
	}

	if p.Sort == "random" {
		return s.randomPick(ctx, p, filter, args)
	}

	var total int
	countSQL := "SELECT count(*) FROM listings l " + publicJoin + filter
	if !p.ExactTotal {
		countSQL = fmt.Sprintf("SELECT count(*) FROM (SELECT 1 FROM listings l %s%s LIMIT %d) x", publicJoin, filter, TotalCap+1)
	}
	if err := s.DB.QueryRowContext(ctx, countSQL, *args...).Scan(&total); err != nil {
		return SearchResult{}, err
	}
	capped := false
	if !p.ExactTotal && total > TotalCap {
		total, capped = TotalCap, true
	}

	limitArg := arg(p.Limit)
	offsetArg := arg(p.Offset)
	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+listingCols+" FROM listings l "+publicJoin+filter+
			" ORDER BY "+order+" LIMIT "+limitArg+" OFFSET "+offsetArg, *args...)
	if err != nil {
		return SearchResult{}, err
	}
	defer rows.Close()
	res := SearchResult{Listings: []Listing{}, Total: total, TotalCapped: capped, Limit: p.Limit, Offset: p.Offset}
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return SearchResult{}, err
		}
		l.ExternalID = nil     // internal to the seller
		l.DescriptionHTML = "" // results show tiles; the full page loads it
		res.Listings = append(res.Listings, l)
	}
	return res, rows.Err()
}

// filter is a WHERE fragment plus its arguments.
type filter struct {
	args *[]any
	arg  func(any) string
	sql  string // " AND ..." or ""
	tsq  string // the tsquery expression, if any
}

// buildFilter turns search params into SQL. skip names one dimension to
// leave out (used for facet counts, so picking "PSA" still shows how
// many BGS cards there are).
func buildFilter(p SearchParams, skip ...string) filter {
	var where []string
	args := &[]any{}
	arg := func(v any) string {
		*args = append(*args, v)
		return fmt.Sprintf("$%d", len(*args))
	}
	skipping := func(dim string) bool {
		for _, s := range skip {
			if s == dim {
				return true
			}
		}
		return false
	}

	tsq := ""
	if q := tsQueryText(p); q != "" {
		tsq = "websearch_to_tsquery('simple', " + arg(q) + ")"
		target := "l.search"
		if p.TitleOnly {
			target = "to_tsvector('simple', l.title)"
		}
		where = append(where, target+" @@ "+tsq)
	}
	upper := func(xs []string) []string {
		out := []string{}
		for _, x := range xs {
			if x = strings.ToUpper(strings.TrimSpace(x)); x != "" {
				out = append(out, x)
			}
		}
		return out
	}
	if p.Category != "" && !skipping("category") {
		where = append(where, "l.category = "+arg(strings.ToLower(p.Category)))
	}
	if games := upper(p.Games); len(games) > 0 && !skipping("game") {
		where = append(where, "upper(l.game_or_sport) = ANY("+arg(pq.Array(games))+"::text[])")
	}
	if p.SetName != "" {
		where = append(where, "lower(l.set_name) = lower("+arg(p.SetName)+")")
	}
	if p.Year != 0 {
		where = append(where, "l.year = "+arg(p.Year))
	}
	if p.YearMin != 0 {
		where = append(where, "l.year >= "+arg(p.YearMin))
	}
	if p.YearMax != 0 {
		where = append(where, "l.year <= "+arg(p.YearMax))
	}
	if p.CardNumber != "" {
		where = append(where, "lower(l.card_number) = lower("+arg(strings.TrimPrefix(strings.TrimSpace(p.CardNumber), "#"))+")")
	}
	if gs := upper(p.Graders); len(gs) > 0 && !skipping("grader") {
		raw, named := false, []string{}
		for _, g := range gs {
			if g == "RAW" {
				raw = true
			} else {
				named = append(named, g)
			}
		}
		parts := []string{}
		if raw {
			parts = append(parts, "l.grader IS NULL")
		}
		if len(named) > 0 {
			parts = append(parts, "l.grader = ANY("+arg(pq.Array(named))+"::text[])")
		}
		where = append(where, "("+strings.Join(parts, " OR ")+")")
	}
	if p.MinGrade > 0 {
		where = append(where, "l.grade >= "+arg(p.MinGrade))
	}
	if p.CertNumber != "" {
		where = append(where, "l.cert_number = "+arg(strings.TrimSpace(p.CertNumber)))
	}
	if cs := upper(p.Conditions); len(cs) > 0 && !skipping("condition") {
		where = append(where, "l.condition = ANY("+arg(pq.Array(cs))+"::text[])")
	}
	if len(p.Attributes) > 0 && !skipping("attribute") {
		attrs := []string{}
		for _, a := range p.Attributes {
			if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
				attrs = append(attrs, a)
			}
		}
		if len(attrs) > 0 {
			where = append(where, "l.attributes @> "+arg(pq.Array(attrs))+"::text[]")
		}
	}
	if !skipping("format") {
		switch p.Format {
		case FormatAuction, FormatFixed:
			where = append(where, "l.format = "+arg(p.Format))
		case "offers":
			where = append(where, "l.offers_enabled")
		}
	}
	if p.Discount {
		where = append(where, "EXISTS (SELECT 1 FROM order_discounts od WHERE od.seller_id = l.seller_id AND od.active)")
	}
	switch p.Lots {
	case "only":
		where = append(where, "l.lot_count IS NOT NULL")
	case "none":
		where = append(where, "l.lot_count IS NULL")
	}
	if p.Seller != "" {
		where = append(where, "lower(u.username) = lower("+arg(p.Seller)+")")
	}
	if p.StoreCategory > 0 {
		where = append(where, "l.store_category_id = "+arg(p.StoreCategory))
	}
	if p.OnSale {
		where = append(where, "l.format = 'fixed' AND store_sale_price(l.seller_id, l.store_category_id, l.game_or_sport, l.price_cents) < l.price_cents")
	}
	if p.MinCents > 0 {
		where = append(where, "l.price_cents >= "+arg(p.MinCents))
	}
	if p.MaxCents > 0 {
		where = append(where, "l.price_cents <= "+arg(p.MaxCents))
	}
	if !p.ListedAfter.IsZero() {
		where = append(where, "l.created_at > "+arg(p.ListedAfter))
	}
	sql := ""
	if len(where) > 0 {
		sql = " AND " + strings.Join(where, " AND ")
	}
	return filter{args: args, arg: arg, sql: sql, tsq: tsq}
}

// Facet is one value in a sidebar filter with its result count.
type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type Facets struct {
	Category  []Facet `json:"category"`
	Game      []Facet `json:"game"`
	Grader    []Facet `json:"grader"` // includes "RAW"
	Condition []Facet `json:"condition"`
	Attribute []Facet `json:"attribute"`
	Format    []Facet `json:"format"`
}

// Facets counts matching listings per filter value, eBay-sidebar style.
// Each dimension ignores its own current selection.
func (s *Service) Facets(ctx context.Context, p SearchParams) (Facets, error) {
	// Browse pages are asked for the same counts over and over (buyers and
	// crawlers), so they're kept for facetTTL.
	q := p
	q.Limit, q.Offset, q.Sort, q.ExactTotal = 0, 0, "", false
	key := fmt.Sprintf("%+v", q)
	s.facetCache.mu.Lock()
	if c, ok := s.facetCache.m[key]; ok && time.Since(c.at) < facetTTL {
		s.facetCache.mu.Unlock()
		return c.f, nil
	}
	s.facetCache.mu.Unlock()

	var out Facets
	dims := []facetDim{
		{"category", "l.category", &out.Category},
		{"game", "l.game_or_sport", &out.Game},
		{"grader", "coalesce(l.grader, 'RAW')", &out.Grader},
		{"condition", "l.condition", &out.Condition},
		{"format", "l.format", &out.Format},
	}
	// Each dimension ignores its own selection, so dimensions with no
	// selection share the same filter: those are counted in one pass over
	// the listings instead of one pass each. Attributes (several per card)
	// get their own pass.
	type group struct {
		f    filter
		dims []facetDim
	}
	var groups []*group
	byFilter := map[string]*group{}
	for _, d := range dims {
		f := buildFilter(p, d.name)
		k := f.sql + fmt.Sprint(*f.args)
		g := byFilter[k]
		if g == nil {
			g = &group{f: f}
			byFilter[k] = g
			groups = append(groups, g)
		}
		g.dims = append(g.dims, d)
	}
	errs := make([]error, len(groups)+1)
	var wg sync.WaitGroup
	for i, g := range groups {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.facetGroup(ctx, g.f, g.dims)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		out.Attribute, errs[len(groups)] = s.facet(ctx, p, "attribute", "unnest(l.attributes)")
	}()
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return Facets{}, err
	}
	s.facetCache.mu.Lock()
	if s.facetCache.m == nil || len(s.facetCache.m) > 1000 {
		s.facetCache.m = map[string]facetEntry{}
	}
	s.facetCache.m[key] = facetEntry{at: time.Now(), f: out}
	s.facetCache.mu.Unlock()
	return out, nil
}

const facetTTL = time.Minute

type facetDim struct {
	name string
	expr string
	dst  *[]Facet
}

type facetEntry struct {
	at time.Time
	f  Facets
}

type facetCache struct {
	mu sync.Mutex
	m  map[string]facetEntry
}

// facetGroup counts several dimensions that share one filter in a single
// pass: each listing's values are unpivoted into (dimension, value) rows.
func (s *Service) facetGroup(ctx context.Context, f filter, dims []facetDim) error {
	if len(dims) == 1 {
		var err error
		*dims[0].dst, err = s.facetWith(ctx, f, dims[0].expr)
		return err
	}
	cols := make([]string, len(dims))
	vals := make([]string, len(dims))
	for i, d := range dims {
		cols[i] = fmt.Sprintf("%s AS v%d", d.expr, i)
		vals[i] = fmt.Sprintf("(%d, x.v%d)", i, i)
	}
	q := "SELECT u.d, u.v, count(*) FROM (SELECT " + strings.Join(cols, ", ") + " FROM listings l " + publicJoin + f.sql + ") x" +
		" CROSS JOIN LATERAL (VALUES " + strings.Join(vals, ", ") + ") AS u(d, v) WHERE u.v IS NOT NULL GROUP BY u.d, u.v"
	rows, err := s.DB.QueryContext(ctx, q, *f.args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	lists := make([][]Facet, len(dims))
	for rows.Next() {
		var d int
		var fc Facet
		if err := rows.Scan(&d, &fc.Value, &fc.Count); err != nil {
			return err
		}
		lists[d] = append(lists[d], fc)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i, d := range dims {
		l := lists[i]
		sort.Slice(l, func(a, b int) bool {
			if l[a].Count != l[b].Count {
				return l[a].Count > l[b].Count
			}
			return l[a].Value < l[b].Value
		})
		if len(l) > 30 {
			l = l[:30]
		}
		if l == nil {
			l = []Facet{}
		}
		*d.dst = l
	}
	return nil
}

func (s *Service) facet(ctx context.Context, p SearchParams, name, expr string) ([]Facet, error) {
	return s.facetWith(ctx, buildFilter(p, name), expr)
}

func (s *Service) facetWith(ctx context.Context, f filter, expr string) ([]Facet, error) {
	inner := "SELECT " + expr + " AS v FROM listings l " + publicJoin + f.sql
	rows, err := s.DB.QueryContext(ctx,
		"SELECT v, count(*) FROM ("+inner+") x WHERE v IS NOT NULL GROUP BY v ORDER BY count(*) DESC, v LIMIT 30",
		*f.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Facet{}
	for rows.Next() {
		var fc Facet
		if err := rows.Scan(&fc.Value, &fc.Count); err != nil {
			return nil, err
		}
		list = append(list, fc)
	}
	return list, rows.Err()
}

// Random shelves (the home page) would sort every matching listing on each
// visit. Instead a pool of up to randomPoolSize random matches is kept per
// filter for randomPoolTTL, and each visit draws its own pick from it, so
// visitors still see different cards.
const (
	randomPoolSize = 150
	randomPoolTTL  = 2 * time.Minute
)

type randomPools struct {
	mu sync.Mutex
	m  map[string]randomPool
}

type randomPool struct {
	at    time.Time
	items []Listing
	total int
}

func (s *Service) randomPick(ctx context.Context, p SearchParams, filter string, args *[]any) (SearchResult, error) {
	key := filter + fmt.Sprint(*args)
	s.pools.mu.Lock()
	pool, ok := s.pools.m[key]
	s.pools.mu.Unlock()
	if !ok || time.Since(pool.at) > randomPoolTTL {
		var err error
		if pool, err = s.loadRandomPool(ctx, filter, args); err != nil {
			return SearchResult{}, err
		}
		s.pools.mu.Lock()
		if s.pools.m == nil || len(s.pools.m) > 500 {
			s.pools.m = map[string]randomPool{}
		}
		s.pools.m[key] = pool
		s.pools.mu.Unlock()
	}
	idx := rand.Perm(len(pool.items))
	if len(idx) > p.Limit {
		idx = idx[:p.Limit]
	}
	res := SearchResult{Listings: make([]Listing, 0, len(idx)), Total: pool.total, Limit: p.Limit}
	for _, i := range idx {
		res.Listings = append(res.Listings, pool.items[i])
	}
	return res, nil
}

func (s *Service) loadRandomPool(ctx context.Context, filter string, args *[]any) (randomPool, error) {
	pool := randomPool{at: time.Now()}
	if err := s.DB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT count(*) FROM (SELECT 1 FROM listings l %s%s LIMIT %d) x", publicJoin, filter, TotalCap), *args...).Scan(&pool.total); err != nil {
		return pool, err
	}
	// Pick ids first (cheap), then load only those listings.
	rows, err := s.DB.QueryContext(ctx, fmt.Sprintf(
		"SELECT "+listingCols+" FROM listings l "+publicJoin+" AND l.id IN (SELECT l.id FROM listings l %s%s ORDER BY random() LIMIT %d)",
		publicJoin, filter, randomPoolSize), *args...)
	if err != nil {
		return pool, err
	}
	defer rows.Close()
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return pool, err
		}
		l.ExternalID = nil
		l.DescriptionHTML = ""
		pool.items = append(pool.items, l)
	}
	return pool, rows.Err()
}
