package listings

import (
	"context"
	"fmt"
	"strings"
)

// SearchParams are the buyer-facing filters. Zero values mean "any".
type SearchParams struct {
	Query       string
	Category    string // sports | tcg
	GameOrSport string
	SetName     string
	Year        int
	Grader      string // PSA, BGS, ... or "RAW" for ungraded
	MinGrade    float64
	Condition   string
	Attribute   string
	Seller      string // username
	MinCents    int
	MaxCents    int
	Sort        string // newest (default), price_asc, price_desc, relevance
	Limit       int
	Offset      int
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

	var where []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	q := strings.TrimSpace(p.Query)
	tsq := ""
	if q != "" {
		tsq = "websearch_to_tsquery('simple', " + arg(q) + ")"
		where = append(where, "l.search @@ "+tsq)
	}
	if p.Category != "" {
		where = append(where, "l.category = "+arg(strings.ToLower(p.Category)))
	}
	if p.GameOrSport != "" {
		where = append(where, "lower(l.game_or_sport) = lower("+arg(p.GameOrSport)+")")
	}
	if p.SetName != "" {
		where = append(where, "lower(l.set_name) = lower("+arg(p.SetName)+")")
	}
	if p.Year != 0 {
		where = append(where, "l.year = "+arg(p.Year))
	}
	switch g := strings.ToUpper(p.Grader); g {
	case "":
	case "RAW":
		where = append(where, "l.grader IS NULL")
	default:
		where = append(where, "l.grader = "+arg(g))
	}
	if p.MinGrade > 0 {
		where = append(where, "l.grade >= "+arg(p.MinGrade))
	}
	if p.Condition != "" {
		where = append(where, "l.condition = "+arg(strings.ToUpper(p.Condition)))
	}
	if p.Attribute != "" {
		where = append(where, arg(strings.ToLower(p.Attribute))+" = ANY(l.attributes)")
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

	filter := ""
	if len(where) > 0 {
		filter = " AND " + strings.Join(where, " AND ")
	}

	order := "l.created_at DESC, l.id"
	switch p.Sort {
	case "price_asc":
		order = "l.price_cents ASC, l.created_at DESC, l.id"
	case "price_desc":
		order = "l.price_cents DESC, l.created_at DESC, l.id"
	case "relevance":
		if tsq != "" {
			order = "ts_rank(l.search, " + tsq + ") DESC, l.created_at DESC, l.id"
		}
	}

	var total int
	if err := s.DB.QueryRowContext(ctx,
		"SELECT count(*) FROM listings l "+publicJoin+filter, args...).Scan(&total); err != nil {
		return SearchResult{}, err
	}

	limitArg := arg(p.Limit)
	offsetArg := arg(p.Offset)
	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+listingCols+" FROM listings l "+publicJoin+filter+
			" ORDER BY "+order+" LIMIT "+limitArg+" OFFSET "+offsetArg, args...)
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
