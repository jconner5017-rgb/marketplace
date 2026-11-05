// Package blog stores the site's guides, written and edited by staff.
package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrNotFound = errors.New("post not found")

// FieldError is a problem with one form field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Field + ": " + e.Msg }

// Link is one entry of a links block.
type Link struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Text  string `json:"text,omitempty"`
}

// Block is one piece of a post: exactly one field is set.
type Block struct {
	P     *string  `json:"p,omitempty"`
	H     *string  `json:"h,omitempty"`
	List  []string `json:"list,omitempty"`
	Note  *string  `json:"note,omitempty"`
	Links []Link   `json:"links,omitempty"`
}

type Post struct {
	ID          int        `json:"id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Keyword     string     `json:"keyword"`
	Intro       string     `json:"intro"`
	Body        []Block    `json:"body"`
	Status      string     `json:"status"`
	PublishedAt *time.Time `json:"published_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Minutes     int        `json:"minutes"` // reading time
}

type Service struct{ DB *sql.DB }

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Slugify turns a title into an address ("How to Sell!" -> "how-to-sell").
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 100 {
		out = strings.Trim(out[:100], "-")
	}
	return out
}

func okHref(h string) bool {
	if strings.HasPrefix(h, "/") && !strings.HasPrefix(h, "//") {
		return true
	}
	return strings.HasPrefix(h, "https://") && !strings.ContainsAny(h, " \"<>")
}

// Clean checks and tidies a post before saving.
func (p *Post) Clean() error {
	p.Title = strings.TrimSpace(p.Title)
	if n := utf8.RuneCountInString(p.Title); n < 5 || n > 140 {
		return &FieldError{"title", "a title of 5 to 140 characters"}
	}
	p.Slug = strings.TrimSpace(strings.ToLower(p.Slug))
	if p.Slug == "" {
		p.Slug = Slugify(p.Title)
	}
	if !slugRe.MatchString(p.Slug) || len(p.Slug) > 100 {
		return &FieldError{"slug", "use lowercase letters, numbers and dashes"}
	}
	p.Description, p.Keyword, p.Intro = strings.TrimSpace(p.Description), strings.TrimSpace(p.Keyword), strings.TrimSpace(p.Intro)
	if utf8.RuneCountInString(p.Description) > 300 {
		return &FieldError{"description", "keep the search description under 300 characters (about 155 is best)"}
	}
	if utf8.RuneCountInString(p.Keyword) > 120 {
		return &FieldError{"keyword", "keep the keyword under 120 characters"}
	}
	if utf8.RuneCountInString(p.Intro) > 2000 {
		return &FieldError{"intro", "keep the introduction under 2,000 characters"}
	}
	if p.Status != "published" {
		p.Status = "draft"
	}
	if len(p.Body) > 400 {
		return &FieldError{"body", "the post is too long"}
	}
	out := make([]Block, 0, len(p.Body))
	for _, b := range p.Body {
		set := 0
		for _, v := range []bool{b.P != nil, b.H != nil, len(b.List) > 0, b.Note != nil, len(b.Links) > 0} {
			if v {
				set++
			}
		}
		if set != 1 {
			continue // empty or ambiguous block: drop it
		}
		for i := range b.Links {
			b.Links[i].Label, b.Links[i].Href = strings.TrimSpace(b.Links[i].Label), strings.TrimSpace(b.Links[i].Href)
			if b.Links[i].Label == "" || !okHref(b.Links[i].Href) {
				return &FieldError{"body", "each link needs a label and an address starting with / or https://"}
			}
		}
		out = append(out, b)
	}
	p.Body = out
	return nil
}

func words(p Post) int {
	n := len(strings.Fields(p.Intro))
	for _, b := range p.Body {
		for _, s := range []*string{b.P, b.H, b.Note} {
			if s != nil {
				n += len(strings.Fields(*s))
			}
		}
		for _, s := range b.List {
			n += len(strings.Fields(s))
		}
	}
	return n
}

const cols = `id, slug, title, description, keyword, intro, body, status, published_at, updated_at`

func scan(row interface{ Scan(...any) error }) (Post, error) {
	var p Post
	var body []byte
	var pub sql.NullTime
	err := row.Scan(&p.ID, &p.Slug, &p.Title, &p.Description, &p.Keyword, &p.Intro, &body, &p.Status, &pub, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if pub.Valid {
		p.PublishedAt = &pub.Time
	}
	if err := json.Unmarshal(body, &p.Body); err != nil {
		return p, err
	}
	if p.Body == nil {
		p.Body = []Block{}
	}
	p.Minutes = max(1, (words(p)+219)/220)
	return p, nil
}

func (s *Service) list(ctx context.Context, q string, args ...any) ([]Post, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Published lists live posts, newest first.
func (s *Service) Published(ctx context.Context) ([]Post, error) {
	return s.list(ctx, `SELECT `+cols+` FROM blog_posts WHERE status = 'published' AND published_at <= now()
		ORDER BY published_at DESC, id DESC LIMIT 500`)
}

// All lists every post for the back office.
func (s *Service) All(ctx context.Context) ([]Post, error) {
	return s.list(ctx, `SELECT `+cols+` FROM blog_posts ORDER BY coalesce(published_at, created_at) DESC, id DESC`)
}

// BySlug is a live post.
func (s *Service) BySlug(ctx context.Context, slug string) (Post, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM blog_posts WHERE slug = $1 AND status = 'published'
		AND published_at <= now()`, strings.ToLower(strings.TrimSpace(slug))))
}

// Get is any post, for editing.
func (s *Service) Get(ctx context.Context, id int) (Post, error) {
	return scan(s.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM blog_posts WHERE id = $1`, id))
}

func dup(err error) bool { return err != nil && strings.Contains(err.Error(), "blog_posts_slug") }

// Save creates (ID 0) or updates a post. Publishing sets the date the first
// time; it stays when the post is edited later.
func (s *Service) Save(ctx context.Context, tx *sql.Tx, p Post, staffID string) (Post, error) {
	if err := p.Clean(); err != nil {
		return p, err
	}
	body, err := json.Marshal(p.Body)
	if err != nil {
		return p, err
	}
	var id int
	if p.ID == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO blog_posts (slug, title, description, keyword, intro, body, status, published_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $7 = 'published' THEN now() END, $8) RETURNING id`,
			p.Slug, p.Title, p.Description, p.Keyword, p.Intro, body, p.Status, staffID).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE blog_posts SET slug = $2, title = $3, description = $4, keyword = $5, intro = $6, body = $7,
			status = $8, published_at = CASE WHEN $8 = 'published' THEN coalesce(published_at, now()) ELSE published_at END, updated_at = now()
			WHERE id = $1 RETURNING id`, p.ID, p.Slug, p.Title, p.Description, p.Keyword, p.Intro, body, p.Status).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return p, ErrNotFound
		}
	}
	if dup(err) {
		return p, &FieldError{"slug", "another post already uses that address"}
	}
	if err != nil {
		return p, err
	}
	return scan(tx.QueryRowContext(ctx, `SELECT `+cols+` FROM blog_posts WHERE id = $1`, id))
}

// Delete removes a post.
func (s *Service) Delete(ctx context.Context, tx *sql.Tx, id int) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM blog_posts WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
