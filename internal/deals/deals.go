// Package deals prices one seller's part of an order (2026-11-20). A buyer
// gets the single best deal of three: the store's sale prices, a coupon they
// typed in, or the seller's best order discount. The seller pays for the
// discount; the site's fee is worked out on what the buyer actually pays
// (see Allocate). Auction wins and accepted offers aren't discounted.
package deals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Item is one cart line.
type Item struct {
	ListingID string
	Game      string // the listing's sport or game (game_or_sport)
	Full      int    // list price, cents
	Sale      int    // sale price now (== Full when there's no sale)
	Qty       int
}

// Coupon is a seller's coupon code.
type Coupon struct {
	ID           int        `json:"id"`
	Code         string     `json:"code"`
	Label        string     `json:"label"`
	Percent      *int       `json:"percent,omitempty"`
	AmountCents  *int       `json:"amount_cents,omitempty"`
	Games        []string   `json:"games"` // empty = every card
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at,omitempty"`
	OncePerBuyer bool       `json:"once_per_buyer"`
	Active       bool       `json:"active"`
	CreatedAt    time.Time  `json:"created_at"`
	Uses         int        `json:"uses"`           // paid orders that used it
	DiscountedC  int        `json:"discount_cents"` // total given on those orders
}

// Rule is one of a seller's order discounts.
type Rule struct {
	ID            int       `json:"id"`
	MinCards      int       `json:"min_cards"`
	PriceMinCents *int      `json:"price_min_cents,omitempty"`
	PriceMaxCents *int      `json:"price_max_cents,omitempty"`
	PercentBP     *int      `json:"percent_bp,omitempty"`
	AmountCents   *int      `json:"amount_cents,omitempty"`
	PerCard       bool      `json:"per_card"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	Label         string    `json:"label"`
}

// Deal is what the buyer gets.
type Deal struct {
	Kind  string `json:"kind,omitempty"`  // "sale", "coupon", "order" or "" (none)
	Label string `json:"label,omitempty"` // shown on the cart, order and receipts
	Cents int    `json:"cents"`           // savings against list prices
	// CouponID when Kind is coupon; CouponNote explains a typed coupon that
	// wasn't used (another deal saves more, or it doesn't fit these cards).
	CouponID   int    `json:"-"`
	CouponNote string `json:"coupon_note,omitempty"`
	// eligible[i]: the item shares in the discount (coupon and order deals).
	eligible []bool
}

func money(c int) string {
	if c%100 == 0 {
		return fmt.Sprintf("$%d", c/100)
	}
	return fmt.Sprintf("$%d.%02d", c/100, c%100)
}

func pct(bp int) string {
	if bp%100 == 0 {
		return fmt.Sprintf("%d%%", bp/100)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(bp)/100), "0"), ".") + "%"
}

// Describe is a rule in words: "25% off 20+ cards under $5".
func (r Rule) Describe() string {
	what := "off"
	switch {
	case r.PercentBP != nil:
		what = pct(*r.PercentBP) + " off"
	case r.PerCard:
		what = money(*r.AmountCents) + " off each extra card"
	default:
		what = money(*r.AmountCents) + " off"
	}
	cards := fmt.Sprintf("%d+ cards", r.MinCards)
	switch {
	case r.PriceMinCents != nil && r.PriceMaxCents != nil:
		cards += fmt.Sprintf(" priced %s to %s", money(*r.PriceMinCents), money(*r.PriceMaxCents))
	case r.PriceMaxCents != nil:
		cards += " up to " + money(*r.PriceMaxCents)
	case r.PriceMinCents != nil:
		cards += " from " + money(*r.PriceMinCents)
	}
	if r.PerCard {
		return what + " (" + cards + ")"
	}
	return what + " " + cards
}

// Describe is a coupon in words: "COLLECT10: 10% off Baseball cards".
func (c Coupon) Describe() string {
	off := ""
	if c.Percent != nil {
		off = fmt.Sprintf("%d%% off", *c.Percent)
	} else {
		off = money(*c.AmountCents) + " off"
	}
	if len(c.Games) > 0 {
		off += " " + strings.Join(c.Games, ", ") + " cards"
	}
	return c.Code + ": " + off
}

func (c Coupon) covers(game string) bool {
	if len(c.Games) == 0 {
		return true
	}
	for _, g := range c.Games {
		if strings.EqualFold(g, game) {
			return true
		}
	}
	return false
}

// halfCap: a discount never exceeds half of what it applies to, so a
// mistake can't give cards away (the same rule the old multi-card discount had).
func halfCap(d, base int) int { return min(d, base/2) }

// orderSavings is a rule's discount on these items (0 if they don't qualify).
func (r Rule) savings(items []Item) (int, []bool) {
	elig := make([]bool, len(items))
	count, base := 0, 0
	for i, it := range items {
		if r.PriceMinCents != nil && it.Full < *r.PriceMinCents {
			continue
		}
		if r.PriceMaxCents != nil && it.Full > *r.PriceMaxCents {
			continue
		}
		elig[i] = true
		count += it.Qty
		base += it.Full * it.Qty
	}
	if count < r.MinCards || base <= 0 {
		return 0, nil
	}
	d := 0
	switch {
	case r.PercentBP != nil:
		d = base * *r.PercentBP / 10000
	case r.PerCard:
		d = *r.AmountCents * (count - 1)
	default:
		d = *r.AmountCents
	}
	return halfCap(d, base), elig
}

func (c Coupon) savings(items []Item) (int, []bool) {
	elig := make([]bool, len(items))
	base := 0
	for i, it := range items {
		if c.covers(it.Game) {
			elig[i] = true
			base += it.Full * it.Qty
		}
	}
	if base <= 0 {
		return 0, nil
	}
	d := 0
	if c.Percent != nil {
		d = base * *c.Percent / 100
	} else {
		d = *c.AmountCents
	}
	return halfCap(d, base), elig
}

// Best picks the single deal that saves the buyer the most. Ties go to the
// sale (nothing to use up), then the order discount, then the coupon.
func Best(items []Item, coupon *Coupon, rules []Rule) Deal {
	best := Deal{}
	sale := 0
	for _, it := range items {
		sale += (it.Full - it.Sale) * it.Qty
	}
	if sale > 0 {
		best = Deal{Kind: "sale", Label: "Sale price", Cents: sale}
	}
	for _, r := range rules {
		if d, elig := r.savings(items); d > best.Cents {
			best = Deal{Kind: "order", Label: r.Describe(), Cents: d, eligible: elig}
		}
	}
	if coupon != nil {
		d, elig := coupon.savings(items)
		switch {
		case d == 0:
			best.CouponNote = coupon.Code + " doesn't apply to these cards."
		case d > best.Cents:
			note := best.CouponNote
			best = Deal{Kind: "coupon", Label: coupon.Describe(), Cents: d, CouponID: coupon.ID, eligible: elig, CouponNote: note}
		default:
			best.CouponNote = fmt.Sprintf("%s saves %s, but you're already getting a better deal (%s).", coupon.Code, money(d), strings.ToLower(best.Label))
		}
	}
	return best
}

// Prices are what to charge for each item given the deal: sale prices for a
// sale, list prices otherwise. Subtotal is their total; the order discount
// (DiscountCents) is the deal's savings unless it's a sale (already in the prices).
func (d Deal) Prices(items []Item) (units []int, subtotal, discount int) {
	units = make([]int, len(items))
	for i, it := range items {
		units[i] = it.Full
		if d.Kind == "sale" {
			units[i] = it.Sale
		}
		subtotal += units[i] * it.Qty
	}
	if d.Kind == "coupon" || d.Kind == "order" {
		discount = d.Cents
	}
	return units, subtotal, discount
}

// Allocate spreads a coupon or order discount over the items it applied to,
// in proportion to their price, and returns what the buyer pays for one of
// each item - the price the site's fee is worked out on.
func (d Deal) Allocate(items []Item) []int {
	units, _, discount := d.Prices(items)
	paid := append([]int(nil), units...)
	if discount <= 0 || d.eligible == nil {
		return paid
	}
	base := 0
	for i, it := range items {
		if d.eligible[i] {
			base += it.Full * it.Qty
		}
	}
	if base <= 0 {
		return paid
	}
	left := discount
	last := -1
	for i, it := range items {
		if !d.eligible[i] {
			continue
		}
		share := discount * it.Full * it.Qty / base
		per := share / it.Qty
		paid[i] = units[i] - per
		left -= per * it.Qty
		last = i
	}
	// Rounding leftovers come off the last eligible item.
	if last >= 0 && left > 0 && items[last].Qty > 0 {
		paid[last] = max(paid[last]-left/items[last].Qty, 0)
	}
	return paid
}

// ---------------- loading from the database ----------------

var (
	ErrNoCoupon   = errors.New("that coupon code isn't valid for this seller")
	ErrUsedCoupon = errors.New("you've already used that coupon")
	ErrOldCoupon  = errors.New("that coupon has expired")
	ErrSoonCoupon = errors.New("that coupon isn't active yet")
)

// NormalizeCode is how codes are stored and compared.
func NormalizeCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

const couponCols = `c.id, c.code, c.label, c.percent, c.amount_cents, coalesce(c.games, '{}'), c.starts_at, c.ends_at, c.once_per_buyer, c.active, c.created_at`

func scanCoupon(sc interface{ Scan(...any) error }, c *Coupon, extra ...any) error {
	var games []string
	err := sc.Scan(append([]any{&c.ID, &c.Code, &c.Label, &c.Percent, &c.AmountCents, pq.Array(&games), &c.StartsAt, &c.EndsAt,
		&c.OncePerBuyer, &c.Active, &c.CreatedAt}, extra...)...)
	c.Games = games
	if c.Games == nil {
		c.Games = []string{}
	}
	return err
}

// FindCoupon returns the seller's coupon if this buyer may use it now. With
// lock, the coupon row is locked for the rest of the transaction (checkout),
// so two orders can't both use a once-per-buyer code.
func FindCoupon(ctx context.Context, db DBTX, sellerID, buyerID, code string, now time.Time, lock bool) (*Coupon, error) {
	code = NormalizeCode(code)
	if code == "" {
		return nil, nil
	}
	q := `SELECT ` + couponCols + ` FROM seller_coupons c WHERE c.seller_id = $1 AND c.code = $2`
	if lock {
		q += ` FOR UPDATE`
	}
	var c Coupon
	if err := scanCoupon(db.QueryRowContext(ctx, q, sellerID, code), &c); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoCoupon
		}
		return nil, err
	}
	switch {
	case !c.Active:
		return nil, ErrNoCoupon
	case now.Before(c.StartsAt):
		return nil, ErrSoonCoupon
	case c.EndsAt != nil && !now.Before(*c.EndsAt):
		return nil, ErrOldCoupon
	}
	if c.OncePerBuyer {
		var used bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM coupon_redemptions r JOIN orders o ON o.id = r.order_id
			WHERE r.coupon_id = $1 AND r.buyer_id = $2 AND o.status <> 'cancelled')`, c.ID, buyerID).Scan(&used); err != nil {
			return nil, err
		}
		if used {
			return nil, ErrUsedCoupon
		}
	}
	return &c, nil
}

