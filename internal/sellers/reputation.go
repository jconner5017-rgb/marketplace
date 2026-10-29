package sellers

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Tier requirements (spec: Seller tiers and reputation). Starting values,
// to be tuned with real data.
var TierRules = []struct {
	Tier       int
	Sales      int     // qualified sales to different buyers
	MinPercent float64 // positive feedback, of positive + negative
	MinAge     time.Duration
}{
	{3, 150, 98, 90 * 24 * time.Hour},
	{2, 50, 97, 30 * 24 * time.Hour},
}

// Stats is what a seller's tier is decided on.
type Stats struct {
	QualifiedSales int     `json:"qualified_sales"`
	Positive       int     `json:"positive"`
	Negative       int     `json:"negative"`
	Percent        float64 `json:"percent_positive"`
	AgeDays        int     `json:"account_age_days"`
	Chargebacks    int     `json:"chargebacks_lost"`
	Oversells      int     `json:"oversells_90d"`
	Orders90       int     `json:"orders_90d"`
}

// A sale qualifies when: one per buyer; a card at $2 or more; the order
// shipped or was delivered and wasn't refunded or lost to a chargeback; and
// the buyer never signed in from an address the seller used.
const qualifiedSalesSQL = `
	SELECT count(DISTINCT o.buyer_id) FROM orders o
	WHERE o.seller_id = $1 AND o.status IN ('shipped', 'delivered')
	  AND EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.unit_cents >= 200)
	  AND NOT EXISTS (SELECT 1 FROM ledger_entries le WHERE le.order_id = o.id AND le.kind IN ('chargeback_lost', 'refund'))
	  AND NOT EXISTS (
	      SELECT 1 FROM login_events b JOIN login_events s ON s.ip = b.ip
	      WHERE b.user_id = o.buyer_id AND s.user_id = o.seller_id AND b.success AND s.success AND b.ip IS NOT NULL)`

// SellerStats works out a seller's reputation numbers.
func SellerStats(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, userID string) (Stats, error) {
	var st Stats
	var created time.Time
	if err := q.QueryRowContext(ctx, qualifiedSalesSQL, userID).Scan(&st.QualifiedSales); err != nil {
		return st, err
	}
	if err := q.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE rating = 1), count(*) FILTER (WHERE rating = -1) FROM feedback
		WHERE to_user = $1 AND role = 'buyer_to_seller' AND removed_at IS NULL AND created_at > now() - interval '12 months'`,
		userID).Scan(&st.Positive, &st.Negative); err != nil {
		return st, err
	}
	if err := q.QueryRowContext(ctx, `SELECT created_at FROM users WHERE id = $1`, userID).Scan(&created); err != nil {
		return st, err
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM ledger_entries WHERE seller_id = $1 AND kind = 'chargeback_lost'
		AND created_at > now() - interval '12 months'`, userID).Scan(&st.Chargebacks); err != nil {
		return st, err
	}
	if err := q.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM oversells WHERE seller_id = $1 AND outcome = 'refunded' AND created_at > now() - interval '90 days'),
		(SELECT count(*) FROM orders WHERE seller_id = $1 AND paid_at > now() - interval '90 days')`, userID).
		Scan(&st.Oversells, &st.Orders90); err != nil {
		return st, err
	}
	st.Percent = 100
	if n := st.Positive + st.Negative; n > 0 {
		st.Percent = float64(st.Positive) * 100 / float64(n)
	}
	st.AgeDays = int(time.Since(created).Hours() / 24)
	return st, nil
}

// TierFor is the tier a seller's numbers earn. Frequent cross-listing
// oversells (3 or more, over 5% of orders in 90 days) cost one level.
func TierFor(st Stats) int {
	t := 1
	for _, r := range TierRules {
		if st.QualifiedSales >= r.Sales && st.Percent >= r.MinPercent && st.AgeDays >= int(r.MinAge.Hours()/24) {
			t = r.Tier
			break
		}
	}
	if st.Oversells >= 3 && st.Orders90 > 0 && st.Oversells*100 > st.Orders90*5 && t > 1 {
		t--
	}
	return t
}

// Reputation is the job that moves sellers between tiers.
type Reputation struct{ DB *sql.DB }

// Review re-checks one seller (after feedback, a chargeback, or nightly).
// Sellers whose tier staff have held are left alone. It returns the change, if any.
func (r *Reputation) Review(ctx context.Context, userID string) (from, to int, err error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var tier int
	var locked bool
	err = tx.QueryRowContext(ctx, `SELECT tier, tier_locked FROM seller_profiles WHERE user_id = $1 FOR UPDATE`, userID).Scan(&tier, &locked)
	if err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE seller_profiles SET tier_reviewed_at = now() WHERE user_id = $1`, userID); err != nil {
		return 0, 0, err
	}
	if locked {
		return tier, tier, tx.Commit()
	}
	st, err := SellerStats(ctx, tx, userID)
	if err != nil {
		return 0, 0, err
	}
	want := TierFor(st)
	if want == tier {
		return tier, tier, tx.Commit()
	}
	reason := fmt.Sprintf("%d qualified sales, %.1f%% positive feedback, account %d days old.", st.QualifiedSales, st.Percent, st.AgeDays)
	if want > tier {
		reason = "Congratulations! " + reason
	} else {
		reason = "Your numbers dropped below this level's requirements: " + reason
	}
	stats := map[string]any{"by": "reputation_job", "qualified_sales": st.QualifiedSales, "positive": st.Positive,
		"negative": st.Negative, "percent": st.Percent, "age_days": st.AgeDays}
	if _, err := ApplyTier(ctx, tx, userID, want, reason, "", stats); err != nil {
		return 0, 0, err
	}
	return tier, want, tx.Commit()
}

// RunNightly reviews every seller not checked in the last 20 hours. It
// returns how many tiers changed.
func (r *Reputation) RunNightly(ctx context.Context) (int, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT sp.user_id FROM seller_profiles sp JOIN users u ON u.id = sp.user_id
		WHERE u.status IN ('active', 'frozen') AND (sp.tier_reviewed_at IS NULL OR sp.tier_reviewed_at < now() - interval '20 hours')
		ORDER BY sp.tier_reviewed_at NULLS FIRST LIMIT 2000`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	changed := 0
	for _, id := range ids {
		from, to, err := r.Review(ctx, id)
		if err != nil {
			return changed, err
		}
		if from != to {
			changed++
		}
	}
	return changed, nil
}
