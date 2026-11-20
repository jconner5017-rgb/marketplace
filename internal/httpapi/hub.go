package httpapi

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/jconner5017-rgb/marketplace/internal/sellers"
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
		// The last 31 days against the 31 before (2026-11-20), like eBay's traffic card.
		Impressions31    int     `json:"impressions_31d"`
		ImpressionsPrev  int     `json:"impressions_prev_31d"`
		ViewsPrev        int     `json:"views_prev_31d"`
		CTR31            float64 `json:"ctr_31d"` // views ÷ impressions, percent
		CTRPrev          float64 `json:"ctr_prev_31d"`
		Orders31         int     `json:"orders_31d"`
		OrdersPrev       int     `json:"orders_prev_31d"`
		Conversion31     float64 `json:"conversion_31d"` // orders ÷ views, percent
		ConversionPrev   float64 `json:"conversion_prev_31d"`
		ImpressionsSince string  `json:"impressions_since"` // when counting began
	} `json:"traffic"`
	// Selling costs over the last 31 days (2026-11-20): what came in and what
	// came out of it, paid orders by the day they were paid.
	Costs struct {
		TotalSales int64 `json:"total_sales_cents"` // what buyers paid, with sales tax
		Tax        int64 `json:"tax_cents"`         // sales tax Stripe collected and remits
		Fees       int64 `json:"fee_cents"`         // our fees, less fees returned on refunds
		Labels     int64 `json:"label_cents"`       // shipping labels and envelope postage, less voids
		Refunds    int64 `json:"refund_cents"`
		Discounts  int64 `json:"discount_cents"` // coupons and order discounts (already off the total)
		Net        int64 `json:"net_cents"`      // before Stripe's card processing fee
	} `json:"costs_31d"`
	Level struct {
		Tier      int     `json:"tier"`       // now
		TierToday int     `json:"tier_today"` // if we checked right now
		Held      bool    `json:"held"`       // staff have set the level by hand
		Defects   int     `json:"defects_12m"`
		DefectPct float64 `json:"defect_percent"` // orders with a problem reported, refunded for an oversell or charged back
	} `json:"level"`
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
		{"year", `(date_trunc('year', now() AT TIME ZONE 'America/New_York') AT TIME ZONE 'America/New_York')`},
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

	// The last 31 days against the 31 before: impressions, views, click-through and conversion.
	var since sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT
		  coalesce((SELECT sum(i.impressions) FROM listing_impressions_daily i JOIN listings l ON l.id = i.listing_id
		            WHERE l.seller_id = $1 AND i.day > current_date - 31), 0),
		  coalesce((SELECT sum(i.impressions) FROM listing_impressions_daily i JOIN listings l ON l.id = i.listing_id
		            WHERE l.seller_id = $1 AND i.day > current_date - 62 AND i.day <= current_date - 31), 0),
		  coalesce((SELECT sum(v.views) FROM listing_views_daily v JOIN listings l ON l.id = v.listing_id
		            WHERE l.seller_id = $1 AND v.day > current_date - 62 AND v.day <= current_date - 31), 0),
		  (SELECT count(*) FROM orders WHERE seller_id = $1 AND paid_at IS NOT NULL AND status NOT IN ('cancelled') AND paid_at > now() - interval '31 days'),
		  (SELECT count(*) FROM orders WHERE seller_id = $1 AND paid_at IS NOT NULL AND status NOT IN ('cancelled')
		     AND paid_at > now() - interval '62 days' AND paid_at <= now() - interval '31 days'),
		  (SELECT min(day)::timestamptz FROM listing_impressions_daily)`, sellerID).
		Scan(&t.Impressions31, &t.ImpressionsPrev, &t.ViewsPrev, &t.Orders31, &t.OrdersPrev, &since); err != nil {
		return o, err
	}
	pct := func(a, b int) float64 {
		if b <= 0 {
			return 0
		}
		return round1(float64(a) * 100 / float64(b))
	}
	t.CTR31, t.CTRPrev = pct(t.Views31, t.Impressions31), pct(t.ViewsPrev, t.ImpressionsPrev)
	t.Conversion31, t.ConversionPrev = pct(t.Orders31, t.Views31), pct(t.OrdersPrev, t.ViewsPrev)
	if since.Valid {
		t.ImpressionsSince = since.Time.Format("2006-01-02")
	}

	// Selling costs, last 31 days.
	c := &o.Costs
	if err := db.QueryRowContext(ctx, `SELECT
		  coalesce(sum(o.total_cents + o.tax_cents), 0), coalesce(sum(o.tax_cents), 0), coalesce(sum(o.fee_cents), 0),
		  coalesce(sum(o.refunded_cents), 0), coalesce(sum(o.discount_cents), 0)
		FROM orders o WHERE o.seller_id = $1 AND o.paid_at > now() - interval '31 days' AND o.status <> 'cancelled' AND NOT o.test`,
		sellerID).Scan(&c.TotalSales, &c.Tax, &c.Fees, &c.Refunds, &c.Discounts); err != nil {
		return o, err
	}
	var feeBack, labels, voids int64
	if err := db.QueryRowContext(ctx, `SELECT
		  coalesce(sum(amount_cents) FILTER (WHERE kind = 'fee_refund'), 0),
		  coalesce(sum(amount_cents) FILTER (WHERE kind = 'label_cost'), 0),
		  coalesce(sum(amount_cents) FILTER (WHERE kind = 'adjustment' AND (note LIKE 'Label voided%' OR note LIKE 'Postage voided%')), 0)
		FROM ledger_entries WHERE seller_id = $1 AND created_at > now() - interval '31 days'`, sellerID).Scan(&feeBack, &labels, &voids); err != nil {
		return o, err
	}
	c.Fees = max(c.Fees-feeBack, 0)
	c.Labels = max(labels-voids, 0)
	c.Net = c.TotalSales - c.Tax - c.Fees - c.Labels - c.Refunds

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

	// Seller level: now, and if we checked today; transaction defects (12 months):
	// orders with a problem the buyer reported, refunded because the card had
	// sold elsewhere, or charged back.
	lv := &o.Level
	if err := db.QueryRowContext(ctx, `SELECT tier, tier_locked FROM seller_profiles WHERE user_id = $1`, sellerID).Scan(&lv.Tier, &lv.Held); err != nil {
		return o, err
	}
	lv.TierToday = lv.Tier
	if !lv.Held {
		if st, err := sellers.SellerStats(ctx, db, sellerID); err == nil {
			lv.TierToday = sellers.TierFor(st)
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM orders o WHERE o.seller_id = $1 AND o.paid_at > now() - interval '12 months'
		AND (EXISTS (SELECT 1 FROM cases c WHERE c.order_id = o.id AND c.opened_by = o.buyer_id)
		  OR EXISTS (SELECT 1 FROM oversells x WHERE x.order_id = o.id AND x.outcome = 'refunded')
		  OR EXISTS (SELECT 1 FROM ledger_entries le WHERE le.order_id = o.id AND le.kind = 'chargeback_lost'))`, sellerID).Scan(&lv.Defects); err != nil {
		return o, err
	}
	if pf.Orders12 > 0 {
		lv.DefectPct = round1(float64(lv.Defects) * 100 / float64(pf.Orders12))
	}

	f := &o.Feedback30
	if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE rating = 1), count(*) FILTER (WHERE rating = 0), count(*) FILTER (WHERE rating = -1)
		FROM feedback WHERE to_user = $1 AND role = 'buyer_to_seller' AND removed_at IS NULL AND state = 'published'
		AND visible_at IS NOT NULL AND created_at > now() - interval '30 days'`, sellerID).Scan(&f.Positive, &f.Neutral, &f.Negative); err != nil {
		return o, err
	}
	return o, nil
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
