package httpapi

import "testing"

func TestNonSportCategory(t *testing.T) {
	e := setupEnv(t)
	sc, _ := newSeller(t, e, "ns_shop", true)
	card := rawCard("NS-1", 900)
	card["category"] = "nonsport"
	card["game_or_sport"] = "marvel" // alias
	card["title"] = "zzspiderman 1992 Marvel Masterpieces"
	code, body := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", card)
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	l := body["listing"].(map[string]any)
	if l["category"] != "nonsport" || l["game_or_sport"] != "Superheroes" {
		t.Fatalf("listing: %v", l)
	}
	// Themes aren't games any more.
	card["external_id"] = "NS-2"
	card["category"] = "tcg"
	card["game_or_sport"] = "Superheroes"
	if code, _ := do(t, sc, "POST", e.ts.URL+"/v1/seller/listings", card); code != 422 {
		t.Fatalf("tcg superheroes: %d", code)
	}
	_, res := do(t, client(t), "GET", e.ts.URL+"/v1/listings?category=nonsport&q=zzspiderman", nil)
	if res["total"].(float64) != 1 {
		t.Fatalf("search: %v", res)
	}
	_, cat := do(t, client(t), "GET", e.ts.URL+"/v1/catalog", nil)
	ns := cat["nonsport"].([]any)
	if len(ns) == 0 || ns[0] != "Superheroes" {
		t.Fatalf("catalog nonsport: %v", ns)
	}
	for _, g := range cat["games"].([]any) {
		if g == "Superheroes" {
			t.Fatal("Superheroes still listed as a game")
		}
	}
}
