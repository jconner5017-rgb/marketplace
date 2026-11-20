package deals

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
)

// A seller's coupons and order discounts (Marketing > Discounts).

type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

var ErrNotFound = errors.New("not found")

const (
	maxCoupons = 50 // switched-on coupons per seller
	maxRules   = 10 // switched-on order discounts per seller
)

var codeRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,19}$`)

// CouponInput is a new coupon.
type CouponInput struct {
	Code         string     `json:"code"`
	Label        string     `json:"label"`
	Percent      int        `json:"percent"`      // % off, or
	AmountCents  int        `json:"amount_cents"` // $ off the cards it covers
	Games        []string   `json:"games"`        // empty = every card
	StartsAt     *time.Time `json:"starts_at"`    // nil = now
	EndsAt       *time.Time `json:"ends_at"`      // nil = no end
	OncePerBuyer *bool      `json:"once_per_buyer"`
}

func cleanGames(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, g := range in {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if utf8.RuneCountInString(g) > 40 {
			return nil, &FieldError{"games", "that category name is too long"}
		}
		if k := strings.ToLower(g); !seen[k] {
			seen[k] = true
			out = append(out, g)
		}
	}
	if len(out) > 30 {
		return nil, &FieldError{"games", "choose up to 30 categories, or all of them"}
	}
	return out, nil
}

func nullArray(g []string) any {
	if len(g) == 0 {
		return nil
	}
	return pq.Array(g)
}

// CreateCoupon adds a coupon for the seller's buyers.
func CreateCoupon(ctx context.Context, db *sql.DB, sellerID string, in CouponInput, now time.Time) (Coupon, error) {
	in.Code = NormalizeCode(in.Code)
	in.Label = strings.TrimSpace(in.Label)
	switch {
	case !codeRE.MatchString(in.Code):
		return Coupon{}, &FieldError{"code", "use 3 to 20 letters, numbers, - or _ (like SAVE10)"}
	case utf8.RuneCountInString(in.Label) > 60:
		return Coupon{}, &FieldError{"label", "keep the name to 60 characters"}
	case in.Percent > 0 && in.AmountCents > 0:
		return Coupon{}, &FieldError{"percent", "choose % off or $ off, not both"}
	case in.AmountCents > 0 && (in.AmountCents < 50 || in.AmountCents > 50000):
		return Coupon{}, &FieldError{"amount_cents", "choose $0.50 to $500 off"}
	case in.AmountCents == 0 && (in.Percent < 5 || in.Percent > 50):
		return Coupon{}, &FieldError{"percent", "choose 5% to 50% off"}
	}
	games, err := cleanGames(in.Games)
	if err != nil {
		return Coupon{}, err
	}
	start := now
	if in.StartsAt != nil && in.StartsAt.After(now) {
		start = *in.StartsAt
	}
	if in.EndsAt != nil && !in.EndsAt.After(start) {
		return Coupon{}, &FieldError{"ends_at", "end after it starts"}
	}
	once := true
	if in.OncePerBuyer != nil {
		once = *in.OncePerBuyer
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM seller_coupons WHERE seller_id = $1 AND active`, sellerID).Scan(&n); err != nil {
		return Coupon{}, err
	}
	if n >= maxCoupons {
		return Coupon{}, &FieldError{"code", "you have 50 coupons switched on; switch some off first"}
	}
	var id int
	err = db.QueryRowContext(ctx, `INSERT INTO seller_coupons (seller_id, code, label, percent, amount_cents, games, starts_at, ends_at, once_per_buyer)
		VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, 0), $6, $7, $8, $9) RETURNING id`,
		sellerID, in.Code, in.Label, in.Percent, in.AmountCents, nullArray(games), start, in.EndsAt, once).Scan(&id)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return Coupon{}, &FieldError{"code", "you already have a coupon with that code"}
	}
	if err != nil {
		return Coupon{}, err
	}
	cs, err := Coupons(ctx, db, sellerID, id)
	if err != nil || len(cs) == 0 {
		return Coupon{}, err
	}
	return cs[0], nil
}

