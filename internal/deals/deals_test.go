package deals

import "testing"

func ip(v int) *int { return &v }

func cards(n, cents int, game string) Item {
	return Item{ListingID: "x", Game: game, Full: cents, Sale: cents, Qty: n}
}

// "At least 20 cards under $5 get 25% off": only the cheap cards count and get the discount.
func TestOrderRule(t *testing.T) {
	r := Rule{MinCards: 20, PriceMaxCents: ip(500), PercentBP: ip(2500)}
	if r.Describe() != "25% off 20+ cards up to $5" {
		t.Fatalf("label %q", r.Describe())
	}
	items := []Item{cards(20, 300, "Baseball"), cards(2, 1000, "Baseball")}
	if d, _ := r.savings(items); d != 1500 { // 25% of 20 x $3
		t.Fatalf("20 cheap cards: %d", d)
	}
	if d, _ := r.savings([]Item{cards(19, 300, "Baseball"), cards(5, 1000, "Baseball")}); d != 0 {
		t.Fatalf("19 cheap cards still qualified: %d", d)
	}
	// A price range.
	rr := Rule{MinCards: 3, PriceMinCents: ip(1000), PriceMaxCents: ip(2500), AmountCents: ip(500)}
	if d, _ := rr.savings([]Item{cards(3, 1500, "Football"), cards(10, 200, "Football")}); d != 500 {
		t.Fatalf("$5 off 3 cards $10-$25: %d", d)
	}
	// Never more than half: $5 off two $2 cards is capped at $2.
	if d, _ := (Rule{MinCards: 2, AmountCents: ip(500)}).savings([]Item{cards(2, 200, "Soccer")}); d != 200 {
		t.Fatalf("half cap: %d", d)
	}
	// The old "$0.50 off each extra card".
	old := Rule{MinCards: 2, AmountCents: ip(50), PerCard: true}
	if d, _ := old.savings([]Item{cards(4, 500, "Hockey")}); d != 150 {
		t.Fatalf("per extra card: %d", d)
	}
}

func TestBestSingleDeal(t *testing.T) {
	sale := []Item{{Game: "Baseball", Full: 1000, Sale: 800, Qty: 3}, {Game: "Pokemon", Full: 400, Sale: 400, Qty: 5}}
	rule := Rule{MinCards: 5, PercentBP: ip(1000)} // 10% off 5+ cards: 10% of $50 = $5
	pc := ip(20)
	coupon := &Coupon{ID: 7, Code: "BASE20", Percent: pc, Games: []string{"baseball"}} // 20% of $30 = $6

	d := Best(sale, nil, []Rule{rule})
	if d.Kind != "sale" || d.Cents != 600 { // sale saves $6, beats $5
		t.Fatalf("sale vs order: %+v", d)
	}
	d = Best(sale, coupon, []Rule{rule})
	if d.Kind != "sale" || d.CouponNote == "" { // coupon ties the sale: the sale wins, coupon kept
		t.Fatalf("tie: %+v", d)
	}
	big := &Coupon{ID: 8, Code: "BIG", AmountCents: ip(1500)}
	d = Best(sale, big, []Rule{rule})
	if d.Kind != "coupon" || d.Cents != 1500 || d.CouponID != 8 {
		t.Fatalf("coupon wins: %+v", d)
	}
	units, sub, disc := d.Prices(sale)
	if units[0] != 1000 || sub != 5000 || disc != 1500 { // list prices + the coupon, not the sale too
		t.Fatalf("prices with coupon: %v %d %d", units, sub, disc)
	}
	// A coupon for other cards says so.
	d = Best([]Item{cards(2, 500, "Pokemon")}, coupon, nil)
	if d.Kind != "" || d.CouponNote != "BASE20 doesn't apply to these cards." {
		t.Fatalf("wrong cards: %+v", d)
	}
}

// The fee is on what the buyer pays: the discount comes off the cards it applied to.
func TestAllocate(t *testing.T) {
	items := []Item{cards(20, 300, "Baseball"), cards(2, 1000, "Baseball")}
	d := Best(items, nil, []Rule{{MinCards: 20, PriceMaxCents: ip(500), PercentBP: ip(2500)}})
	paid := d.Allocate(items)
	if paid[0] != 225 || paid[1] != 1000 {
		t.Fatalf("paid per card: %v", paid)
	}
	total := 0
	for i, it := range items {
		total += paid[i] * it.Qty
	}
	if total != 8000-1500 {
		t.Fatalf("paid total %d", total)
	}
	// Uneven amounts still add up to the discount.
	items = []Item{cards(3, 333, "Golf"), cards(1, 1001, "Golf")}
	d = Best(items, &Coupon{ID: 1, Code: "X", AmountCents: ip(700)}, nil)
	paid = d.Allocate(items)
	sum := 0
	for i, it := range items {
		sum += (it.Full - paid[i]) * it.Qty
	}
	if sum < 690 || sum > 700 {
		t.Fatalf("allocated %d of 700: %v", sum, paid)
	}
}
