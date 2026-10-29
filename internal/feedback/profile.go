package feedback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// counted is the feedback that counts toward scores: not removed, past any
// cooling-off or mediation, and revealed.
const counted = `f.removed_at IS NULL AND f.state = 'published' AND f.visible_at IS NOT NULL`

// Summary is a member's reputation as a seller (or as a buyer).
type Summary struct {
	Username      string    `json:"username"`
	Score         int       `json:"score"`            // lifetime positives minus negatives
	Percent       *float64  `json:"positive_percent"` // last 12 months, of positive + negative
	Positive12    int       `json:"positive_12m"`
	Neutral12     int       `json:"neutral_12m"`
	Negative12    int       `json:"negative_12m"`
	Total         int       `json:"total"`
	Item          *float64  `json:"item_avg"`
	Shipping      *float64  `json:"shipping_avg"`
	Communication *float64  `json:"communication_avg"`
	UnderReview   int       `json:"under_review"` // a new seller's first negative in mediation
	MemberSince   time.Time `json:"member_since"`
	Sales         int       `json:"sales"`
	Badges        []Badge   `json:"badges"`
	Outside       []Outside `json:"outside"`
}

type Badge struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// Outside is feedback verified on another platform.
type Outside struct {
	Platform        string    `json:"platform"`
	ProfileURL      string    `json:"profile_url"`
	FeedbackCount   int       `json:"feedback_count"`
	PositivePercent float64   `json:"positive_percent"`
	VerifiedAt      time.Time `json:"verified_at"`
}

