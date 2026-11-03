package listings

import "testing"

func TestNormalizeGameOrSport(t *testing.T) {
	cases := []struct{ cat, in, want string }{
		{"sports", "baseball", "Baseball"},
		{"sports", " NASCAR ", "Auto Racing"},
		{"sports", "UFC", "MMA"},
		{"sports", "Mixed Martial Arts", "MMA"},
		{"sports", "WWE", "Wrestling"},
		{"sports", "cycling", "Cycling"},
		{"sports", "Curling", "Curling"}, // unknown: left alone, then rejected by validate
		{"tcg", "pokémon", "Pokemon"},
		{"tcg", "MTG", "Magic"},
		{"tcg", "yugioh", "Yu-Gi-Oh!"},
		{"tcg", "Dragon Ball Z", "Dragon Ball"},
		{"tcg", "Weiß Schwarz", "Weiss Schwarz"},
		{"tcg", "LOTR", "Lord of the Rings"},
		{"tcg", "Universal Fighting System", "UniVersus"},
		{"tcg", "vanguard", "Cardfight!! Vanguard"},
		{"tcg", "SWU", "Star Wars"},
		{"tcg", "other", "Other TCG"},
		{"tcg", "Sorcery Contested Realm", "Sorcery"},
		{"tcg", "Gloomhaven Duel Masters", "Gloomhaven Duel Masters"}, // unknown: rejected by validate
	}
	for _, c := range cases {
		if got := normalizeGameOrSport(c.cat, c.in); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.cat, c.in, got, c.want)
		}
	}
}

func TestSportsAreRequiredToBeKnown(t *testing.T) {
	cond := "NM"
	in := Input{Category: "sports", GameOrSport: "Curling", SetName: "X", Title: "Y", Condition: &cond,
		PhotoURLs: []string{"https://x.test/a.jpg"}, PriceCents: 100, Quantity: 1}
	in.normalize()
	err := in.validate(100000)
	ve, ok := err.(*ValidationError)
	if !ok || ve.Fields["game_or_sport"] == "" {
		t.Fatalf("want game_or_sport error, got %v", err)
	}
	for _, s := range Sports {
		in.GameOrSport = s
		if err := in.validate(100000); err != nil {
			t.Errorf("%s rejected: %v", s, err)
		}
	}
	in.Category = "tcg"
	for _, g := range Games {
		in.GameOrSport = g
		if err := in.validate(100000); err != nil {
			t.Errorf("%s rejected: %v", g, err)
		}
	}
	in.GameOrSport = "Gloomhaven Duel Masters"
	if err := in.validate(100000); err == nil {
		t.Error("unknown game accepted; it should go under Other TCG")
	}
}

func TestTidyTitle(t *testing.T) {
	for in, want := range map[string]string{
		" 2025 2025-26 Topps Chrome Basketball Nikola Topic ": "2025-26 Topps Chrome Basketball Nikola Topic",
		"2024 2024-25 Panini Prizm Global Reach":              "2024-25 Panini Prizm Global Reach",
		"2025 2025 Topps Chrome Future Stars Brice Turang":    "2025 Topps Chrome Future Stars Brice Turang",
		"2023 2022/23 Bowman University's Best":               "2022/23 Bowman University's Best",
		"2025 Topps Chrome Update (1985 Topps design)":        "2025 Topps Chrome Update (1985 Topps design)",
		"1999 Pokemon Base Set Charizard":                     "1999 Pokemon Base Set Charizard",
	} {
		if got := TidyTitle(in); got != want {
			t.Errorf("TidyTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
