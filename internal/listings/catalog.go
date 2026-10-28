package listings

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/lib/pq"
)

// Sports is the fixed list of sports a sports card can be listed under.
// Names are stored exactly as written here so search filters and counts
// line up.
var Sports = []string{
	"Baseball", "Football", "Basketball", "Hockey", "Soccer", "Auto Racing",
	"Wrestling", "MMA", "Golf", "Boxing", "Tennis", "Cycling",
}

// Games is the fixed list of trading card games, plus "Other TCG" for
// games not on the list yet. Names are stored exactly as written here.
var Games = []string{
	"Pokemon", "Magic", "Yu-Gi-Oh!", "One Piece", "Lorcana", "Dragon Ball",
	"Digimon", "Weiss Schwarz", "Flesh and Blood", "Final Fantasy", "Star Wars",
	"Lord of the Rings", "Naruto", "UniVersus", "Cardfight!! Vanguard", OtherTCG,
}

const OtherTCG = "Other TCG"

var sportAliases = map[string]string{
	"nfl": "Football", "american football": "Football",
	"mlb": "Baseball", "nba": "Basketball", "wnba": "Basketball",
	"nhl": "Hockey", "ice hockey": "Hockey",
	"futbol": "Soccer", "fútbol": "Soccer", "mls": "Soccer",
	"racing": "Auto Racing", "nascar": "Auto Racing", "f1": "Auto Racing", "formula 1": "Auto Racing",
	"motorsport": "Auto Racing", "motorsports": "Auto Racing", "indycar": "Auto Racing",
	"wwe": "Wrestling", "aew": "Wrestling", "pro wrestling": "Wrestling",
	"mixed martial arts": "MMA", "mma (mixed martial arts)": "MMA", "ufc": "MMA",
	"pga": "Golf", "atp": "Tennis", "wta": "Tennis", "cycling cards": "Cycling", "bike": "Cycling",
}

var gameAliases = map[string]string{
	"pokémon": "Pokemon", "pokemon tcg": "Pokemon", "pokémon tcg": "Pokemon", "ptcg": "Pokemon",
	"magic the gathering": "Magic", "magic: the gathering": "Magic", "magic: the gathering (mtg)": "Magic", "mtg": "Magic",
	"yugioh": "Yu-Gi-Oh!", "yu-gi-oh": "Yu-Gi-Oh!", "yu gi oh": "Yu-Gi-Oh!", "yu-gi-oh! tcg": "Yu-Gi-Oh!", "ygo": "Yu-Gi-Oh!",
	"one piece card game": "One Piece", "one piece tcg": "One Piece", "one piece ccg": "One Piece", "optcg": "One Piece",
	"disney lorcana": "Lorcana", "disney lorcana tcg": "Lorcana", "lorcana tcg": "Lorcana",
	"dragon ball super": "Dragon Ball", "dragon ball z": "Dragon Ball", "dragon ball ccg": "Dragon Ball", "dragon ball z tcg": "Dragon Ball",
	"dragon ball super card game": "Dragon Ball", "dbs": "Dragon Ball", "dbz": "Dragon Ball", "dbs fusion world": "Dragon Ball",
	"digimon ccg": "Digimon", "digimon card game": "Digimon", "digimon tcg": "Digimon",
	"weiß schwarz": "Weiss Schwarz", "weiss": "Weiss Schwarz", "ws": "Weiss Schwarz",
	"fab": "Flesh and Blood", "flesh & blood": "Flesh and Blood",
	"final fantasy tcg": "Final Fantasy", "fftcg": "Final Fantasy", "final fantasy trading card game": "Final Fantasy",
	"star wars unlimited": "Star Wars", "star wars ccg": "Star Wars", "star wars trading card game": "Star Wars",
	"star wars tcg": "Star Wars", "swu": "Star Wars", "swccg": "Star Wars",
	"the lord of the rings": "Lord of the Rings", "the lord of the rings tcg": "Lord of the Rings", "lord of the rings tcg": "Lord of the Rings",
	"lotr": "Lord of the Rings", "lotr tcg": "Lord of the Rings",
	"naruto ccg": "Naruto", "naruto tcg": "Naruto",
	"universal fighting system": "UniVersus", "ufs": "UniVersus", "universus ccg": "UniVersus",
	"cardfight vanguard": "Cardfight!! Vanguard", "cardfight!! vanguard tcg": "Cardfight!! Vanguard", "vanguard": "Cardfight!! Vanguard",
	"other": OtherTCG, "other tcg": OtherTCG,
}

