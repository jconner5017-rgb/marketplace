// Package demo loads clearly-labeled sample listings onto a test server so
// the website has something to show. Enable with SEED_DEMO=true; it is
// safe to run on every start (it updates, never duplicates).
package demo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/jconner5017-rgb/marketplace/internal/listings"
)

const (
	SellerEmail    = "demo-seller@example.invalid"
	SellerUsername = "sample_card_shop"
	stripeAccount  = "acct_demo_sample_shop"
)

type card struct {
	sku, cat, game, set string
	year                int
	num, title          string
	cond                string  // raw condition, or "" when graded
	grader              string  // "" when raw
	grade               float64 // 0 when raw
	attrs               []string
	cents, qty          int
}

var cards = []card{
	// TCG — Pokémon
	{"PKM-001", "tcg", "Pokemon", "Base Set", 1999, "4/102", "Charizard Holo Base Set", "LP", "", 0, []string{"holo"}, 18500, 1},
	{"PKM-002", "tcg", "Pokemon", "Base Set", 1999, "2/102", "Blastoise Holo Base Set", "", "PSA", 8, []string{"holo"}, 42000, 1},
	{"PKM-003", "tcg", "Pokemon", "Base Set", 1999, "58/102", "Pikachu Yellow Cheeks", "NM", "", 0, nil, 1299, 3},
	{"PKM-004", "tcg", "Pokemon", "Jungle", 1999, "1/64", "Clefable Holo 1st Edition", "MP", "", 0, []string{"holo", "first_edition"}, 4800, 1},
	{"PKM-005", "tcg", "Pokemon", "Evolving Skies", 2021, "215/203", "Umbreon VMAX Alt Art", "", "PSA", 10, []string{"secret_rare", "full_art"}, 89900, 1},
	{"PKM-006", "tcg", "Pokemon", "Scarlet & Violet 151", 2023, "199/165", "Charizard ex Special Illustration", "NM", "", 0, []string{"full_art", "secret_rare"}, 9500, 2},
	{"PKM-007", "tcg", "Pokemon", "Crown Zenith", 2023, "GG70", "Mewtwo VSTAR Galarian Gallery", "NM", "", 0, []string{"full_art"}, 2450, 4},
	{"PKM-008", "tcg", "Pokemon", "Paldea Evolved", 2023, "193/193", "Magikarp Illustration Rare", "NM", "", 0, []string{"full_art"}, 6200, 1},
	{"PKM-009", "tcg", "Pokemon", "Team Rocket", 2000, "4/82", "Dark Charizard Holo", "", "BGS", 9, []string{"holo"}, 61000, 1},
	{"PKM-010", "tcg", "Pokemon", "Base Set", 1999, "44/102", "Bulbasaur Common", "HP", "", 0, nil, 99, 12},
	// TCG — Magic
	{"MTG-001", "tcg", "Magic", "Revised", 1994, "", "Lightning Bolt", "LP", "", 0, nil, 1650, 4},
	{"MTG-002", "tcg", "Magic", "Modern Horizons 2", 2021, "", "Ragavan, Nimble Pilferer", "NM", "", 0, nil, 4200, 2},
	{"MTG-003", "tcg", "Magic", "Commander Masters", 2023, "", "Jeweled Lotus Foil", "NM", "", 0, []string{"foil"}, 7800, 1},
	{"MTG-004", "tcg", "Magic", "Dominaria United", 2022, "", "Sheoldred, the Apocalypse", "", "CGC", 9.5, nil, 11500, 1},
	{"MTG-005", "tcg", "Magic", "Unlimited", 1993, "", "Serra Angel", "MP", "", 0, nil, 3500, 1},
	{"MTG-006", "tcg", "Magic", "Wilds of Eldraine", 2023, "", "Agatha's Soul Cauldron Borderless", "NM", "", 0, []string{"full_art"}, 450, 8},
	// TCG — Yu-Gi-Oh!, One Piece, Lorcana
	{"YGO-001", "tcg", "Yu-Gi-Oh!", "Legend of Blue Eyes White Dragon", 2002, "LOB-001", "Blue-Eyes White Dragon 1st Edition", "LP", "", 0, []string{"first_edition"}, 32500, 1},
	{"YGO-002", "tcg", "Yu-Gi-Oh!", "Metal Raiders", 2002, "MRD-060", "Gate Guardian", "NM", "", 0, nil, 2400, 1},
	{"YGO-003", "tcg", "Yu-Gi-Oh!", "25th Anniversary Rarity Collection", 2023, "RA01-EN054", "Ash Blossom Quarter Century Secret", "", "PSA", 10, []string{"secret_rare"}, 15000, 1},
	{"OP-001", "tcg", "One Piece", "Romance Dawn", 2022, "OP01-120", "Shanks Secret Rare", "NM", "", 0, []string{"secret_rare"}, 8900, 1},
	{"OP-002", "tcg", "One Piece", "Paramount War", 2023, "OP02-013", "Portgas D. Ace Alt Art", "NM", "", 0, []string{"full_art"}, 5500, 2},
	{"LOR-001", "tcg", "Lorcana", "The First Chapter", 2023, "207/204", "Elsa Spirit of Winter Enchanted", "NM", "", 0, []string{"foil", "secret_rare"}, 39000, 1},
	{"DBS-001", "tcg", "Dragon Ball", "Fusion World Awakening", 2024, "", "Son Goku Secret Rare", "NM", "", 0, []string{"secret_rare"}, 5200, 1},
	{"DGM-001", "tcg", "Digimon", "BT-01 Release Special", 2021, "", "Omnimon Alt Art", "NM", "", 0, []string{"full_art"}, 3100, 1},
	{"FAB-001", "tcg", "Flesh and Blood", "Welcome to Rathe", 2019, "", "Cold Foil Hero", "LP", "", 0, []string{"foil"}, 8800, 1},
	{"FF-001", "tcg", "Final Fantasy", "Opus I", 2016, "", "Legend Foil", "NM", "", 0, []string{"foil"}, 1450, 2},
	{"SW-001", "tcg", "Star Wars", "Spark of Rebellion", 2024, "", "Hyperspace Leader", "NM", "", 0, []string{"full_art"}, 2600, 1},
	{"VG-001", "tcg", "Cardfight!! Vanguard", "Set 1", 2023, "", "Overdress Signature Rare", "NM", "", 0, []string{"secret_rare"}, 1750, 1},
	{"LOR-002", "tcg", "Lorcana", "Rise of the Floodborn", 2023, "", "Cinderella Stouthearted", "NM", "", 0, []string{"foil"}, 725, 3},
	// Sports — Baseball
	{"BB-001", "sports", "Baseball", "Topps Chrome", 2018, "150", "Shohei Ohtani Rookie", "", "PSA", 10, []string{"rookie"}, 52500, 1},
	{"BB-002", "sports", "Baseball", "Topps", 1989, "647", "Randy Johnson Rookie", "NM", "", 0, []string{"rookie"}, 650, 2},
	{"BB-003", "sports", "Baseball", "Upper Deck", 1989, "1", "Ken Griffey Jr. Rookie", "", "PSA", 9, []string{"rookie"}, 21000, 1},
	{"BB-004", "sports", "Baseball", "Bowman Chrome", 2023, "BCP-1", "Prospect Refractor /499", "NM", "", 0, []string{"numbered", "parallel"}, 3400, 1},
	{"BB-005", "sports", "Baseball", "Topps Heritage", 2024, "", "Team Card Short Print", "NM", "", 0, []string{"short_print"}, 899, 1},
	{"BB-006", "sports", "Baseball", "Donruss", 1990, "", "Commons Lot Card", "LP", "", 0, nil, 25, 40},
	// Sports — Basketball
	{"BK-001", "sports", "Basketball", "Panini Prizm", 2019, "248", "Zion Williamson Rookie Silver", "", "BGS", 9.5, []string{"rookie", "parallel"}, 31000, 1},
	{"BK-002", "sports", "Basketball", "Fleer", 1996, "", "Kobe Bryant Rookie", "LP", "", 0, []string{"rookie"}, 7500, 1},
	{"BK-003", "sports", "Basketball", "Panini Select", 2023, "", "Victor Wembanyama Concourse Rookie", "NM", "", 0, []string{"rookie"}, 4500, 3},
	{"BK-004", "sports", "Basketball", "Hoops", 2022, "", "Rookie Auto", "NM", "", 0, []string{"rookie", "auto"}, 1800, 1},
	// Sports — Football
	{"FB-001", "sports", "Football", "Panini Prizm", 2017, "269", "Patrick Mahomes Rookie", "", "PSA", 9, []string{"rookie"}, 79500, 1},
	{"FB-002", "sports", "Football", "Donruss Optic", 2023, "", "C.J. Stroud Rated Rookie Holo", "NM", "", 0, []string{"rookie", "holo"}, 2200, 2},
	{"FB-003", "sports", "Football", "Topps", 1986, "", "Jerry Rice Rookie", "MP", "", 0, []string{"rookie"}, 9900, 1},
	{"FB-004", "sports", "Football", "Panini Contenders", 2021, "", "Rookie Ticket Auto", "", "SGC", 9.5, []string{"rookie", "auto"}, 13800, 1},
	// Sports — Hockey and Soccer
	{"HK-001", "sports", "Hockey", "Upper Deck Young Guns", 2015, "201", "Connor McDavid Young Guns", "", "PSA", 9, []string{"rookie"}, 68000, 1},
	{"HK-002", "sports", "Hockey", "O-Pee-Chee", 2023, "", "Base Card", "NM", "", 0, nil, 150, 10},
	{"SC-001", "sports", "Soccer", "Topps Chrome UEFA", 2023, "", "Jude Bellingham Refractor", "NM", "", 0, []string{"parallel"}, 2900, 1},
	{"SC-002", "sports", "Soccer", "Panini Prizm World Cup", 2022, "", "Patch Auto /25", "", "PSA", 10, []string{"patch", "auto", "numbered"}, 47500, 1},
	// Sports — the rest of the lineup
	{"AR-001", "sports", "Auto Racing", "Wheels High Gear", 1998, "", "Dale Earnhardt Sr. Base", "NM", "", 0, nil, 1200, 1},
	{"AR-002", "sports", "Auto Racing", "Topps Chrome Formula 1", 2023, "", "Driver Refractor", "NM", "", 0, []string{"parallel"}, 3800, 1},
	{"WR-001", "sports", "Wrestling", "Topps WWE", 2023, "", "Superstar Base Card", "NM", "", 0, nil, 300, 6},
	{"MMA-001", "sports", "MMA", "Topps UFC Knockout", 2010, "", "Fighter Rookie", "", "PSA", 9, []string{"rookie"}, 16500, 1},
	{"GF-001", "sports", "Golf", "Upper Deck", 2001, "1", "Tiger Woods Rookie", "", "BGS", 9, []string{"rookie"}, 24000, 1},
	{"BX-001", "sports", "Boxing", "Ringside", 1996, "", "Heavyweight Champion", "LP", "", 0, nil, 1400, 1},
	{"TN-001", "sports", "Tennis", "Ace Authentic", 2003, "", "Grand Slam Champion Auto", "NM", "", 0, []string{"auto"}, 6900, 1},
	{"CY-001", "sports", "Cycling", "Panini Tour de France", 2004, "", "Stage Winner Sticker Card", "LP", "", 0, nil, 450, 2},
}

