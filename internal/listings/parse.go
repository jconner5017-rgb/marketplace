package listings

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// ParseSearch reads search filters from a query string (the same one the
// website's /search page uses). bad lists malformed filters by name.
// Limit and Offset are left for the caller.
func ParseSearch(q url.Values) (p SearchParams, bad map[string]string) {
	// List filters accept repeated params (?grader=PSA&grader=BGS) or commas.
	list := func(key string) []string {
		out := []string{}
		for _, v := range q[key] {
			for _, part := range strings.Split(v, ",") {
				if part = strings.TrimSpace(part); part != "" {
					out = append(out, part)
				}
			}
		}
		return out
	}
	p = SearchParams{
		Query:      q.Get("q"),
		QueryMode:  q.Get("mode"),
		Exclude:    q.Get("exclude"),
		TitleOnly:  q.Get("in") == "title",
		Category:   q.Get("category"),
		Games:      list("game"),
		SetName:    q.Get("set"),
		CardNumber: q.Get("number"),
		Graders:    list("grader"),
		CertNumber: q.Get("cert"),
		Conditions: list("condition"),
		Attributes: list("attribute"),
		Seller:     q.Get("seller"),
		Discount:   q.Get("discount") == "1" || q.Get("discount") == "true",
		Sort:       q.Get("sort"),
		Format:     q.Get("format"),
	}
	bad = map[string]string{}
	if len(p.Query) > 200 || len(p.Exclude) > 200 {
		bad["q"] = "search text is too long"
	}
	if v := q.Get("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil {
			bad["year"] = "must be a year"
		}
		p.Year = y
	}
	for key, dst := range map[string]*int{"year_min": &p.YearMin, "year_max": &p.YearMax} {
		if v := q.Get(key); v != "" {
			y, err := strconv.Atoi(v)
			if err != nil {
				bad[key] = "must be a year"
			}
			*dst = y
		}
	}
	switch p.QueryMode {
	case "", "all", "any", "exact":
	default:
		bad["mode"] = "all, any or exact"
	}
	if v := q.Get("min_grade"); v != "" {
		g, err := strconv.ParseFloat(v, 64)
		if err != nil {
			bad["min_grade"] = "must be a number"
		}
		p.MinGrade = g
	}
	for key, dst := range map[string]*int{"min_price": &p.MinCents, "max_price": &p.MaxCents} {
		if v := q.Get(key); v != "" {
			c, err := DollarsToCents(v)
			if err != nil {
				bad[key] = "must be a dollar amount"
			}
			*dst = c
		}
	}
	switch p.Sort {
	case "", "newest", "price_asc", "price_desc", "relevance", "ending":
	default:
		bad["sort"] = "newest, price_asc, price_desc, relevance or ending"
	}
	switch p.Format {
	case "", "auction", "fixed", "offers":
	default:
		bad["format"] = "auction, fixed or offers"
	}
	return p, bad
}

// DollarsToCents reads "12.50" or "$12.50".
func DollarsToCents(v string) (int, error) {
	f, err := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(v), "$"), 64)
	if err != nil || f < 0 || f > 1_000_000 {
		return 0, errors.New("bad amount")
	}
	return int(f*100 + 0.5), nil
}
