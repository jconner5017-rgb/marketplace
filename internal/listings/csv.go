package listings

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CSVColumns are the columns the importer understands. Only the header
// names matter, not their order; unknown columns are an error so typos
// don't silently drop data.
var CSVColumns = []string{
	"external_id", "category", "game_or_sport", "set_name", "year", "card_number", "title",
	"description", "condition", "grader", "grade", "cert_number", "attributes", "photo_urls",
	"price", "quantity", "package_preset", "weight_oz", "status",
}

const MaxCSVRows = 5000

// RowResult reports what happened to one CSV row (row 2 = first data row,
// matching what a spreadsheet shows).
type RowResult struct {
	Row       int               `json:"row"`
	Status    string            `json:"status"` // created, updated, error
	ListingID string            `json:"listing_id,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
}

type ImportResult struct {
	Created int         `json:"created"`
	Updated int         `json:"updated"`
	Failed  int         `json:"failed"`
	Rows    []RowResult `json:"rows"`
}

// ImportError is a problem with the file as a whole (not one row).
type ImportError struct{ Msg string }

func (e *ImportError) Error() string { return e.Msg }

// ImportCSV creates or updates listings from a CSV file. Every row must
// have an external_id (the seller's own SKU), so re-uploading the same
// file updates listings instead of duplicating them. Good rows are saved
// even when other rows have errors.
func (s *Service) ImportCSV(ctx context.Context, sellerID string, r io.Reader) (ImportResult, error) {
	prof, err := s.sellerFor(ctx, sellerID)
	if err != nil {
		return ImportResult{}, err
	}

	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1 // checked per row below, with a friendlier message

	header, err := cr.Read()
	if err == io.EOF {
		return ImportResult{}, &ImportError{"the file is empty"}
	}
	if err != nil {
		return ImportResult{}, &ImportError{"could not read the header row: " + err.Error()}
	}
	col := map[string]int{}
	known := map[string]bool{}
	for _, c := range CSVColumns {
		known[c] = true
	}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF"))) // strip Excel's BOM
		if !known[h] {
			return ImportResult{}, &ImportError{fmt.Sprintf("unknown column %q (allowed: %s)", h, strings.Join(CSVColumns, ", "))}
		}
		if _, dup := col[h]; dup {
			return ImportResult{}, &ImportError{fmt.Sprintf("column %q appears twice", h)}
		}
		col[h] = i
	}
	for _, req := range []string{"external_id", "category", "game_or_sport", "set_name", "title", "price", "quantity", "photo_urls"} {
		if _, ok := col[req]; !ok {
			return ImportResult{}, &ImportError{"missing required column " + req}
		}
	}

	res := ImportResult{Rows: []RowResult{}}
	seen := map[string]int{}
	for rowNum := 2; ; rowNum++ {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if rowNum-1 > MaxCSVRows {
			return ImportResult{}, &ImportError{fmt.Sprintf("too many rows; the limit is %d per file", MaxCSVRows)}
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return ImportResult{}, &ImportError{fmt.Sprintf("row %d: %v", rowNum, pe.Err)}
			}
			return ImportResult{}, &ImportError{err.Error()}
		}
		if len(rec) != len(header) {
			res.add(RowResult{Row: rowNum, Status: "error",
				Errors: map[string]string{"row": fmt.Sprintf("has %d columns, header has %d", len(rec), len(header))}})
			continue
		}
		get := func(name string) string {
			if i, ok := col[name]; ok {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}

		in, ferrs := parseRow(get)
		if in.ExternalID != nil {
			if first, dup := seen[*in.ExternalID]; dup {
				ferrs["external_id"] = fmt.Sprintf("same as row %d", first)
			} else {
				seen[*in.ExternalID] = rowNum
			}
		}
		in.normalize()
		if err := in.validate(prof.TierCapCents); err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				for k, v := range ve.Fields {
					if _, already := ferrs[k]; !already {
						ferrs[k] = v
					}
				}
			}
		}
		if len(ferrs) > 0 {
			res.add(RowResult{Row: rowNum, Status: "error", Errors: ferrs})
			continue
		}

		l, created, err := s.upsert(ctx, sellerID, in)
		if err != nil {
			if errors.Is(err, ErrDuplicateExtern) {
				res.add(RowResult{Row: rowNum, Status: "error", Errors: map[string]string{"external_id": "already in use"}})
				continue
			}
			return ImportResult{}, err
		}
		st := "updated"
		if created {
			st = "created"
		}
		res.add(RowResult{Row: rowNum, Status: st, ListingID: l.ID})
	}
	if len(res.Rows) == 0 {
		return ImportResult{}, &ImportError{"the file has a header but no rows"}
	}
	return res, nil
}

func (r *ImportResult) add(row RowResult) {
	switch row.Status {
	case "created":
		r.Created++
	case "updated":
		r.Updated++
	default:
		r.Failed++
	}
	r.Rows = append(r.Rows, row)
}

// parseRow turns CSV text into an Input. Type errors (a price that isn't
// a number) are returned per field; business rules are checked later.
func parseRow(get func(string) string) (Input, map[string]string) {
	errs := map[string]string{}
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	in := Input{
		ExternalID:    opt(get("external_id")),
		Category:      get("category"),
		GameOrSport:   get("game_or_sport"),
		SetName:       get("set_name"),
		CardNumber:    opt(get("card_number")),
		Title:         get("title"),
		Description:   get("description"),
		Condition:     opt(get("condition")),
		Grader:        opt(get("grader")),
		CertNumber:    opt(get("cert_number")),
		PackagePreset: strings.ToLower(get("package_preset")),
		Status:        strings.ToLower(get("status")),
	}
	if in.ExternalID == nil {
		errs["external_id"] = "required (your own SKU, so re-uploads update instead of duplicating)"
	}
	if v := get("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil {
			errs["year"] = "must be a whole number"
		} else {
			in.Year = &y
		}
	}
	if v := get("grade"); v != "" {
		g, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs["grade"] = "must be a number like 9 or 9.5"
		} else {
			in.Grade = &g
		}
	}
	if v := get("price"); v == "" {
		errs["price"] = "required"
	} else if c, err := parseDollars(v); err != nil {
		errs["price"] = err.Error()
	} else {
		in.PriceCents = c
	}
	if v := get("quantity"); v == "" {
		in.Quantity = 1
	} else if q, err := strconv.Atoi(v); err != nil {
		errs["quantity"] = "must be a whole number"
	} else {
		in.Quantity = q
	}
	if v := get("weight_oz"); v != "" {
		w, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs["weight_oz"] = "must be a number"
		} else {
			in.WeightOz = w
		}
	}
	if v := get("attributes"); v != "" {
		in.Attributes = strings.Split(v, "|")
	}
	if v := get("photo_urls"); v != "" {
		in.PhotoURLs = strings.Split(v, "|")
	}
	return in, errs
}

// parseDollars reads "12", "12.5", "12.50" or "$1,234.00" as cents,
// without floating-point rounding.
func parseDollars(s string) (int, error) {
	s = strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(s), "$"), ",", "")
	whole, frac, hasFrac := strings.Cut(s, ".")
	bad := fmt.Errorf("must be a dollar amount like 12.50")
	if whole == "" && !hasFrac {
		return 0, bad
	}
	if whole == "" {
		whole = "0"
	}
	if !allDigits(whole) || (hasFrac && !allDigits(frac)) {
		return 0, bad
	}
	w, err := strconv.Atoi(whole)
	if err != nil || w < 0 || w > 1_000_000 {
		return 0, bad
	}
	cents := 0
	if hasFrac {
		if len(frac) == 0 || len(frac) > 2 {
			return 0, bad
		}
		if len(frac) == 1 {
			frac += "0"
		}
		c, err := strconv.Atoi(frac)
		if err != nil || c < 0 {
			return 0, bad
		}
		cents = c
	}
	return w*100 + cents, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
