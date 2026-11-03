// Package promos runs promo codes that give sellers a number of sales with
// no marketplace fee. Staff create codes in the back office; a seller
// redeems one and each card they sell uses one fee-free sale until they run
// out or the redemption expires.
package promos

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DBTX is a *sql.DB or *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

type Code struct {
	ID             int        `json:"id"`
	Code           string     `json:"code"`
	Label          string     `json:"label"`
	Audience       string     `json:"audience"` // any, individual, business
	FeeFreeSales   int        `json:"fee_free_sales"`
	MaxRedemptions *int       `json:"max_redemptions"`
	NewSellersOnly bool       `json:"new_sellers_only"`
	ExpiresAt      *time.Time `json:"expires_at"`
	ValidDays      *int       `json:"valid_days"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"created_at"`
	// Filled in by List.
	Redemptions int `json:"redemptions"`
	SalesUsed   int `json:"sales_used"`
}

type Redemption struct {
	ID           int        `json:"id"`
	Code         string     `json:"code"`
	SalesGranted int        `json:"sales_granted"`
	SalesUsed    int        `json:"sales_used"`
	ExpiresAt    *time.Time `json:"expires_at"`
	RedeemedAt   time.Time  `json:"redeemed_at"`
	// For staff.
	Seller string `json:"seller,omitempty"`
}

// Remaining is how many fee-free sales are left on a redemption now.
func (r Redemption) Remaining(now time.Time) int {
	if r.ExpiresAt != nil && !now.Before(*r.ExpiresAt) {
		return 0
	}
	return max(r.SalesGranted-r.SalesUsed, 0)
}

type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

var (
	ErrNotFound  = errors.New("that promo code isn't valid")
	ErrUsed      = errors.New("you've already used this promo code")
	ErrExpired   = errors.New("this promo code has expired")
	ErrFull      = errors.New("this promo code has been fully redeemed")
	ErrNotNew    = errors.New("this promo code is for new sellers only")
	ErrDuplicate = &FieldError{"code", "a promo code with that name already exists"}
)

var codeRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,31}$`)

// Normalize upper-cases a code and trims spaces.
func Normalize(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

type Service struct {
	DB  *sql.DB
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func validate(c *Code) error {
	c.Code = Normalize(c.Code)
	c.Label = strings.TrimSpace(c.Label)
	if !codeRE.MatchString(c.Code) {
		return &FieldError{"code", "use 3 to 32 letters, numbers, dashes or underscores"}
	}
	if c.Audience == "" {
		c.Audience = "any"
	}
	if c.Audience != "any" && c.Audience != "individual" && c.Audience != "business" {
		return &FieldError{"audience", "choose any, individual or business"}
	}
	if c.FeeFreeSales < 1 || c.FeeFreeSales > 100000 {
		return &FieldError{"fee_free_sales", "fee-free sales must be 1 to 100,000"}
	}
	if c.MaxRedemptions != nil && *c.MaxRedemptions < 1 {
		return &FieldError{"max_redemptions", "leave blank for unlimited, or at least 1"}
	}
	if c.ValidDays != nil && (*c.ValidDays < 1 || *c.ValidDays > 3650) {
		return &FieldError{"valid_days", "leave blank for no limit, or 1 to 3,650 days"}
	}
	if len(c.Label) > 200 {
		return &FieldError{"label", "keep the label under 200 characters"}
	}
	return nil
}

const codeCols = `id, code, label, audience, fee_free_sales, max_redemptions, new_sellers_only, expires_at, valid_days, active, created_at`

func scanCode(sc interface{ Scan(...any) error }, c *Code, extra ...any) error {
	var maxR, days sql.NullInt64
	var exp sql.NullTime
	dest := append([]any{&c.ID, &c.Code, &c.Label, &c.Audience, &c.FeeFreeSales, &maxR, &c.NewSellersOnly, &exp, &days, &c.Active, &c.CreatedAt}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return err
	}
	if maxR.Valid {
		v := int(maxR.Int64)
		c.MaxRedemptions = &v
	}
	if days.Valid {
		v := int(days.Int64)
		c.ValidDays = &v
	}
	if exp.Valid {
		t := exp.Time
		c.ExpiresAt = &t
	}
	return nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "promo_codes_code")
}

// Create adds a code.
func (s *Service) Create(ctx context.Context, tx DBTX, c Code, staffID string) (Code, error) {
	if err := validate(&c); err != nil {
		return c, err
	}
	err := scanCode(tx.QueryRowContext(ctx, `INSERT INTO promo_codes (code, label, audience, fee_free_sales, max_redemptions, new_sellers_only, expires_at, valid_days, active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+codeCols,
		c.Code, c.Label, c.Audience, c.FeeFreeSales, c.MaxRedemptions, c.NewSellersOnly, c.ExpiresAt, c.ValidDays, c.Active, staffID), &c)
	if isUnique(err) {
		return c, ErrDuplicate
	}
	return c, err
}