// Coupons lists the seller's coupons (one when id > 0), newest first, with
// how many paid orders used each.
func Coupons(ctx context.Context, db *sql.DB, sellerID string, id int) ([]Coupon, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+couponCols+`,
			(SELECT count(*) FROM coupon_redemptions r JOIN orders o ON o.id = r.order_id
			 WHERE r.coupon_id = c.id AND o.status NOT IN ('pending_payment', 'cancelled')),
			(SELECT coalesce(sum(r.cents), 0) FROM coupon_redemptions r JOIN orders o ON o.id = r.order_id
			 WHERE r.coupon_id = c.id AND o.status NOT IN ('pending_payment', 'cancelled'))
		FROM seller_coupons c WHERE c.seller_id = $1 AND ($2 = 0 OR c.id = $2)
		ORDER BY c.active DESC, c.created_at DESC LIMIT 200`, sellerID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Coupon{}
	for rows.Next() {
		var c Coupon
		if err := scanCoupon(rows, &c, &c.Uses, &c.DiscountedC); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCouponActive switches a coupon on or off.
func SetCouponActive(ctx context.Context, db *sql.DB, sellerID string, id int, on bool) error {
	res, err := db.ExecContext(ctx, `UPDATE seller_coupons SET active = $3 WHERE id = $1 AND seller_id = $2`, id, sellerID, on)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RuleInput is a new order discount.
type RuleInput struct {
	MinCards      int  `json:"min_cards"`
	PriceMinCents *int `json:"price_min_cents"`
	PriceMaxCents *int `json:"price_max_cents"`
	Percent       int  `json:"percent"`      // whole % off, or
	AmountCents   int  `json:"amount_cents"` // $ off the qualifying cards
}

// CreateRule adds an order discount.
func CreateRule(ctx context.Context, db *sql.DB, sellerID string, in RuleInput) (Rule, error) {
	switch {
	case in.MinCards < 2 || in.MinCards > 500:
		return Rule{}, &FieldError{"min_cards", "choose 2 to 500 cards"}
	case in.Percent > 0 && in.AmountCents > 0:
		return Rule{}, &FieldError{"percent", "choose % off or $ off, not both"}
	case in.AmountCents > 0 && in.AmountCents > 50000:
		return Rule{}, &FieldError{"amount_cents", "choose up to $500 off"}
	case in.AmountCents == 0 && (in.Percent < 1 || in.Percent > 50):
		return Rule{}, &FieldError{"percent", "choose 1% to 50% off"}
	case in.PriceMinCents != nil && *in.PriceMinCents < 0, in.PriceMaxCents != nil && *in.PriceMaxCents <= 0:
		return Rule{}, &FieldError{"price_max_cents", "enter prices above $0"}
	case in.PriceMinCents != nil && in.PriceMaxCents != nil && *in.PriceMaxCents < *in.PriceMinCents:
		return Rule{}, &FieldError{"price_max_cents", "the top price is below the bottom price"}
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM order_discounts WHERE seller_id = $1 AND active`, sellerID).Scan(&n); err != nil {
		return Rule{}, err
	}
	if n >= maxRules {
		return Rule{}, &FieldError{"min_cards", "you have 10 order discounts; remove one first"}
	}
	var bp any
	if in.Percent > 0 {
		bp = in.Percent * 100
	}
	var amt any
	if in.AmountCents > 0 {
		amt = in.AmountCents
	}
	r, err := scanRule(db.QueryRowContext(ctx, `INSERT INTO order_discounts (seller_id, min_cards, price_min_cents, price_max_cents, percent_bp, amount_cents)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+ruleCols, sellerID, in.MinCards, in.PriceMinCents, in.PriceMaxCents, bp, amt))
	return r, err
}

// Rules lists the seller's switched-on order discounts.
func Rules(ctx context.Context, db *sql.DB, sellerID string) ([]Rule, error) {
	return ActiveRules(ctx, db, sellerID)
}

// RemoveRule switches an order discount off (orders that used it keep its label).
func RemoveRule(ctx context.Context, db *sql.DB, sellerID string, id int) error {
	res, err := db.ExecContext(ctx, `UPDATE order_discounts SET active = false WHERE id = $1 AND seller_id = $2 AND active`, id, sellerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
