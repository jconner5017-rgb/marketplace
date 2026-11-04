package httpapi

import (
	"context"
	"database/sql"
	"net/http"
)

// The seller hub's overview: everything the Overview tab shows, in one call.

type hubCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type hubDay struct {
	Day   string `json:"day"` // YYYY-MM-DD, US Eastern
	Cents int64  `json:"cents"`
	Count int    `json:"orders"`
}

type hubPeriod struct {
	Key    string `json:"key"` // today, 7d, 31d, 90d
	Cents  int64  `json:"cents"`
	Orders int    `json:"orders"`
}

type hubOverview struct {
	Listings map[string]int `json:"listings"`
	Orders   map[string]int `json:"orders"`
	Messages int            `json:"unread_messages"`
	Offers   int            `json:"offers_to_answer"`
	Sales    struct {
		Days    []hubDay    `json:"days"` // last 31 days, oldest first, every day present
		Periods []hubPeriod `json:"periods"`
	} `json:"sales"`
	Traffic struct {
		Views90    int     `json:"views_90d"`
		Views31    int     `json:"views_31d"`
		Orders90   int     `json:"orders_90d"`
		Conversion float64 `json:"conversion_90d"` // orders ÷ listing views, percent
	} `json:"traffic"`
	Performance struct {
		Orders12      int     `json:"orders_12m"`
		Late12        int     `json:"late_12m"`
		LatePct       float64 `json:"late_percent"`
		Tracked12     int     `json:"tracked_12m"`
		TrackedPct    float64 `json:"tracked_percent"` // label orders with tracking, percent
		Chargebacks12 int     `json:"chargebacks_12m"`
		OpenProblems  int     `json:"open_problems"`
	} `json:"performance"`
	Feedback30 struct {
		Positive int `json:"positive"`
		Neutral  int `json:"neutral"`
		Negative int `json:"negative"`
	} `json:"feedback_30d"`
}