// Seed creates (or refreshes) the sample seller and listings.
// imageBase is the public URL the card images are served from.
func Seed(ctx context.Context, db *sql.DB, svc *listings.Service, imageBase string) (int, error) {
	if imageBase == "" {
		return 0, fmt.Errorf("demo seed needs PUBLIC_URL for image links")
	}
	pw := make([]byte, 16)
	rand.Read(pw)
	// "!" is not a valid hash scheme, so nobody can log in as the sample seller.
	var sellerID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO users (email, username, password_hash, email_verified_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (lower(email)) DO UPDATE SET updated_at = now()
		RETURNING id`, SellerEmail, SellerUsername, "!"+hex.EncodeToString(pw)).Scan(&sellerID)
	if err != nil {
		return 0, fmt.Errorf("demo seller: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO seller_profiles (user_id, stripe_account_id, charges_enabled, details_submitted, tier,
			bundle_discount_type, bundle_discount_val)
		VALUES ($1, $2, true, true, 3, 'per_extra_item', 50)
		ON CONFLICT (user_id) DO UPDATE SET charges_enabled = true, tier = 3`,
		sellerID, stripeAccount); err != nil {
		return 0, fmt.Errorf("demo seller profile: %w", err)
	}

	n := 0
	for _, c := range cards {
		img := func(side string) string {
			v := url.Values{"t": {c.title}, "s": {fmt.Sprintf("%s %d", c.set, c.year)}, "k": {c.cat}, "side": {side}}
			return fmt.Sprintf("%s/demo/card/%s.svg?%s", imageBase, c.sku, v.Encode())
		}
		sku := "DEMO-" + c.sku
		year := c.year
		in := listings.Input{
			ExternalID:  &sku,
			Category:    c.cat,
			GameOrSport: c.game,
			SetName:     c.set,
			Year:        &year,
			Title:       c.title,
			Description: "Sample listing for testing the marketplace. This card is not for sale.",
			Attributes:  c.attrs,
			PhotoURLs:   []string{img("front"), img("back")},
			PriceCents:  c.cents,
			Quantity:    c.qty,
		}
		if c.num != "" {
			num := c.num
			in.CardNumber = &num
		}
		if c.grader != "" {
			g, gr := c.grader, c.grade
			cert := fmt.Sprintf("SAMPLE%06d", n+1)
			in.Grader, in.Grade, in.CertNumber = &g, &gr, &cert
			in.PackagePreset, in.WeightOz = "bubble", 3
		} else {
			cond := c.cond
			in.Condition = &cond
		}
		// Every third card takes offers, with instant rules so testers see
		// an answer right away: 90% or more is accepted, under 60% declined.
		if n%3 == 1 {
			pct, low, kind := 90, 60, "percent"
			in.Offers = &listings.OfferSettings{Enabled: true, AcceptKind: &kind, AcceptValue: &pct, DeclineKind: &kind, DeclineValue: &low}
		}
		if _, _, err := svc.UpsertByExternalID(ctx, sellerID, in); err != nil {
			return n, fmt.Errorf("demo listing %s: %w", c.sku, err)
		}
		n++
	}
	a, err := seedAuctions(ctx, db, svc, sellerID, imageBase)
	return n + a, err
}

