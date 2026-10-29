package shopping

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

const (
	// MaxSavedSearches caps how many searches one member can save.
	MaxSavedSearches = 50
	// alertGap is the least time between two alerts for one saved search.
	alertGap = 6 * time.Hour
)

var (
	ErrSavedFull    = errors.New("you can save up to 50 searches; delete one first")
	ErrSavedExists  = errors.New("you've already saved this search")
	ErrSavedEmpty   = errors.New("add a keyword or a filter before saving a search")
	ErrSavedBad     = errors.New("that search has a filter we can't read")
	ErrSavedMissing = errors.New("saved search not found")
)

type SavedSearch struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	Notify    bool      `json:"notify"`
	NewCount  int       `json:"new_count"` // cards listed since the last alert or visit
	CreatedAt time.Time `json:"created_at"`
}

// normalizeQuery drops paging, view and empty params and sorts the rest, so
// the same search saved twice is recognised.
func normalizeQuery(raw string) (string, url.Values, error) {
	q, err := url.ParseQuery(strings.TrimPrefix(strings.TrimSpace(raw), "?"))
	if err != nil {
		return "", nil, ErrSavedBad
	}
	out := url.Values{}
	for k, vs := range q {
		switch k {
		case "offset", "limit", "page", "view", "sort", "saved":
			continue
		}
		for _, v := range vs {
			if v = strings.TrimSpace(v); v != "" {
				out.Add(k, v)
			}
		}
	}
	if len(out) == 0 {
		return "", nil, ErrSavedEmpty
	}
	if _, bad := listings.ParseSearch(out); len(bad) > 0 {
		return "", nil, ErrSavedBad
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out.Encode(), out, nil // Encode sorts keys
}

// SaveSearch stores a search for userID.
func (s *Service) SaveSearch(ctx context.Context, userID, name, rawQuery string, notify bool) (SavedSearch, error) {
	q, vals, err := normalizeQuery(rawQuery)
	if err != nil {
		return SavedSearch{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = describe(vals)
	}
	if r := []rune(name); len(r) > 80 {
		name = string(r[:80])
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM saved_searches WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return SavedSearch{}, err
	}
	if n >= MaxSavedSearches {
		return SavedSearch{}, ErrSavedFull
	}
	ss := SavedSearch{Name: name, Query: q, Notify: notify}
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO saved_searches (user_id, name, query, notify) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`, userID, name, q, notify).Scan(&ss.ID, &ss.CreatedAt)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return SavedSearch{}, ErrSavedExists
	}
	return ss, err
}

// describe makes a short name from a search: the keywords, or the filters.
func describe(v url.Values) string {
	if q := strings.TrimSpace(v.Get("q")); q != "" {
		parts := []string{q}
		for _, k := range []string{"game", "grader", "set"} {
			if x := v.Get(k); x != "" {
				parts = append(parts, x)
			}
		}
		return strings.Join(parts, " · ")
	}
	var parts []string
	for _, k := range []string{"game", "category", "set", "grader", "min_grade", "condition", "attribute", "format", "max_price"} {
		if x := v.Get(k); x != "" {
			switch k {
			case "min_grade":
				x = x + "+"
			case "max_price":
				x = "under $" + x
			case "category":
				x = map[string]string{"tcg": "TCG", "sports": "Sports cards"}[x]
			}
			if x != "" {
				parts = append(parts, x)
			}
		}
	}
	if len(parts) == 0 {
		return "Saved search"
	}
	return strings.Join(parts, " · ")
}

// SavedSearches lists userID's saved searches with a count of cards listed
// since each was last checked for them.
func (s *Service) SavedSearches(ctx context.Context, userID string) ([]SavedSearch, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, name, query, notify, coalesce(last_notified_at, created_at), created_at
		FROM saved_searches WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	type row struct {
		SavedSearch
		since time.Time
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Name, &r.Query, &r.Notify, &r.since, &r.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()
	out := make([]SavedSearch, 0, len(list))
	for _, r := range list {
		if n, err := s.newMatches(ctx, r.Query, userID, r.since); err == nil {
			r.NewCount = n
		}
		out = append(out, r.SavedSearch)
	}
	return out, nil
}

// SetSavedNotify turns alerts on or off.
func (s *Service) SetSavedNotify(ctx context.Context, userID, id string, on bool) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE saved_searches SET notify = $3, checked_at = CASE WHEN $3 AND NOT notify THEN now() ELSE checked_at END
		WHERE id = $1 AND user_id = $2`, id, userID, on)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedMissing
	}
	return nil
}