// GET /v1/seller/overview
func (s *Server) handleSellerOverview(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	o, err := sellerOverview(r.Context(), s.db, u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if o.Messages, err = s.inbox.UnreadMessages(r.Context(), u.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func sellerOverview(ctx context.Context, db *sql.DB, sellerID string) (hubOverview, error) {
	var o hubOverview
	o.Listings, o.Orders = map[string]int{}, map[string]int{}
	counts := func(dst map[string]int, q string) error {
		rows, err := db.QueryContext(ctx, q, sellerID)
		if err != nil {
			return err
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			return err
		}
		vals := make([]int, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
		}
		for i, c := range cols {
			dst[c] = vals[i]
		}
		return rows.Err()
	}
	if err := counts(o.Listings, `SELECT
		  count(*) FILTER (WHERE status = 'active')::int AS active,
		  count(*) FILTER (WHERE status = 'active' AND format = 'fixed')::int AS fixed,
		  count(*) FILTER (WHERE auction_state = 'live')::int AS auctions,
		  count(*) FILTER (WHERE auction_state = 'live' AND ends_at < now() + interval '24 hours')::int AS ending_today,
		  count(*) FILTER (WHERE auction_state = 'live' AND reserve_cents IS NOT NULL AND bid_count > 0
		                   AND coalesce((SELECT max(b.shown_cents) FROM bids b WHERE b.listing_id = l.id AND b.round = l.auction_round AND b.removed_at IS NULL), 0) >= reserve_cents)::int AS reserve_met,
		  count(*) FILTER (WHERE status = 'active' AND EXISTS (SELECT 1 FROM offers f WHERE f.listing_id = l.id AND f.status = 'pending' AND f.from_role = 'buyer' AND f.expires_at > now()))::int AS with_offers,
		  count(*) FILTER (WHERE status = 'ended' OR auction_state = 'unsold')::int AS unsold,
		  count(*) FILTER (WHERE status = 'hidden')::int AS hidden,
		  count(*) FILTER (WHERE status = 'sold_out')::int AS sold_out
		FROM listings l WHERE seller_id = $1 AND status <> 'removed'`); err != nil {
		return o, err
	}
	if err := counts(o.Orders, `SELECT
		  count(*) FILTER (WHERE status = 'paid')::int AS to_ship,
		  count(*) FILTER (WHERE status = 'paid' AND ship_by < now())::int AS late,
		  count(*) FILTER (WHERE status = 'pending_payment' AND created_at > now() - interval '7 days')::int AS checkout_started,
		  (SELECT count(*) FROM holds h WHERE h.seller_id = $1 AND h.status = 'awaiting_payment')::int AS awaiting_payment,
		  (SELECT count(*) FROM cases c JOIN orders x ON x.id = c.order_id
		    WHERE x.seller_id = $1 AND c.status IN ('open', 'waiting_seller', 'waiting_buyer'))::int AS open_problems,
		  count(*) FILTER (WHERE status IN ('shipped', 'delivered') AND shipped_at > now() - interval '60 days'
		    AND NOT EXISTS (SELECT 1 FROM feedback f WHERE f.order_id = orders.id AND f.role = 'seller_to_buyer'))::int AS awaiting_feedback,
		  count(*) FILTER (WHERE status = 'paid' AND buyer_id IN (
		    SELECT buyer_id FROM orders y WHERE y.seller_id = $1 AND y.status = 'paid' GROUP BY buyer_id HAVING count(*) > 1))::int AS combinable
		FROM orders WHERE seller_id = $1`); err != nil {
		return o, err
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM offers WHERE seller_id = $1 AND status = 'pending'
		AND from_role = 'buyer' AND expires_at > now()`, sellerID).Scan(&o.Offers); err != nil {
		return o, err
	}

	// Sales by day for 31 days (US Eastern), and period totals.
	const net = `(o.total_cents - o.refunded_cents)`
	const paid = `o.seller_id = $1 AND o.paid_at IS NOT NULL AND o.status NOT IN ('cancelled', 'refunded')`
	rows, err := db.QueryContext(ctx, `
		SELECT d::date::text, coalesce(sum(`+net+`), 0), count(o.id)
		FROM generate_series((now() AT TIME ZONE 'America/New_York')::date - 30, (now() AT TIME ZONE 'America/New_York')::date, interval '1 day') d
		LEFT JOIN orders o ON `+paid+` AND (o.paid_at AT TIME ZONE 'America/New_York')::date = d::date
		GROUP BY d ORDER BY d`, sellerID)
	if err != nil {
		return o, err
	}
	defer rows.Close()
	o.Sales.Days = []hubDay{}
	for rows.Next() {
		var d hubDay
		if err := rows.Scan(&d.Day, &d.Cents, &d.Count); err != nil {
			return o, err
		}
		o.Sales.Days = append(o.Sales.Days, d)
	}
	if err := rows.Err(); err != nil {
		return o, err
	}
	for _, p := range []struct {
		key  string
		from string
	}{
		{"today", `(date_trunc('day', now() AT TIME ZONE 'America/New_York') AT TIME ZONE 'America/New_York')`},
		{"7d", `now() - interval '7 days'`}, {"31d", `now() - interval '31 days'`}, {"90d", `now() - interval '90 days'`},
	} {
		x := hubPeriod{Key: p.key}
		if err := db.QueryRowContext(ctx, `SELECT coalesce(sum(`+net+`), 0), count(*) FROM orders o WHERE `+paid+` AND o.paid_at >= `+p.from,
			sellerID).Scan(&x.Cents, &x.Orders); err != nil {
			return o, err
		}
		o.Sales.Periods = append(o.Sales.Periods, x)
	}

	// Traffic: listing page views.
	t := &o.Traffic
	if err := db.QueryRowContext(ctx, `SELECT
		  coalesce(sum(v.views) FILTER (WHERE v.day > current_date - 90), 0),
		  coalesce(sum(v.views) FILTER (WHERE v.day > current_date - 31), 0)
		FROM listing_views_daily v JOIN listings l ON l.id = v.listing_id WHERE l.seller_id = $1`, sellerID).Scan(&t.Views90, &t.Views31); err != nil {
		return o, err
	}
	for _, p := range o.Sales.Periods {
		if p.Key == "90d" {
			t.Orders90 = p.Orders
		}
	}
	if t.Views90 > 0 {
		t.Conversion = round1(float64(t.Orders90) * 100 / float64(t.Views90))
	}

	// Performance over 12 months.
	pf := &o.Performance
	var labelShipped int
	if err := db.QueryRowContext(ctx, `SELECT
		  count(*),
		  count(*) FILTER (WHERE o.ship_by IS NOT NULL AND coalesce(o.shipped_at, CASE WHEN o.status IN ('shipped', 'delivered') THEN NULL ELSE now() END) > o.ship_by),
		  count(*) FILTER (WHERE o.ship_method = 'label' AND o.status IN ('shipped', 'delivered')),
		  count(*) FILTER (WHERE o.ship_method = 'label' AND o.status IN ('shipped', 'delivered')
		    AND EXISTS (SELECT 1 FROM shipments sh WHERE sh.order_id = o.id))
		FROM orders o WHERE o.seller_id = $1 AND o.paid_at > now() - interval '12 months'`, sellerID).
		Scan(&pf.Orders12, &pf.Late12, &labelShipped, &pf.Tracked12); err != nil {
		return o, err
	}
	if pf.Orders12 > 0 {
		pf.LatePct = round1(float64(pf.Late12) * 100 / float64(pf.Orders12))
	}
	pf.TrackedPct = 100
	if labelShipped > 0 {
		pf.TrackedPct = round1(float64(pf.Tracked12) * 100 / float64(labelShipped))
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM ledger_entries WHERE seller_id = $1 AND kind = 'chargeback_lost'
		AND created_at > now() - interval '12 months'`, sellerID).Scan(&pf.Chargebacks12); err != nil {
		return o, err
	}
	pf.OpenProblems = o.Orders["open_problems"]

	f := &o.Feedback30
	if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE rating = 1), count(*) FILTER (WHERE rating = 0), count(*) FILTER (WHERE rating = -1)
		FROM feedback WHERE to_user = $1 AND role = 'buyer_to_seller' AND removed_at IS NULL AND state = 'published'
		AND visible_at IS NOT NULL AND created_at > now() - interval '30 days'`, sellerID).Scan(&f.Positive, &f.Neutral, &f.Negative); err != nil {
		return o, err
	}
	return o, nil
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