type auctionCard struct {
	card
	start, reserve, bin int
	days                int
	endsInHours         int
}

var auctions = []auctionCard{
	{card{"AUC-001", "tcg", "Pokemon", "Fossil", 1999, "4/62", "Gengar Holo Fossil", "LP", "", 0, []string{"holo"}, 0, 1}, 99, 0, 0, 5, 3},
	{card{"AUC-002", "sports", "Baseball", "Topps Chrome", 2018, "1", "Shohei Ohtani Rookie Refractor", "", "PSA", 9, []string{"rookie", "parallel"}, 0, 1}, 2500, 6000, 0, 7, 20},
	{card{"AUC-003", "tcg", "Magic", "Revised", 1994, "", "Serra Angel", "MP", "", 0, nil, 0, 1}, 199, 0, 1500, 7, 44},
	{card{"AUC-004", "sports", "Basketball", "Hoops", 1996, "", "Rookie Point Guard", "NM", "", 0, []string{"rookie"}, 0, 1}, 500, 0, 0, 10, 70},
	{card{"AUC-005", "tcg", "One Piece", "Romance Dawn", 2022, "OP01-001", "Roronoa Zoro Leader Parallel", "", "PSA", 10, []string{"parallel"}, 0, 1}, 4000, 9000, 15000, 7, 110},
	{card{"AUC-006", "sports", "Football", "Prizm", 2020, "307", "Rookie Quarterback Silver", "", "BGS", 9.5, []string{"rookie", "parallel"}, 0, 1}, 1000, 0, 0, 10, 200},
}

