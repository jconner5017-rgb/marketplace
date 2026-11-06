// Package identify reads a card from its photos with Claude and returns the
// listing details (card, set, year, number, grade or condition, features),
// checked against the site's catalog so only values the listing form accepts
// come back.
package identify

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
)

var (
	// ErrOff: no API key is set.
	ErrOff = errors.New("photo identification isn't switched on yet")
	// ErrUnreadable: Claude couldn't tell what the card is.
	ErrUnreadable = errors.New("couldn't identify the card from these photos; try clearer, closer photos of the front and back")
)

const DefaultModel = "claude-haiku-5-5"

// Option is one allowed value and its label.
type Option struct{ Value, Label string }

// Choices are the values the listing form accepts.
type Choices struct {
	Kinds      map[string][]Option // sports, tcg, nonsport -> sports, games, themes
	Graders    []string
	Conditions []string
	Features   []Option
}

// Result is what the form is filled with. Empty fields mean "couldn't tell".
type Result struct {
	Category    string   `json:"category"`
	GameOrSport string   `json:"game_or_sport"`
	Title       string   `json:"title"`
	SetName     string   `json:"set_name"`
	Year        *int     `json:"year"`
	CardNumber  string   `json:"card_number"`
	Graded      bool     `json:"graded"`
	Grader      string   `json:"grader"`
	Grade       *float64 `json:"grade"`
	CertNumber  string   `json:"cert_number"`
	Condition   string   `json:"condition"`
	Attributes  []string `json:"attributes"`
	Notes       string   `json:"notes"`
}

// Client calls the Anthropic Messages API.
type Client struct {
	APIKey  string
	Model   string
	BaseURL string // tests point this at a fake server
	HTTP    *http.Client
}

func (c *Client) Enabled() bool { return c != nil && strings.TrimSpace(c.APIKey) != "" }

func (c *Client) prompt(ch Choices) string {
	var b strings.Builder
	b.WriteString("You identify trading cards for a marketplace listing. The photos show one card: the front first, then usually the back.\n")
	b.WriteString("Reply with ONLY a JSON object, no other text, with these keys:\n")
	b.WriteString(`{"category":"sports|tcg|nonsport","game_or_sport":"<value from the list>","title":"...","set_name":"...","year":2024,"card_number":"...","graded":false,"grader":"","grade":null,"cert_number":"","condition":"","attributes":[],"notes":""}` + "\n")
	b.WriteString("Rules:\n")
	b.WriteString("- category and game_or_sport must use the values below exactly; use \"\" if unsure.\n")
	for _, k := range []string{"sports", "tcg", "nonsport"} {
		vals := []string{}
		for _, o := range ch.Kinds[k] {
			vals = append(vals, o.Value+" ("+o.Label+")")
		}
		fmt.Fprintf(&b, "  %s: %s\n", k, strings.Join(vals, ", "))
	}
	b.WriteString("- title: how a collector would search for it, under 80 characters: year, brand/set, player or character, parallel or variant, card number with #, and RC/Auto/serial like /99 when shown. Example: \"2023 Panini Prizm Victor Wembanyama Silver Prizm RC #136\".\n")
	b.WriteString("- set_name: the set or product (e.g. \"Topps Chrome\", \"Scarlet & Violet 151\"). year: the year printed on the card or set, as a number, or null.\n")
	b.WriteString("- card_number: as printed, without #.\n")
	fmt.Fprintf(&b, "- graded: true only if the card is in a grading slab. Then grader is one of %s (OTHER for any other company), grade is the number on the label (1 to 10, halves allowed), cert_number is the certification number on the label.\n", strings.Join(ch.Graders, ", "))
	fmt.Fprintf(&b, "- condition (raw cards only): one of %s from what the photos show; \"\" if you can't judge.\n", strings.Join(ch.Conditions, ", "))
	feats := []string{}
	for _, f := range ch.Features {
		feats = append(feats, f.Value+" ("+f.Label+")")
	}
	fmt.Fprintf(&b, "- attributes: any that clearly apply, from: %s.\n", strings.Join(feats, ", "))
	b.WriteString("- notes: one short sentence about anything you weren't sure of, or \"\".\n")
	b.WriteString("- Never guess a player, set or number you can't read; leave it \"\". If the photos don't show a trading card, return all fields empty.\n")
	return b.String()
}