// Update changes a code. Sellers who already redeemed it keep what they got.
func (s *Service) Update(ctx context.Context, tx DBTX, c Code) (Code, error) {
	if err := validate(&c); err != nil {
		return c, err
	}
	err := scanCode(tx.QueryRowContext(ctx, `UPDATE promo_codes SET code = $2, label = $3, audience = $4, fee_free_sales = $5,
		max_redemptions = $6, new_sellers_only = $7, expires_at = $8, valid_days = $9, active = $10, updated_at = now()
		WHERE id = $1 RETURNING `+codeCols,
		c.ID, c.Code, c.Label, c.Audience, c.FeeFreeSales, c.MaxRedemptions, c.NewSellersOnly, c.ExpiresAt, c.ValidDays, c.Active), &c)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if isUnique(err) {
		return c, ErrDuplicate
	}
	return c, err
}

// Get returns one code.
func (s *Service) Get(ctx context.Context, id int) (Code, error) {
	var c Code
	err := scanCode(s.DB.QueryRowContext(ctx, `SELECT `+codeCols+` FROM promo_codes WHERE id = $1`, id), &c)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// List returns every code, newest first, with how much each has been used.
func (s *Service) List(ctx context.Context) ([]Code, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+prefixed("pc", codeCols)+`,
		(SELECT count(*) FROM promo_redemptions r WHERE r.code_id = pc.id),
		(SELECT coalesce(sum(r.sales_used), 0) FROM promo_redemptions r WHERE r.code_id = pc.id)
		FROM promo_codes pc ORDER BY pc.created_at DESC, pc.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Code{}
	for rows.Next() {
		var c Code
		if err := scanCode(rows, &c, &c.Redemptions, &c.SalesUsed); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func prefixed(p, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = p + "." + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}

// Redemptions lists who redeemed a code.
func (s *Service) Redemptions(ctx context.Context, codeID int) ([]Redemption, error) {
	return s.redemptions(ctx, `WHERE r.code_id = $1`, codeID)
}

// ForSeller lists a seller's redemptions, newest first.
func (s *Service) ForSeller(ctx context.Context, sellerID string) ([]Redemption, error) {
	return s.redemptions(ctx, `WHERE r.seller_id = $1`, sellerID)
}

func (s *Service) redemptions(ctx context.Context, where string, arg any) ([]Redemption, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id, pc.code, r.sales_granted, r.sales_used, r.expires_at, r.redeemed_at, u.username
		FROM promo_redemptions r JOIN promo_codes pc ON pc.id = r.code_id JOIN users u ON u.id = r.seller_id `+where+`
		ORDER BY r.redeemed_at DESC, r.id DESC`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Redemption{}
	for rows.Next() {
		var r Redemption
		var exp sql.NullTime
		if err := rows.Scan(&r.ID, &r.Code, &r.SalesGranted, &r.SalesUsed, &exp, &r.RedeemedAt, &r.Seller); err != nil {
			return nil, err
		}
		if exp.Valid {
			t := exp.Time
			r.ExpiresAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Remaining is the seller's fee-free sales left across all redemptions.
func (s *Service) Remaining(ctx context.Context, sellerID string) (int, error) {
	rs, err := s.ForSeller(ctx, sellerID)
	if err != nil {
		return 0, err
	}
	n, now := 0, s.now()
	for _, r := range rs {
		n += r.Remaining(now)
	}
	return n, nil
}

// Redeem applies a code to a seller's account.
func (s *Service) Redeem(ctx context.Context, sellerID, code string) (Redemption, error) {
	code = Normalize(code)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Redemption{}, err
	}
	defer tx.Rollback()
	var c Code
	err = scanCode(tx.QueryRowContext(ctx, `SELECT `+codeCols+` FROM promo_codes WHERE upper(code) = $1 FOR UPDATE`, code), &c)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !c.Active) {
		return Redemption{}, ErrNotFound
	}
	if err != nil {
		return Redemption{}, err
	}
	now := s.now()
	if c.ExpiresAt != nil && !now.Before(*c.ExpiresAt) {
		return Redemption{}, ErrExpired
	}
	var used, total int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE seller_id = $2), count(*) FROM promo_redemptions WHERE code_id = $1`,
		c.ID, sellerID).Scan(&used, &total); err != nil {
		return Redemption{}, err
	}
	if used > 0 {
		return Redemption{}, ErrUsed
	}
	if c.MaxRedemptions != nil && total >= *c.MaxRedemptions {
		return Redemption{}, ErrFull
	}
	if c.NewSellersOnly {
		var sold bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM orders WHERE seller_id = $1 AND paid_at IS NOT NULL AND NOT test)`,
			sellerID).Scan(&sold); err != nil {
			return Redemption{}, err
		}
		if sold {
			return Redemption{}, ErrNotNew
		}
	}
	r := Redemption{Code: c.Code, SalesGranted: c.FeeFreeSales, RedeemedAt: now}
	if c.ValidDays != nil {
		t := now.Add(time.Duration(*c.ValidDays) * 24 * time.Hour)
		r.ExpiresAt = &t
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO promo_redemptions (code_id, seller_id, sales_granted, expires_at, redeemed_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, c.ID, sellerID, r.SalesGranted, r.ExpiresAt, now).Scan(&r.ID); err != nil {
		return Redemption{}, err
	}
	return r, tx.Commit()
}