// SummaryFor builds a member's public reputation. role is "seller" or "buyer".
func (s *Service) SummaryFor(ctx context.Context, userID, role string) (Summary, error) {
	r := "buyer_to_seller"
	if role == "buyer" {
		r = "seller_to_buyer"
	}
	var sm Summary
	var pct, item, ship, comm sql.NullFloat64
	err := s.DB.QueryRowContext(ctx, `
		SELECT u.username, u.created_at,
		  coalesce(sum(f.rating) FILTER (WHERE `+counted+`), 0),
		  count(*) FILTER (WHERE `+counted+` AND f.rating = 1 AND f.created_at > now() - interval '12 months'),
		  count(*) FILTER (WHERE `+counted+` AND f.rating = 0 AND f.created_at > now() - interval '12 months'),
		  count(*) FILTER (WHERE `+counted+` AND f.rating = -1 AND f.created_at > now() - interval '12 months'),
		  count(*) FILTER (WHERE `+counted+`),
		  avg(f.item_rating) FILTER (WHERE `+counted+` AND f.created_at > now() - interval '12 months'),
		  avg(f.shipping_rating) FILTER (WHERE `+counted+` AND f.created_at > now() - interval '12 months'),
		  avg(f.communication_rating) FILTER (WHERE `+counted+` AND f.created_at > now() - interval '12 months'),
		  count(*) FILTER (WHERE f.removed_at IS NULL AND f.state = 'mediation')
		FROM users u LEFT JOIN feedback f ON f.to_user = u.id AND f.role = $2
		WHERE u.id = $1 GROUP BY u.id`, userID, r).Scan(&sm.Username, &sm.MemberSince, &sm.Score, &sm.Positive12, &sm.Neutral12,
		&sm.Negative12, &sm.Total, &item, &ship, &comm, &sm.UnderReview)
	if errors.Is(err, sql.ErrNoRows) {
		return sm, ErrNotFound
	}
	if err != nil {
		return sm, err
	}
	if n := sm.Positive12 + sm.Negative12; n > 0 {
		v := float64(sm.Positive12) * 100 / float64(n)
		pct = sql.NullFloat64{Float64: v, Valid: true}
	}
	sm.Percent, sm.Item, sm.Shipping, sm.Communication = nf(pct), nf(item), nf(ship), nf(comm)
	sm.Badges, sm.Outside = []Badge{}, []Outside{}
	if role != "seller" {
		return sm, nil
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM orders WHERE seller_id = $1 AND status IN ('shipped', 'delivered')`,
		userID).Scan(&sm.Sales); err != nil {
		return sm, err
	}
	// Verified new seller: three or more small starter orders delivered with
	// no case, refund or chargeback, while still under 50 sales.
	var starters int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM orders o WHERE o.seller_id = $1 AND o.status = 'delivered'
		AND o.total_cents <= 5000 AND o.refunded_cents = 0
		AND NOT EXISTS (SELECT 1 FROM cases c WHERE c.order_id = o.id)
		AND NOT EXISTS (SELECT 1 FROM ledger_entries l WHERE l.order_id = o.id AND l.kind = 'chargeback_opened')`, userID).Scan(&starters); err != nil {
		return sm, err
	}
	if starters >= 3 && sm.Sales < 50 {
		sm.Badges = append(sm.Badges, Badge{"verified_new_seller", "Verified new seller",
			"Completed starter orders on time with no problems reported"})
	}
	if sm.Sales >= 50 && sm.Percent != nil && *sm.Percent >= 99 {
		sm.Badges = append(sm.Badges, Badge{"top_rated", "Top rated", "50+ sales and 99% or better positive in the last year"})
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT platform, profile_url, feedback_count, positive_percent::float8, reviewed_at
		FROM external_reputation WHERE user_id = $1 AND status = 'verified' ORDER BY feedback_count DESC`, userID)
	if err != nil {
		return sm, err
	}
	defer rows.Close()
	for rows.Next() {
		var o Outside
		if err := rows.Scan(&o.Platform, &o.ProfileURL, &o.FeedbackCount, &o.PositivePercent, &o.VerifiedAt); err != nil {
			return sm, err
		}
		sm.Outside = append(sm.Outside, o)
	}
	return sm, rows.Err()
}

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := float64(int(v.Float64*10+0.5)) / 10
	return &x
}

// Mask shows a username as "c***r" in public feedback lists.
func Mask(name string) string {
	r := []rune(name)
	if len(r) <= 2 {
		return string(r[:1]) + "***"
	}
	return string(r[0]) + "***" + string(r[len(r)-1])
}

// Entry is one public feedback line.
type Entry struct {
	ID            string    `json:"id"`
	From          string    `json:"from"` // masked
	Role          string    `json:"role"`
	Rating        int       `json:"rating"`
	Comment       string    `json:"comment"`
	Item          *int      `json:"item_rating"`
	Shipping      *int      `json:"shipping_rating"`
	Communication *int      `json:"communication_rating"`
	ItemTitle     string    `json:"item_title"`
	Reply         *string   `json:"reply"`
	CanReply      bool      `json:"can_reply,omitempty"`
	At            time.Time `json:"created_at"`
	Verified      bool      `json:"verified_purchase"`
}

// Received lists feedback a member received (public view). viewerID, when
// it's the member themself, marks entries they can reply to.
func (s *Service) Received(ctx context.Context, userID, role, filter, viewerID string, limit int) ([]Entry, error) {
	r := "buyer_to_seller"
	if role == "buyer" {
		r = "seller_to_buyer"
	}
	where := ""
	switch filter {
	case "positive":
		where = " AND f.rating = 1"
	case "neutral":
		where = " AND f.rating = 0"
	case "negative":
		where = " AND f.rating = -1"
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.id, fu.username, f.role, f.rating, f.comment, f.item_rating, f.shipping_rating, f.communication_rating,
		       coalesce((SELECT title FROM order_items WHERE order_id = f.order_id ORDER BY id LIMIT 1), ''), f.reply, f.created_at
		FROM feedback f JOIN users fu ON fu.id = f.from_user
		WHERE f.to_user = $1 AND f.role = $2 AND `+counted+where+`
		ORDER BY f.created_at DESC LIMIT $3`, userID, r, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var item, ship, comm sql.NullInt64
		var reply sql.NullString
		var from string
		if err := rows.Scan(&e.ID, &from, &e.Role, &e.Rating, &e.Comment, &item, &ship, &comm, &e.ItemTitle, &reply, &e.At); err != nil {
			return nil, err
		}
		e.From, e.Verified = Mask(from), true
		e.Item, e.Shipping, e.Communication = ni(item), ni(ship), ni(comm)
		if reply.Valid {
			e.Reply = &reply.String
		}
		e.CanReply = viewerID == userID && e.Rating <= 0 && e.Reply == nil && e.Role == "buyer_to_seller"
		out = append(out, e)
	}
	return out, rows.Err()
}

func ni(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	x := int(v.Int64)
	return &x
}

// Reliability is a buyer's track record, shown to sellers they deal with.
type Reliability struct {
	Username       string    `json:"username"`
	MemberSince    time.Time `json:"member_since"`
	OrdersPaid     int       `json:"orders_paid"`
	WinsPaid       int       `json:"wins_paid"`   // auction wins and accepted offers paid
	WinsUnpaid     int       `json:"wins_unpaid"` // let lapse
	PayRate        *float64  `json:"pay_rate"`    // percent of wins and offers paid in time
	Strikes        int       `json:"unpaid_strikes"`
	Cases          int       `json:"cases_opened"`
	Refunds        int       `json:"orders_refunded"`
	ReturnRate     *float64  `json:"refund_rate"`
	Chargebacks    int       `json:"chargebacks"`
	SellerFeedback Summary   `json:"from_sellers"`
}