const ruleCols = `id, min_cards, price_min_cents, price_max_cents, percent_bp, amount_cents, per_card, active, created_at`

func scanRule(sc interface{ Scan(...any) error }) (Rule, error) {
	var r Rule
	err := sc.Scan(&r.ID, &r.MinCards, &r.PriceMinCents, &r.PriceMaxCents, &r.PercentBP, &r.AmountCents, &r.PerCard, &r.Active, &r.CreatedAt)
	if err == nil {
		r.Label = r.Describe()
	}
	return r, err
}

// ActiveRules are the seller's order discounts that are switched on.
func ActiveRules(ctx context.Context, db DBTX, sellerID string) ([]Rule, error) {
	return rules(ctx, db, `WHERE seller_id = $1 AND active ORDER BY id`, sellerID)
}

func rules(ctx context.Context, db DBTX, where string, args ...any) ([]Rule, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+ruleCols+` FROM order_discounts `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordCoupon notes that an order used a coupon.
func RecordCoupon(ctx context.Context, db DBTX, orderID, buyerID string, d Deal) error {
	if d.Kind != "coupon" || d.CouponID == 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `INSERT INTO coupon_redemptions (order_id, coupon_id, buyer_id, cents) VALUES ($1, $2, $3, $4)`,
		orderID, d.CouponID, buyerID, d.Cents)
	return err
}