// Built-in display labels, used when the catalog tables are first filled.
var defaultLabels = map[string][2]string{
	"MMA":                  {"MMA (Mixed Martial Arts)", "MMA"},
	"Pokemon":              {"Pokémon TCG", "Pokémon"},
	"Magic":                {"Magic: The Gathering", "Magic"},
	"Yu-Gi-Oh!":            {"Yu-Gi-Oh! TCG", "Yu-Gi-Oh!"},
	"One Piece":            {"One Piece Card Game", "One Piece"},
	"Lorcana":              {"Disney Lorcana", "Lorcana"},
	"Dragon Ball":          {"Dragon Ball Super / Z", "Dragon Ball"},
	"Digimon":              {"Digimon Card Game", "Digimon"},
	"Weiss Schwarz":        {"Weiß Schwarz", "Weiß Schwarz"},
	"Final Fantasy":        {"Final Fantasy TCG", "Final Fantasy"},
	"Star Wars":            {"Star Wars CCG / Unlimited", "Star Wars"},
	"Lord of the Rings":    {"The Lord of the Rings TCG", "Lord of the Rings"},
	"Naruto":               {"Naruto CCG", "Naruto"},
	"UniVersus":            {"UniVersus (UFS)", "UniVersus"},
	"Cardfight!! Vanguard": {"Cardfight!! Vanguard", "Vanguard"},
	OtherTCG:               {"Other trading card games", "Other TCG"},
}

var defaultAttrLabels = map[string]string{
	"holo": "Holo", "reverse_holo": "Reverse holo", "first_edition": "1st Edition", "shadowless": "Shadowless",
	"foil": "Foil", "full_art": "Full art", "secret_rare": "Secret rare", "rookie": "Rookie", "auto": "Autograph",
	"patch": "Patch", "numbered": "Serial numbered", "parallel": "Parallel", "short_print": "Short print", "error": "Error card",
}

// Category is one sport or game.
type Category struct {
	ID         int      `json:"id"`
	Kind       string   `json:"kind"` // sports or tcg
	Value      string   `json:"value"`
	Label      string   `json:"label"`
	ShortLabel string   `json:"short_label"`
	Aliases    []string `json:"aliases"`
	Active     bool     `json:"active"`
	Sort       int      `json:"sort"`
	Listings   int      `json:"listings,omitempty"`
}

// Feature is one card feature (item specific).
type Feature struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Active   bool   `json:"active"`
	Sort     int    `json:"sort"`
	Listings int    `json:"listings,omitempty"`
}

type catalogState struct {
	sports, games         []string // active values
	sportAlias, gameAlias map[string]string
	attrs                 []string // active keys
	categories            []Category
	features              []Feature
}

var current atomic.Pointer[catalogState]

func init() { current.Store(defaultState()) }

func defaultState() *catalogState {
	st := &catalogState{sports: Sports, games: Games, sportAlias: sportAliases, gameAlias: gameAliases, attrs: Attributes}
	for i, v := range Sports {
		st.categories = append(st.categories, defaultCategory("sports", v, i, sportAliases))
	}
	for i, v := range Games {
		st.categories = append(st.categories, defaultCategory("tcg", v, i, gameAliases))
	}
	for i, k := range Attributes {
		st.features = append(st.features, Feature{Key: k, Label: defaultAttrLabels[k], Active: true, Sort: i})
	}
	return st
}