// seedAuctions keeps six sample auctions running: any that ended are
// replaced with fresh ones ending at staggered times.
func seedAuctions(ctx context.Context, db *sql.DB, svc *listings.Service, sellerID, imageBase string) (int, error) {
	n := 0
	for _, a := range auctions {
		sku := "DEMO-" + a.sku
		var live bool
		err := db.QueryRowContext(ctx, `SELECT auction_state = 'live' AND status = 'active' FROM listings
			WHERE seller_id = $1 AND external_id = $2`, sellerID, sku).Scan(&live)
		if err == nil && live {
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			return n, err
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM listings WHERE seller_id = $1 AND external_id = $2`, sellerID, sku); err != nil {
			return n, err
		}
		img := func(side string) string {
			v := url.Values{"t": {a.title}, "s": {fmt.Sprintf("%s %d", a.set, a.year)}, "k": {a.cat}, "side": {side}}
			return fmt.Sprintf("%s/demo/card/%s.svg?%s", imageBase, a.sku, v.Encode())
		}
		year := a.year
		auc := &listings.AuctionInput{StartCents: a.start, DurationDays: a.days, AutoRelist: true}
		if a.reserve > 0 {
			r := a.reserve
			auc.ReserveCents = &r
		}
		if a.bin > 0 {
			b := a.bin
			auc.BuyNowCents = &b
		}
		in := listings.Input{
			ExternalID: &sku, Category: a.cat, GameOrSport: a.game, SetName: a.set, Year: &year, Title: a.title,
			Description: "Sample auction for testing the marketplace. This card is not for sale.",
			Attributes:  a.attrs, PhotoURLs: []string{img("front"), img("back")},
			Format: listings.FormatAuction, Auction: auc, Quantity: 1,
		}
		if a.num != "" {
			num := a.num
			in.CardNumber = &num
		}
		if a.grader != "" {
			g, gr := a.grader, a.grade
			cert := fmt.Sprintf("SAMPLEA%05d", n+1)
			in.Grader, in.Grade, in.CertNumber = &g, &gr, &cert
			in.PackagePreset, in.WeightOz = "bubble", 3
		} else {
			cond := a.cond
			in.Condition = &cond
		}
		l, err := svc.Create(ctx, sellerID, in)
		if err != nil {
			return n, fmt.Errorf("demo auction %s: %w", a.sku, err)
		}
		// Stagger the end times so "ending soonest" has something to show.
		if _, err := db.ExecContext(ctx, `UPDATE listings SET ends_at = now() + make_interval(hours => $2)
			WHERE id = $1`, l.ID, a.endsInHours); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Remove deletes the sample seller and all sample listings.
func Remove(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM users WHERE lower(email) = lower($1)`, SellerEmail)
	return err
}