var ErrNoRelation = errors.New("you can see a buyer's record once they bid on, make an offer on, or buy one of your cards")

// BuyerReliability returns a buyer's record for a seller who is dealing with them.
func (s *Service) BuyerReliability(ctx context.Context, sellerID, username string) (Reliability, error) {
	var buyerID string
	var rel Reliability
	err := s.DB.QueryRowContext(ctx, `SELECT id, username, created_at, buyer_strikes FROM users WHERE lower(username) = lower($1)`,
		strings.TrimSpace(username)).Scan(&buyerID, &rel.Username, &rel.MemberSince, &rel.Strikes)
	if errors.Is(err, sql.ErrNoRows) {
		return rel, ErrNotFound
	}
	if err != nil {
		return rel, err
	}
	var related bool
	if err := s.DB.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM orders WHERE seller_id = $1 AND buyer_id = $2)
		OR EXISTS (SELECT 1 FROM holds WHERE seller_id = $1 AND buyer_id = $2)
		OR EXISTS (SELECT 1 FROM offers o JOIN listings l ON l.id = o.listing_id WHERE l.seller_id = $1 AND o.buyer_id = $2)
		OR EXISTS (SELECT 1 FROM bids b JOIN listings l ON l.id = b.listing_id WHERE l.seller_id = $1 AND b.bidder_id = $2)`,
		sellerID, buyerID).Scan(&related); err != nil {
		return rel, err
	}
	if !related && sellerID != buyerID {
		return rel, ErrNoRelation
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM orders WHERE buyer_id = $1 AND paid_at IS NOT NULL),
		(SELECT count(*) FROM holds WHERE buyer_id = $1 AND status = 'paid'),
		(SELECT count(*) FROM holds WHERE buyer_id = $1 AND status = 'expired'),
		(SELECT count(*) FROM cases c JOIN orders o ON o.id = c.order_id WHERE o.buyer_id = $1 AND c.opened_by = $1),
		(SELECT count(*) FROM orders WHERE buyer_id = $1 AND status IN ('refunded', 'partially_refunded')),
		(SELECT count(*) FROM ledger_entries l JOIN orders o ON o.id = l.order_id WHERE o.buyer_id = $1 AND l.kind = 'chargeback_opened')`,
		buyerID).Scan(&rel.OrdersPaid, &rel.WinsPaid, &rel.WinsUnpaid, &rel.Cases, &rel.Refunds, &rel.Chargebacks); err != nil {
		return rel, err
	}
	if n := rel.WinsPaid + rel.WinsUnpaid; n > 0 {
		v := float64(rel.WinsPaid) * 100 / float64(n)
		rel.PayRate = nf(sql.NullFloat64{Float64: v, Valid: true})
	}
	if rel.OrdersPaid > 0 {
		v := float64(rel.Refunds) * 100 / float64(rel.OrdersPaid)
		rel.ReturnRate = nf(sql.NullFloat64{Float64: v, Valid: true})
	}
	rel.SellerFeedback, err = s.SummaryFor(ctx, buyerID, "buyer")
	return rel, err
}

// ---- Verified outside feedback ----

var Platforms = map[string]string{"ebay": "eBay", "etsy": "Etsy", "discogs": "Discogs", "tcgplayer": "TCGplayer", "comc": "COMC", "other": "Other"}

var platformHosts = map[string][]string{
	"ebay": {"ebay.com"}, "etsy": {"etsy.com"}, "discogs": {"discogs.com"}, "tcgplayer": {"tcgplayer.com"}, "comc": {"comc.com"},
}