func defaultCategory(kind, v string, i int, aliases map[string]string) Category {
	l, ok := defaultLabels[v]
	if !ok {
		l = [2]string{v, v}
	}
	c := Category{Kind: kind, Value: v, Label: l[0], ShortLabel: l[1], Active: true, Sort: i, Aliases: []string{}}
	for a, to := range aliases {
		if to == v {
			c.Aliases = append(c.Aliases, a)
		}
	}
	slices.Sort(c.Aliases)
	return c
}

// ActiveSports, ActiveGames and ActiveAttributes are what new listings may use.
func ActiveSports() []string     { return current.Load().sports }
func ActiveGames() []string      { return current.Load().games }
func ActiveAttributes() []string { return current.Load().attrs }

// CatalogView is the public catalog: active categories and features.
func CatalogView() ([]Category, []Feature) {
	st := current.Load()
	cats, feats := []Category{}, []Feature{}
	for _, c := range st.categories {
		if c.Active {
			cats = append(cats, c)
		}
	}
	for _, f := range st.features {
		if f.Active {
			feats = append(feats, f)
		}
	}
	return cats, feats
}

// Catalog stores the editable catalog.
type Catalog struct{ DB *sql.DB }

var (
	ErrCatalogNotFound = errors.New("not in the catalog")
	ErrCatalogInUse    = errors.New("listings use this; hide it instead of deleting it")
	ErrCatalogDup      = errors.New("that name or alias is already in the catalog")
)

// Load fills the tables from the built-in lists on first run, then makes
// the stored catalog the one listings are checked against.
func (c *Catalog) Load(ctx context.Context) error {
	var n int
	if err := c.DB.QueryRowContext(ctx, `SELECT count(*) FROM catalog_categories`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		for _, cat := range defaultState().categories {
			if _, err := c.DB.ExecContext(ctx, `INSERT INTO catalog_categories (kind, value, label, short_label, aliases, sort)
				VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, cat.Kind, cat.Value, cat.Label, cat.ShortLabel, pq.Array(cat.Aliases), cat.Sort); err != nil {
				return err
			}
		}
	}
	if err := c.DB.QueryRowContext(ctx, `SELECT count(*) FROM catalog_attributes`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		for _, f := range defaultState().features {
			if _, err := c.DB.ExecContext(ctx, `INSERT INTO catalog_attributes (key, label, sort) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				f.Key, f.Label, f.Sort); err != nil {
				return err
			}
		}
	}
	cats, err := c.Categories(ctx, false)
	if err != nil {
		return err
	}
	feats, err := c.Features(ctx, false)
	if err != nil {
		return err
	}
	st := &catalogState{sportAlias: map[string]string{}, gameAlias: map[string]string{}, categories: cats, features: feats}
	for _, cat := range cats {
		alias := st.gameAlias
		if cat.Kind == "sports" {
			alias = st.sportAlias
		}
		for _, a := range cat.Aliases {
			alias[strings.ToLower(a)] = cat.Value
		}
		if !cat.Active {
			continue
		}
		if cat.Kind == "sports" {
			st.sports = append(st.sports, cat.Value)
		} else {
			st.games = append(st.games, cat.Value)
		}
	}
	for _, f := range feats {
		if f.Active {
			st.attrs = append(st.attrs, f.Key)
		}
	}
	current.Store(st)
	return nil
}