// Identify sends the photos (JPEG bytes) and returns the cleaned result.
func (c *Client) Identify(ctx context.Context, images [][]byte, ch Choices) (Result, error) {
	if !c.Enabled() {
		return Result{}, ErrOff
	}
	content := []map[string]any{}
	for _, img := range images {
		content = append(content, map[string]any{"type": "image", "source": map[string]any{
			"type": "base64", "media_type": sniff(img), "data": base64.StdEncoding.EncodeToString(img)}})
	}
	content = append(content, map[string]any{"type": "text", "text": c.prompt(ch)})
	model := c.Model
	if model == "" {
		model = DefaultModel
	}
	body, _ := json.Marshal(map[string]any{"model": model, "max_tokens": 700,
		"messages": []map[string]any{{"role": "user", "content": content}}})
	base := c.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("identify: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("identify: anthropic status %d: %s", res.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
	}
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Result{}, fmt.Errorf("identify: %w", err)
	}
	text := ""
	for _, part := range msg.Content {
		if part.Type == "text" {
			text += part.Text
		}
	}
	r, err := parse(text)
	if err != nil {
		return Result{}, ErrUnreadable
	}
	r = Clean(r, ch)
	if r.Title == "" && r.SetName == "" && r.GameOrSport == "" {
		return Result{}, ErrUnreadable
	}
	return r, nil
}

// parse pulls the JSON object out of the reply (tolerating code fences).
func parse(text string) (Result, error) {
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j <= i {
		return Result{}, errors.New("no JSON in reply")
	}
	var r Result
	err := json.Unmarshal([]byte(text[i:j+1]), &r)
	return r, err
}

func match(v string, opts []Option) string {
	v = strings.TrimSpace(v)
	for _, o := range opts {
		if strings.EqualFold(v, o.Value) || strings.EqualFold(v, o.Label) {
			return o.Value
		}
	}
	return ""
}

// Clean keeps only values the listing form accepts.
func Clean(r Result, ch Choices) Result {
	out := Result{Notes: trim(r.Notes, 200)}
	if _, ok := ch.Kinds[r.Category]; ok {
		out.Category = r.Category
		out.GameOrSport = match(r.GameOrSport, ch.Kinds[r.Category])
	} else {
		// A recognised sport or game also tells us the category.
		for k, opts := range ch.Kinds {
			if v := match(r.GameOrSport, opts); v != "" {
				out.Category, out.GameOrSport = k, v
				break
			}
		}
	}
	out.Title = trim(r.Title, 200)
	out.SetName = trim(r.SetName, 120)
	out.CardNumber = trim(strings.TrimPrefix(strings.TrimSpace(r.CardNumber), "#"), 40)
	if r.Year != nil && *r.Year >= 1850 && *r.Year <= time.Now().Year()+1 {
		y := *r.Year
		out.Year = &y
	}
	if r.Graded {
		g := strings.ToUpper(strings.TrimSpace(r.Grader))
		if !slices.Contains(ch.Graders, g) {
			g = "OTHER"
		}
		out.Graded, out.Grader = true, g
		if r.Grade != nil && *r.Grade >= 1 && *r.Grade <= 10 {
			v := math.Round(*r.Grade*2) / 2
			out.Grade = &v
		}
		out.CertNumber = trim(r.CertNumber, 40)
	} else if slices.Contains(ch.Conditions, strings.ToUpper(strings.TrimSpace(r.Condition))) {
		out.Condition = strings.ToUpper(strings.TrimSpace(r.Condition))
	}
	out.Attributes = []string{}
	for _, a := range r.Attributes {
		if v := match(a, ch.Features); v != "" && !slices.Contains(out.Attributes, v) {
			out.Attributes = append(out.Attributes, v)
		}
	}
	return out
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func sniff(b []byte) string {
	switch {
	case len(b) > 3 && b[0] == 0x89 && b[1] == 'P':
		return "image/png"
	case len(b) > 3 && string(b[:3]) == "GIF":
		return "image/gif"
	case len(b) > 12 && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return "image/jpeg"
}