// SeenSaved resets a search's "new" count when the member opens it.
func (s *Service) SeenSaved(ctx context.Context, userID, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE saved_searches SET last_notified_at = now() WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// DeleteSaved removes a saved search.
func (s *Service) DeleteSaved(ctx context.Context, userID, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM saved_searches WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedMissing
	}
	return nil
}

// RunSavedAlerts notifies members about new cards matching their saved
// searches, at most once every six hours per search, never for their own
// listings. main calls it every few minutes.
func (s *Service) RunSavedAlerts(ctx context.Context) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ss.id, ss.user_id, ss.name, ss.query, ss.checked_at
		FROM saved_searches ss JOIN users u ON u.id = ss.user_id
		WHERE ss.notify AND u.status = 'active'
		  AND (ss.last_notified_at IS NULL OR ss.last_notified_at < now() - $1::interval)
		ORDER BY ss.checked_at LIMIT 200`, fmt.Sprintf("%d seconds", int(alertGap.Seconds())))
	if err != nil {
		return 0, err
	}
	type due struct {
		id, user, name, query string
		since                 time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.user, &d.name, &d.query, &d.since); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, d)
	}
	rows.Close()
	sent := 0
	for _, d := range list {
		now := time.Now()
		n, err := s.newMatches(ctx, d.query, d.user, d.since)
		if errors.Is(err, ErrSavedBad) {
			s.DB.ExecContext(ctx, `UPDATE saved_searches SET notify = false WHERE id = $1`, d.id)
			continue
		}
		if err != nil {
			return sent, err
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return sent, err
		}
		if n > 0 {
			title := fmt.Sprintf("%d new %s for “%s”", n, plural(n, "card", "cards"), d.name)
			if err := inbox.Notify(ctx, tx, d.user, "saved_search", title,
				"New listings match a search you saved.", "/search?"+d.query+"&sort=newest&saved="+d.id); err != nil {
				tx.Rollback()
				return sent, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE saved_searches SET checked_at = $2, last_notified_at = $2 WHERE id = $1`, d.id, now); err != nil {
				tx.Rollback()
				return sent, err
			}
			sent++
		} else if _, err := tx.ExecContext(ctx, `UPDATE saved_searches SET checked_at = $2 WHERE id = $1`, d.id, now); err != nil {
			tx.Rollback()
			return sent, err
		}
		if err := tx.Commit(); err != nil {
			return sent, err
		}
	}
	return sent, nil
}

// newMatches counts public cards matching query listed after since, not
// counting the member's own.
func (s *Service) newMatches(ctx context.Context, query, userID string, since time.Time) (int, error) {
	vals, err := url.ParseQuery(query)
	if err != nil {
		return 0, ErrSavedBad
	}
	p, bad := listings.ParseSearch(vals)
	if len(bad) > 0 {
		return 0, ErrSavedBad
	}
	p.ListedAfter = since
	p.Limit = 100
	res, err := s.Listings.Search(ctx, p)
	if err != nil {
		return 0, err
	}
	n := 0
	var mine string
	s.DB.QueryRowContext(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&mine)
	for _, l := range res.Listings {
		if l.SellerUsername != mine {
			n++
		}
	}
	if res.Total > len(res.Listings) {
		n += res.Total - len(res.Listings)
	}
	return n, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