// Categories lists every sport and game, optionally with listing counts.
func (c *Catalog) Categories(ctx context.Context, counts bool) ([]Category, error) {
	q := `SELECT id, kind, value, label, short_label, aliases, active, sort, 0 FROM catalog_categories ORDER BY kind DESC, sort, id`
	if counts {
		q = `SELECT c.id, c.kind, c.value, c.label, c.short_label, c.aliases, c.active, c.sort,
			(SELECT count(*) FROM listings l WHERE l.category = c.kind AND l.game_or_sport = c.value AND l.status NOT IN ('removed'))
			FROM catalog_categories c ORDER BY c.kind DESC, c.sort, c.id`
	}
	rows, err := c.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var x Category
		if err := rows.Scan(&x.ID, &x.Kind, &x.Value, &x.Label, &x.ShortLabel, pq.Array(&x.Aliases), &x.Active, &x.Sort, &x.Listings); err != nil {
			return nil, err
		}
		if x.Aliases == nil {
			x.Aliases = []string{}
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Features lists every card feature, optionally with listing counts.
func (c *Catalog) Features(ctx context.Context, counts bool) ([]Feature, error) {
	q := `SELECT key, label, active, sort, 0 FROM catalog_attributes ORDER BY sort, key`
	if counts {
		q = `SELECT a.key, a.label, a.active, a.sort,
			(SELECT count(*) FROM listings l WHERE a.key = ANY(l.attributes) AND l.status NOT IN ('removed'))
			FROM catalog_attributes a ORDER BY a.sort, a.key`
	}
	rows, err := c.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Feature{}
	for rows.Next() {
		var f Feature
		if err := rows.Scan(&f.Key, &f.Label, &f.Active, &f.Sort, &f.Listings); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CategoryInput is a new or changed sport/game. Value can't change once
// listings use it (they store it), so renames change the label.
type CategoryInput struct {
	Kind       string   `json:"kind"`
	Value      string   `json:"value"`
	Label      string   `json:"label"`
	ShortLabel string   `json:"short_label"`
	Aliases    []string `json:"aliases"`
	Active     *bool    `json:"active"`
	Sort       *int     `json:"sort"`
}

func cleanAliases(in []string) []string {
	out := []string{}
	for _, a := range in {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && len(a) <= 60 && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}

func validCategory(in *CategoryInput) error {
	in.Value, in.Label, in.ShortLabel = strings.TrimSpace(in.Value), strings.TrimSpace(in.Label), strings.TrimSpace(in.ShortLabel)
	f := map[string]string{}
	if in.Kind != "sports" && in.Kind != "tcg" {
		f["kind"] = "sports or tcg"
	}
	if in.Value == "" || len(in.Value) > 60 {
		f["value"] = "1 to 60 characters"
	}
	if in.Label == "" {
		in.Label = in.Value
	}
	if in.ShortLabel == "" {
		in.ShortLabel = in.Label
	}
	if len(in.Label) > 80 || len(in.ShortLabel) > 40 {
		f["label"] = "labels can be up to 80 characters (short label 40)"
	}
	in.Aliases = cleanAliases(in.Aliases)
	if len(in.Aliases) > 40 {
		f["aliases"] = "up to 40 alternate names"
	}
	if len(f) > 0 {
		return &ValidationError{Fields: f}
	}
	return nil
}

// AddCategory creates a sport or game.
func (c *Catalog) AddCategory(ctx context.Context, tx *sql.Tx, in CategoryInput) (Category, error) {
	if err := validCategory(&in); err != nil {
		return Category{}, err
	}
	active, sort := true, 1000
	if in.Active != nil {
		active = *in.Active
	}
	if in.Sort != nil {
		sort = *in.Sort
	}
	var x Category
	err := tx.QueryRowContext(ctx, `INSERT INTO catalog_categories (kind, value, label, short_label, aliases, active, sort)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, in.Kind, in.Value, in.Label, in.ShortLabel, pq.Array(in.Aliases), active, sort).Scan(&x.ID)
	if dupErr(err) {
		return Category{}, ErrCatalogDup
	}
	if err != nil {
		return Category{}, err
	}
	return Category{ID: x.ID, Kind: in.Kind, Value: in.Value, Label: in.Label, ShortLabel: in.ShortLabel, Aliases: in.Aliases, Active: active, Sort: sort}, nil
}

// UpdateCategory changes labels, aliases, order or visibility. It returns
// the category before and after.
func (c *Catalog) UpdateCategory(ctx context.Context, tx *sql.Tx, id int, in CategoryInput) (Category, Category, error) {
	var b Category
	err := tx.QueryRowContext(ctx, `SELECT id, kind, value, label, short_label, aliases, active, sort FROM catalog_categories WHERE id = $1 FOR UPDATE`, id).
		Scan(&b.ID, &b.Kind, &b.Value, &b.Label, &b.ShortLabel, pq.Array(&b.Aliases), &b.Active, &b.Sort)
	if errors.Is(err, sql.ErrNoRows) {
		return b, b, ErrCatalogNotFound
	}
	if err != nil {
		return b, b, err
	}
	a := b
	if in.Label != "" {
		a.Label = strings.TrimSpace(in.Label)
	}
	if in.ShortLabel != "" {
		a.ShortLabel = strings.TrimSpace(in.ShortLabel)
	}
	if in.Aliases != nil {
		a.Aliases = cleanAliases(in.Aliases)
	}
	if in.Active != nil {
		a.Active = *in.Active
	}
	if in.Sort != nil {
		a.Sort = *in.Sort
	}
	check := CategoryInput{Kind: a.Kind, Value: a.Value, Label: a.Label, ShortLabel: a.ShortLabel, Aliases: a.Aliases}
	if err := validCategory(&check); err != nil {
		return b, a, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE catalog_categories SET label = $2, short_label = $3, aliases = $4, active = $5, sort = $6,
		updated_at = now() WHERE id = $1`, id, check.Label, check.ShortLabel, pq.Array(check.Aliases), a.Active, a.Sort)
	return b, a, err
}

// DeleteCategory removes a sport or game no listing uses.
func (c *Catalog) DeleteCategory(ctx context.Context, tx *sql.Tx, id int) (Category, error) {
	var x Category
	err := tx.QueryRowContext(ctx, `SELECT kind, value FROM catalog_categories WHERE id = $1 FOR UPDATE`, id).Scan(&x.Kind, &x.Value)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrCatalogNotFound
	}
	if err != nil {
		return x, err
	}
	var used bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM listings WHERE category = $1 AND game_or_sport = $2)`, x.Kind, x.Value).Scan(&used); err != nil {
		return x, err
	}
	if used {
		return x, ErrCatalogInUse
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM catalog_categories WHERE id = $1`, id)
	return x, err
}

// SaveFeature adds or changes a card feature.
func (c *Catalog) SaveFeature(ctx context.Context, tx *sql.Tx, f Feature) error {
	f.Key = strings.ToLower(strings.TrimSpace(f.Key))
	f.Label = strings.TrimSpace(f.Label)
	if !featureKey(f.Key) || f.Label == "" || len(f.Label) > 60 {
		return &ValidationError{Fields: map[string]string{"key": "2-40 lowercase letters, numbers or underscores, with a label"}}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO catalog_attributes (key, label, active, sort) VALUES ($1, $2, $3, $4)
		ON CONFLICT (key) DO UPDATE SET label = EXCLUDED.label, active = EXCLUDED.active, sort = EXCLUDED.sort, updated_at = now()`,
		f.Key, f.Label, f.Active, f.Sort)
	return err
}

// DeleteFeature removes a feature no listing uses.
func (c *Catalog) DeleteFeature(ctx context.Context, tx *sql.Tx, key string) error {
	var used bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM listings WHERE $1 = ANY(attributes))`, key).Scan(&used); err != nil {
		return err
	}
	if used {
		return ErrCatalogInUse
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM catalog_attributes WHERE key = $1`, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCatalogNotFound
	}
	return nil
}

func featureKey(k string) bool {
	if len(k) < 2 || len(k) > 40 {
		return false
	}
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

func dupErr(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// canonical returns the official spelling of name from list or aliases,
// or "" when it isn't recognized.
func canonical(name string, list []string, aliases map[string]string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, v := range list {
		if strings.ToLower(v) == key {
			return v
		}
	}
	return aliases[key]
}

// normalizeGameOrSport fixes the spelling of known sports and games.
func normalizeGameOrSport(category, name string) string {
	st := current.Load()
	if category == "sports" {
		if c := canonical(name, st.sports, st.sportAlias); c != "" {
			return c
		}
		return name
	}
	if c := canonical(name, st.games, st.gameAlias); c != "" {
		return c
	}
	return name
}
