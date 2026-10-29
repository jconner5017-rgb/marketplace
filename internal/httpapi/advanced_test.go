package httpapi

import (
	"net/url"
	"testing"
)

func TestAdvancedSearch(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "adv_shop", true)
	e.db.Exec(`UPDATE seller_profiles SET tier = 3, tier_locked = true`)
	add := func(m map[string]any) {
		if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/listings", m); code != 201 {
			t.Fatalf("add: %d %v", code, body)
		}
	}
	a := rawCard("A", 1000)
	a["title"], a["description"], a["year"], a["card_number"] = "Charizard Holo Base Set", "Shadowless print", 1999, "4/102"
	add(a)
	b := rawCard("B", 2000)
	b["title"], b["year"], b["attributes"], b["card_number"] = "Charizard Holo Reprint Celebrations", 2021, []string{"holo", "first_edition"}, "4/25"
	add(b)
	g := rawCard("C", 30000)
	delete(g, "condition")
	g["title"], g["grader"], g["grade"], g["cert_number"], g["year"] = "Blastoise Holo PSA", "PSA", 9, "4401234567", 1999
	g["card_number"] = "2/102"
	add(g)
	m := rawCard("D", 800)
	m["game_or_sport"], m["set_name"], m["title"], m["condition"], m["year"] = "Magic", "Alpha", "Lightning Bolt", "MP", 1993
	m["card_number"] = "161"
	add(m)

	anon := client(t)
	n := func(qs url.Values) int {
		code, body := do(t, anon, "GET", e.ts.URL+"/v1/listings?"+qs.Encode(), nil)
		if code != 200 {
			t.Fatalf("%s: %d %v", qs.Encode(), code, body)
		}
		return int(body["total"].(float64))
	}
	cases := []struct {
		name string
		qs   url.Values
		want int
	}{
		{"all words", url.Values{"q": {"charizard holo"}}, 2},
		{"any words", url.Values{"q": {"blastoise bolt"}, "mode": {"any"}}, 2},
		{"exact phrase", url.Values{"q": {"holo base"}, "mode": {"exact"}}, 1},
		{"all words needs every word", url.Values{"q": {"charizard celebrations"}}, 1},
		{"exclude", url.Values{"q": {"charizard"}, "exclude": {"reprint"}}, 1},
		{"exclude only", url.Values{"exclude": {"charizard"}}, 2},
		{"description match", url.Values{"q": {"shadowless"}}, 1},
		{"title only", url.Values{"q": {"shadowless"}, "in": {"title"}}, 0},
		{"year range", url.Values{"year_min": {"1995"}, "year_max": {"2000"}}, 2},
		{"card number", url.Values{"number": {"#4/102"}}, 1},
		{"graders incl raw", url.Values{"grader": {"PSA", "RAW"}}, 4},
		{"graders comma", url.Values{"grader": {"PSA,BGS"}}, 1},
		{"cert", url.Values{"cert": {"4401234567"}}, 1},
		{"conditions", url.Values{"condition": {"LP", "MP"}}, 3},
		{"games multi", url.Values{"game": {"Pokemon", "Magic"}}, 4},
		{"attributes all", url.Values{"attribute": {"holo", "first_edition"}}, 1},
		{"no discount yet", url.Values{"discount": {"1"}}, 0},
	}
	for _, tc := range cases {
		if got := n(tc.qs); got != tc.want {
			t.Errorf("%s (%s): got %d, want %d", tc.name, tc.qs.Encode(), got, tc.want)
		}
	}
	do(t, c, "PATCH", e.ts.URL+"/v1/seller/settings", map[string]any{"bundle_discount_type": "percent_3plus", "bundle_discount_val": 1000})
	if got := n(url.Values{"discount": {"1"}}); got != 4 {
		t.Errorf("discount after setting: %d", got)
	}
	if code, _ := do(t, anon, "GET", e.ts.URL+"/v1/listings?mode=fuzzy", nil); code != 400 {
		t.Errorf("bad mode: want 400, got %d", code)
	}
}
