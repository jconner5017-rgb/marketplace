package listings

import (
	"context"
	"fmt"
	"strings"
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
	MinCents   int
	MaxCents   int
	Discount   bool   // seller offers a multi-card discount
	Format     string // auction, fixed (Buy It Now) or offers (accepts offers)
	Sort       string // newest (default), price_asc, price_desc, relevance
	// ListedAfter keeps cards listed after this time (saved-search alerts).
	ListedAfter time.Time
	Limit       int
	Offset      int
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

type SearchResult struct {
	Listings []Listing `json:"listings"`
	Total    int       `json:"total"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
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
	case "relevance":
		if tsq != "" {
			order = "ts_rank(l.search, " + tsq + ") DESC, l.created_at DESC, l.id"
		}
	}

	var total int
	if err := s.DB.QueryRowContext(ctx,
		"SELECT count(*) FROM listings l "+publicJoin+filter, *args...).Scan(&total); err != nil {
		return SearchResult{}, err
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
	res := SearchResult{Listings: []Listing{}, Total: total, Limit: p.Limit, Offset: p.Offset}
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return SearchResult{}, err
		}
		l.ExternalID = nil // internal to the seller
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
		where = append(where, "sp.bundle_discount_type IS NOT NULL")
	}
	if p.Seller != "" {
		where = append(where, "lower(u.username) = lower("+arg(p.Seller)+")")
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
	var out Facets
	dims := []struct {
		name string
		expr string
		dst  *[]Facet
	}{
		{"category", "l.category", &out.Category},
		{"game", "l.game_or_sport", &out.Game},
		{"grader", "coalesce(l.grader, 'RAW')", &out.Grader},
		{"condition", "l.condition", &out.Condition},
		{"attribute", "unnest(l.attributes)", &out.Attribute},
		{"format", "l.format", &out.Format},
	}
	for _, d := range dims {
		f := buildFilter(p, d.name)
		inner := "SELECT " + d.expr + " AS v FROM listings l " + publicJoin + f.sql
		rows, err := s.DB.QueryContext(ctx,
			"SELECT v, count(*) FROM ("+inner+") x WHERE v IS NOT NULL GROUP BY v ORDER BY count(*) DESC, v LIMIT 30",
			*f.args...)
		if err != nil {
			return Facets{}, err
		}
		list := []Facet{}
		for rows.Next() {
			var fc Facet
			if err := rows.Scan(&fc.Value, &fc.Count); err != nil {
				rows.Close()
				return Facets{}, err
			}
			list = append(list, fc)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return Facets{}, err
		}
		*d.dst = list
	}
	return out, nil
}