// Waiver is what Reserve took off an order's fee.
type Waiver struct {
	Cards    int
	FeeCents int
	uses     map[int]int // redemption id -> cards
}

// Reserve uses up to len(unitFees) of the seller's fee-free sales for an
// order, inside the order's transaction. unitFees is the marketplace fee of
// each card in the order; the largest fees are waived first. It records
// which redemptions were used so Release can give them back.
func Reserve(ctx context.Context, tx DBTX, now time.Time, sellerID string, unitFees []int) (Waiver, error) {
	w := Waiver{uses: map[int]int{}}
	if len(unitFees) == 0 {
		return w, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, sales_granted - sales_used FROM promo_redemptions
		WHERE seller_id = $1 AND sales_used < sales_granted AND (expires_at IS NULL OR expires_at > $2)
		ORDER BY expires_at NULLS LAST, id FOR UPDATE`, sellerID, now)
	if err != nil {
		return w, err
	}
	type avail struct{ id, left int }
	var as []avail
	for rows.Next() {
		var a avail
		if err := rows.Scan(&a.id, &a.left); err != nil {
			rows.Close()
			return w, err
		}
		as = append(as, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return w, err
	}
	fees := append([]int(nil), unitFees...)
	sort.Sort(sort.Reverse(sort.IntSlice(fees)))
	for _, f := range fees {
		for len(as) > 0 && as[0].left == 0 {
			as = as[1:]
		}
		if len(as) == 0 {
			break
		}
		as[0].left--
		w.uses[as[0].id]++
		w.Cards++
		w.FeeCents += f
	}
	for id, n := range w.uses {
		if _, err := tx.ExecContext(ctx, `UPDATE promo_redemptions SET sales_used = sales_used + $2 WHERE id = $1`, id, n); err != nil {
			return w, err
		}
	}
	return w, nil
}

// Record links a Reserve to its order.
func (w Waiver) Record(ctx context.Context, tx DBTX, orderID string) error {
	for id, n := range w.uses {
		if _, err := tx.ExecContext(ctx, `INSERT INTO order_promo_uses (order_id, redemption_id, cards) VALUES ($1, $2, $3)`,
			orderID, id, n); err != nil {
			return err
		}
	}
	if w.FeeCents > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET fee_waived_cents = $2 WHERE id = $1`, orderID, w.FeeCents); err != nil {
			return err
		}
	}
	return nil
}

// Release gives an unpaid order's fee-free sales back.
func Release(ctx context.Context, tx DBTX, orderID string) error {
	_, err := tx.ExecContext(ctx, `
		WITH back AS (
			UPDATE order_promo_uses SET returned = true WHERE order_id = $1 AND NOT returned RETURNING redemption_id, cards)
		UPDATE promo_redemptions r SET sales_used = greatest(r.sales_used - back.cards, 0)
		FROM back WHERE r.id = back.redemption_id`, orderID)
	return err
}
