package admin

import (
	"context"
	"database/sql"
	"time"
)

// KPIs are the platform's headline numbers for a period.
type KPIs struct {
	Days           int      `json:"days"`
	GMVCents       int64    `json:"gmv_cents"`          // paid orders, before refunds
	FeeCents       int64    `json:"fee_cents"`          // our fees on paid orders
	RefundedFees   int64    `json:"refunded_fee_cents"` // fees on fully refunded orders
	NetFeeCents    int64    `json:"net_fee_cents"`
	Orders         int64    `json:"orders"`
	AOVCents       int64    `json:"aov_cents"`
	ActiveUsers    int64    `json:"active_users"` // signed in during the period
	NewUsers       int64    `json:"new_users"`
	ActiveListings int64    `json:"active_listings"`
	Views          int64    `json:"views"`
	Conversion     float64  `json:"conversion"` // orders ÷ listing views
	Series         []KPIDay `json:"series"`
}

type KPIDay struct {
	Day      string `json:"day"`
	GMVCents int64  `json:"gmv_cents"`
	Orders   int64  `json:"orders"`
	NewUsers int64  `json:"new_users"`
	Views    int64  `json:"views"`
}

// Metrics computes KPIs from the database.
type Metrics struct{ DB *sql.DB }

func (m *Metrics) KPIs(ctx context.Context, days int) (KPIs, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	k := KPIs{Days: days, Series: []KPIDay{}}
	since := time.Now().AddDate(0, 0, -days)
	if err := m.DB.QueryRowContext(ctx, `
		SELECT coalesce(sum(total_cents), 0), coalesce(sum(fee_cents), 0),
		       coalesce(sum(fee_cents) FILTER (WHERE status = 'refunded'), 0), count(*)
		FROM orders WHERE paid_at >= $1`, since).Scan(&k.GMVCents, &k.FeeCents, &k.RefundedFees, &k.Orders); err != nil {
		return k, err
	}
	k.NetFeeCents = k.FeeCents - k.RefundedFees
	if k.Orders > 0 {
		k.AOVCents = k.GMVCents / k.Orders
	}
	for _, q := range []struct {
		dst *int64
		sql string
	}{
		{&k.ActiveUsers, `SELECT count(DISTINCT user_id) FROM login_events WHERE success AND created_at >= $1`},
		{&k.NewUsers, `SELECT count(*) FROM users WHERE created_at >= $1`},
		{&k.Views, `SELECT coalesce(sum(views), 0) FROM listing_views_daily WHERE day >= $1::date`},
	} {
		if err := m.DB.QueryRowContext(ctx, q.sql, since).Scan(q.dst); err != nil {
			return k, err
		}
	}
	if err := m.DB.QueryRowContext(ctx, `SELECT count(*) FROM listings WHERE status = 'active' AND quantity > 0`).Scan(&k.ActiveListings); err != nil {
		return k, err
	}
	if k.Views > 0 {
		k.Conversion = float64(k.Orders) / float64(k.Views)
	}
	rows, err := m.DB.QueryContext(ctx, `
		WITH d AS (SELECT generate_series(($1::timestamptz)::date, current_date, interval '1 day')::date AS day)
		SELECT to_char(d.day, 'YYYY-MM-DD'),
		       coalesce((SELECT sum(total_cents) FROM orders o WHERE o.paid_at::date = d.day), 0),
		       (SELECT count(*) FROM orders o WHERE o.paid_at::date = d.day),
		       (SELECT count(*) FROM users u WHERE u.created_at::date = d.day),
		       coalesce((SELECT sum(views) FROM listing_views_daily v WHERE v.day = d.day), 0)
		FROM d ORDER BY d.day`, since)
	if err != nil {
		return k, err
	}
	defer rows.Close()
	for rows.Next() {
		var x KPIDay
		if err := rows.Scan(&x.Day, &x.GMVCents, &x.Orders, &x.NewUsers, &x.Views); err != nil {
			return k, err
		}
		k.Series = append(k.Series, x)
	}
	return k, rows.Err()
}

// CountView adds one page view to a listing's daily total.
func (m *Metrics) CountView(ctx context.Context, listingID string) error {
	_, err := m.DB.ExecContext(ctx, `INSERT INTO listing_views_daily (listing_id, day, views) VALUES ($1, current_date, 1)
		ON CONFLICT (listing_id, day) DO UPDATE SET views = listing_views_daily.views + 1`, listingID)
	return err
}
