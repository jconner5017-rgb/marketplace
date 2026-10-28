package listings

import "strings"

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
	if category == "sports" {
		if c := canonical(name, Sports, sportAliases); c != "" {
			return c
		}
		return name
	}
	if c := canonical(name, Games, gameAliases); c != "" {
		return c
	}
	return name
}