// OutsideClaim is a seller's request to show feedback from another site.
type OutsideClaim struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id,omitempty"`
	Username        string     `json:"username,omitempty"`
	Platform        string     `json:"platform"`
	ProfileURL      string     `json:"profile_url"`
	FeedbackCount   int        `json:"feedback_count"`
	PositivePercent float64    `json:"positive_percent"`
	VerifyCode      string     `json:"verify_code"`
	Status          string     `json:"status"`
	ReviewNote      *string    `json:"review_note"`
	CreatedAt       time.Time  `json:"created_at"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
}

// ClaimOutside records (or replaces) a seller's claim. To prove the profile
// is theirs, they add the returned code to its bio until staff check it.
func (s *Service) ClaimOutside(ctx context.Context, userID, platform, profile string, count int, percent float64) (OutsideClaim, error) {
	if _, ok := Platforms[platform]; !ok {
		return OutsideClaim{}, &FieldError{"platform", "choose where the feedback is"}
	}
	u, err := url.Parse(strings.TrimSpace(profile))
	if err != nil || u.Scheme != "https" || u.Host == "" || len(profile) > 300 {
		return OutsideClaim{}, &FieldError{"profile_url", "paste the full https:// link to your profile"}
	}
	if hosts, ok := platformHosts[platform]; ok {
		h := strings.ToLower(u.Hostname())
		match := false
		for _, want := range hosts {
			if h == want || strings.HasSuffix(h, "."+want) {
				match = true
			}
		}
		if !match {
			return OutsideClaim{}, &FieldError{"profile_url", "that link isn't on " + Platforms[platform]}
		}
	}
	if count < 1 || count > 10_000_000 {
		return OutsideClaim{}, &FieldError{"feedback_count", "enter how many ratings you have there"}
	}
	if percent < 0 || percent > 100 {
		return OutsideClaim{}, &FieldError{"positive_percent", "enter your positive percentage (0 to 100)"}
	}
	code := fmt.Sprintf("SLABS-%04d-%04d", rand.IntN(10000), rand.IntN(10000))
	var c OutsideClaim
	err = s.DB.QueryRowContext(ctx, `INSERT INTO external_reputation (user_id, platform, profile_url, feedback_count, positive_percent, verify_code)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, platform) DO UPDATE SET profile_url = $3, feedback_count = $4, positive_percent = $5, verify_code = $6,
			status = 'pending', review_note = NULL, reviewed_by = NULL, reviewed_at = NULL, created_at = now()
		RETURNING id, platform, profile_url, feedback_count, positive_percent::float8, verify_code, status, created_at`,
		userID, platform, u.String(), count, percent, code).Scan(&c.ID, &c.Platform, &c.ProfileURL, &c.FeedbackCount, &c.PositivePercent,
		&c.VerifyCode, &c.Status, &c.CreatedAt)
	return c, err
}

const claimCols = `x.id, x.user_id, u.username, x.platform, x.profile_url, x.feedback_count, x.positive_percent::float8, x.verify_code,
	x.status, x.review_note, x.created_at, x.reviewed_at`

func scanClaims(rows *sql.Rows) ([]OutsideClaim, error) {
	defer rows.Close()
	out := []OutsideClaim{}
	for rows.Next() {
		var c OutsideClaim
		if err := rows.Scan(&c.ID, &c.UserID, &c.Username, &c.Platform, &c.ProfileURL, &c.FeedbackCount, &c.PositivePercent,
			&c.VerifyCode, &c.Status, &c.ReviewNote, &c.CreatedAt, &c.ReviewedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) MyClaims(ctx context.Context, userID string) ([]OutsideClaim, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+claimCols+` FROM external_reputation x JOIN users u ON u.id = x.user_id
		WHERE x.user_id = $1 ORDER BY x.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanClaims(rows)
}

// PendingClaims is the staff verification queue.
func (s *Service) Claims(ctx context.Context, status string) ([]OutsideClaim, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+claimCols+` FROM external_reputation x JOIN users u ON u.id = x.user_id
		WHERE ($1 = '' OR x.status = $1) ORDER BY x.created_at LIMIT 200`, status)
	if err != nil {
		return nil, err
	}
	return scanClaims(rows)
}

// ReviewClaim approves or rejects a claim (staff). The caller audits it.
func (s *Service) ReviewClaim(ctx context.Context, staffID, id, decision, note string) (OutsideClaim, error) {
	if decision != "verified" && decision != "rejected" {
		return OutsideClaim{}, &FieldError{"decision", "verify or reject"}
	}
	note = strings.TrimSpace(note)
	if decision == "rejected" && utf8.RuneCountInString(note) < 3 {
		return OutsideClaim{}, &FieldError{"note", "say why (the seller sees this)"}
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE external_reputation SET status = $2, review_note = nullif($3, ''), reviewed_by = $4, reviewed_at = now()
		WHERE id::text = $1 AND status = 'pending'`, id, decision, note, staffID)
	if err != nil {
		return OutsideClaim{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return OutsideClaim{}, ErrNotFound
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+claimCols+` FROM external_reputation x JOIN users u ON u.id = x.user_id WHERE x.id::text = $1`, id)
	if err != nil {
		return OutsideClaim{}, err
	}
	cs, err := scanClaims(rows)
	if err != nil || len(cs) == 0 {
		return OutsideClaim{}, err
	}
	title := "Your " + Platforms[cs[0].Platform] + " feedback is verified"
	body := "It now shows on your seller profile. You can remove the code from your " + Platforms[cs[0].Platform] + " profile."
	if decision == "rejected" {
		title, body = "We couldn't verify your "+Platforms[cs[0].Platform]+" feedback", note
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, 'system', $2, $3, '/sell/feedback')`,
		cs[0].UserID, title, body)
	return cs[0], err
}
