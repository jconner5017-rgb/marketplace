// Package feedback runs ratings between buyers and sellers: who may rate
// whom, blind reveal, sub-ratings, cooling-off for negatives, a grace rule
// for new sellers, extortion scrubbing, public replies, scores and badges.
package feedback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

const (
	// Window is how long after payment either side can leave feedback.
	Window = 60 * 24 * time.Hour
	// RevealAfter: feedback becomes visible this long after delivery even if
	// the other side never rates (shipped + 7 days stands in for delivery
	// when there's no scan; payment + 21 days when it never shipped).
	RevealAfter = 14 * 24 * time.Hour
	// HoldFor is a negative's cooling-off before it locks.
	HoldFor = 72 * time.Hour
	// GraceSales: a seller's first negative within their first 10 sales goes
	// to staff mediation instead of straight onto their profile.
	GraceSales = 10
	ReplyMax   = 300
	CommentMax = 500
)

var (
	ErrNotFound     = errors.New("order not found")
	ErrNotYet       = errors.New("you can leave feedback once the order has shipped")
	ErrClosed       = errors.New("the feedback window for this order has closed")
	ErrAlreadyLeft  = errors.New("you've already left feedback for this order")
	ErrResolveFirst = errors.New("before leaving a negative, message the seller or report the problem so they can put it right")
	ErrCantReply    = errors.New("you can reply once to neutral or negative feedback you received")
)

// FieldError is a problem with one input.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

type Service struct {
	DB  *sql.DB
	Rep *sellers.Reputation // re-checks seller levels after feedback changes
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Input is one rating. Buyers rate sellers +1/0/-1 with three 1–5
// sub-ratings; sellers rate buyers +1 or 0.
type Input struct {
	Rating        int    `json:"rating"`
	Comment       string `json:"comment"`
	Item          *int   `json:"item_rating"`
	Shipping      *int   `json:"shipping_rating"`
	Communication *int   `json:"communication_rating"`
}

// Result says what happened to a rating.
type Result struct {
	State   string `json:"state"`   // published, held, mediation
	Visible bool   `json:"visible"` // false until the other side rates or the window closes
	Revised bool   `json:"revised"`
	Note    string `json:"note"`
}

// Extortion: trading feedback for money or goods.
var extortion = []*regexp.Regexp{
	regexp.MustCompile(`(?is)(\$\s?\d+|\b(refund|money back|partial|discount|compensat\w*|pay me|reimburs\w*|credit)\b).{0,60}\b(or|otherwise|unless|else)\b.{0,40}\b(negative|neg|bad|1[- ]?star|low|poor)\b`),
	regexp.MustCompile(`(?is)\b(negative|bad|neg|1[- ]?star)\s+(feedback|review|rating)\b.{0,50}\b(unless|until|if you don'?t|if you do not|if i don'?t get)\b`),
	regexp.MustCompile(`(?is)\b(change|remove|revise|update|raise|fix)\b.{0,30}\b(feedback|review|rating)\b.{0,50}\b(if|once|when|after)\b.{0,40}(\$|\b(refund|pay|send|money|discount|partial)\b)`),
}

// Extortion reports whether text tries to trade feedback for money.
func Extortion(text string) bool {
	for _, re := range extortion {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func validRating(p *int) bool { return p != nil && *p >= 1 && *p <= 5 }

type orderRow struct {
	id, buyer, seller, status string
	number                    int64
	paid, shipped, delivered  sql.NullTime
}

// Leave records (or, during a negative's cooling-off, revises) a rating.
func (s *Service) Leave(ctx context.Context, userID, orderID string, in Input) (Result, error) {
	var o orderRow
	err := s.DB.QueryRowContext(ctx, `SELECT id, buyer_id, seller_id, status, number, paid_at, shipped_at, delivered_at FROM orders
		WHERE (id::text = $2 OR number::text = $2) AND (buyer_id = $1 OR seller_id = $1)`, userID, orderID).
		Scan(&o.id, &o.buyer, &o.seller, &o.status, &o.number, &o.paid, &o.shipped, &o.delivered)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	// Only a paid order gives the right to rate (a verified purchase).
	if !o.paid.Valid || o.status == "pending_payment" {
		return Result{}, ErrNotFound
	}
	if s.now().Sub(o.paid.Time) > Window {
		return Result{}, ErrClosed
	}
	buying := userID == o.buyer
	role, to := "seller_to_buyer", o.buyer
	if buying {
		role, to = "buyer_to_seller", o.seller
	}

	comment := strings.TrimSpace(in.Comment)
	switch {
	case buying && (in.Rating < -1 || in.Rating > 1):
		return Result{}, &FieldError{"rating", "choose positive, neutral or negative"}
	case !buying && (in.Rating < 0 || in.Rating > 1):
		return Result{}, &FieldError{"rating", "sellers can leave buyers positive or neutral feedback"}
	case utf8.RuneCountInString(comment) > CommentMax:
		return Result{}, &FieldError{"comment", "keep it under 500 characters"}
	case buying && in.Rating < 1 && utf8.RuneCountInString(comment) < 5:
		return Result{}, &FieldError{"comment", "say what went wrong (at least a few words)"}
	case inbox.Screen(comment):
		return Result{}, &FieldError{"comment", "leave out contact details and other payment apps"}
	}
	if buying {
		if o.status != "shipped" && o.status != "delivered" && o.status != "refunded" && o.status != "partially_refunded" {
			return Result{}, ErrNotYet
		}
		if !validRating(in.Item) || !validRating(in.Shipping) || !validRating(in.Communication) {
			return Result{}, &FieldError{"item_rating", "rate the item, shipping and communication from 1 to 5"}
		}
	} else {
		in.Item, in.Shipping, in.Communication = nil, nil, nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()

	var existingID, existingState string
	var existingRating int
	err = tx.QueryRowContext(ctx, `SELECT id, state, rating FROM feedback WHERE order_id = $1 AND from_user = $2 AND removed_at IS NULL FOR UPDATE`,
		o.id, userID).Scan(&existingID, &existingState, &existingRating)
	revising := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Result{}, err
	}
	// A negative can be revised during its cooling-off (or mediation), to a
	// better rating only. Anything else is final.
	if revising && (existingState == "published" || in.Rating <= existingRating) {
		return Result{}, ErrAlreadyLeft
	}

	scrubbed := ""
	if Extortion(comment) {
		scrubbed, comment = comment, ""
	}

	state := "published"
	var heldUntil any
	note := ""
	if buying && in.Rating < 0 {
		var contacted bool
		if err := tx.QueryRowContext(ctx, `SELECT
			EXISTS (SELECT 1 FROM cases WHERE order_id = $1 AND opened_by = $2)
			OR EXISTS (SELECT 1 FROM message_threads t JOIN messages m ON m.thread_id = t.id
			           WHERE t.buyer_id = $2 AND t.seller_id = $3 AND m.sender_id = $2 AND m.created_at >= $4)`,
			o.id, o.buyer, o.seller, o.paid.Time).Scan(&contacted); err != nil {
			return Result{}, err
		}
		if !contacted {
			return Result{}, ErrResolveFirst
		}
		var priorSales, priorNegatives int
		if err := tx.QueryRowContext(ctx, `SELECT
			(SELECT count(*) FROM orders WHERE seller_id = $1 AND paid_at IS NOT NULL AND paid_at < $2),
			(SELECT count(*) FROM feedback WHERE to_user = $1 AND role = 'buyer_to_seller' AND rating < 0 AND removed_at IS NULL AND order_id <> $3)`,
			o.seller, o.paid.Time, o.id).Scan(&priorSales, &priorNegatives); err != nil {
			return Result{}, err
		}
		if priorSales < GraceSales && priorNegatives == 0 {
			state = "mediation"
			note = "This is the seller's first negative as a new seller, so our support team reviews it before it shows."
		} else {
			state, heldUntil = "held", s.now().Add(HoldFor)
			note = "Your negative locks in 3 days. If the seller puts things right, you can change it before then."
		}
	}

	var fbID string
	if revising {
		fbID = existingID
		if in.Rating >= 0 {
			state, heldUntil = "published", nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE feedback SET rating = $2, comment = $3, item_rating = $4, shipping_rating = $5,
			communication_rating = $6, state = $7, held_until = $8, scrubbed_comment = nullif($9, ''), updated_at = now()
			WHERE id = $1`, fbID, in.Rating, comment, in.Item, in.Shipping, in.Communication, state, heldUntil, scrubbed); err != nil {
			return Result{}, err
		}
		note = "Thanks for updating your feedback."
	} else {
		if err := tx.QueryRowContext(ctx, `INSERT INTO feedback (order_id, from_user, to_user, role, rating, comment, item_rating,
			shipping_rating, communication_rating, state, held_until, scrubbed_comment)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, nullif($12, '')) RETURNING id`,
			o.id, userID, to, role, in.Rating, comment, in.Item, in.Shipping, in.Communication, state, heldUntil, scrubbed).Scan(&fbID); err != nil {
			return Result{}, err
		}
	}

	if scrubbed != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
			VALUES ('feedback_extortion', 'high', $1, 'Feedback comment tried to trade a rating for money; comment removed',
			jsonb_build_object('order_id', $2::text, 'comment', $3::text), 'feedback_extortion:' || $4::text)
			ON CONFLICT (dedupe_key) DO NOTHING`, userID, o.id, scrubbed, fbID); err != nil {
			return Result{}, err
		}
		note = strings.TrimSpace(note + " Your comment was removed because feedback can't be traded for refunds or payments; the rating stands.")
	}
	if state == "mediation" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO fraud_alerts (kind, severity, subject_user_id, summary, details, dedupe_key)
			VALUES ('feedback_mediation', 'medium', $1, 'New seller''s first negative is waiting for review before it shows',
			jsonb_build_object('order_id', $2::text, 'feedback_id', $3::text), 'feedback_mediation:' || $3::text)
			ON CONFLICT (dedupe_key) DO NOTHING`, o.seller, o.id, fbID); err != nil {
			return Result{}, err
		}
	}

	// Blind: reveal both once both sides have rated.
	var both int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM feedback WHERE order_id = $1 AND removed_at IS NULL`, o.id).Scan(&both); err != nil {
		return Result{}, err
	}
	visible := false
	if both >= 2 {
		if _, err := tx.ExecContext(ctx, `UPDATE feedback SET visible_at = now() WHERE order_id = $1 AND visible_at IS NULL`, o.id); err != nil {
			return Result{}, err
		}
		visible = true
	} else {
		var v sql.NullTime
		tx.QueryRowContext(ctx, `SELECT visible_at FROM feedback WHERE id = $1`, fbID).Scan(&v)
		visible = v.Valid
	}
	if !revising {
		other := "seller"
		if !buying {
			other = "buyer"
		}
		body := fmt.Sprintf("Your %s left feedback for order #%d. It stays hidden until you leave yours (or the feedback window closes).", other, o.number)
		link := "/purchases/" + o.id
		if buying {
			link = "/sell/orders/" + o.id
		}
		if visible {
			body = fmt.Sprintf("Feedback for order #%d is now visible.", o.number)
		}
		if state == "held" {
			body = fmt.Sprintf("The buyer left a negative for order #%d. It locks in 3 days; reply to them in Messages and try to put it right.", o.number)
		}
		if err := inbox.Notify(ctx, tx, to, "order", "New feedback", body, link); err != nil {
			return Result{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	if buying && s.Rep != nil {
		if _, _, err := s.Rep.Review(ctx, o.seller); err != nil {
			return Result{}, err
		}
	}
	if note == "" && !visible {
		note = "Thanks! Your feedback shows once the other side leaves theirs, or when the feedback window closes."
	}
	return Result{State: state, Visible: visible, Revised: revising, Note: note}, nil
}

// Reply posts a seller's one public answer (300 characters) under neutral
// or negative feedback they received.
func (s *Service) Reply(ctx context.Context, userID, feedbackID, text string) error {
	text = strings.TrimSpace(text)
	switch {
	case utf8.RuneCountInString(text) < 2:
		return &FieldError{"reply", "write a short reply"}
	case utf8.RuneCountInString(text) > ReplyMax:
		return &FieldError{"reply", "keep replies to 300 characters"}
	case inbox.Screen(text):
		return &FieldError{"reply", "leave out contact details and other payment apps"}
	case Extortion(text):
		return &FieldError{"reply", "replies can't offer or ask for money in exchange for feedback"}
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE feedback SET reply = $3, reply_at = now(), updated_at = now()
		WHERE id::text = $1 AND to_user = $2 AND role = 'buyer_to_seller' AND rating <= 0 AND reply IS NULL
		  AND removed_at IS NULL AND visible_at IS NOT NULL`, feedbackID, userID, text)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCantReply
	}
	return nil
}

// RunJobs reveals feedback whose window closed, locks negatives after
// their cooling-off, leaves sellers' automatic feedback, and nudges buyers.
func (s *Service) RunJobs(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `
		UPDATE feedback f SET visible_at = now() FROM orders o
		WHERE f.order_id = o.id AND f.visible_at IS NULL
		  AND now() > coalesce(o.delivered_at, o.shipped_at + interval '7 days', o.paid_at + interval '21 days') + $1::interval`,
		fmt.Sprintf("%d seconds", int(RevealAfter.Seconds()))); err != nil {
		return err
	}
	rows, err := s.DB.QueryContext(ctx, `UPDATE feedback SET state = 'published', held_until = NULL, updated_at = now()
		WHERE state = 'held' AND held_until <= now() RETURNING to_user`)
	if err != nil {
		return err
	}
	var locked []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		locked = append(locked, id)
	}
	rows.Close()
	if s.Rep != nil {
		for _, id := range locked {
			if _, _, err := s.Rep.Review(ctx, id); err != nil {
				return err
			}
		}
	}
	if err := s.autoFeedback(ctx); err != nil {
		return err
	}
	return s.nudge(ctx)
}

// autoFeedback leaves positives for sellers who asked for it, when payment
// clears or when the carrier (or buyer) confirms delivery.
func (s *Service) autoFeedback(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT o.id, o.seller_id, o.buyer_id, sp.auto_feedback_comment FROM orders o
		JOIN seller_profiles sp ON sp.user_id = o.seller_id
		WHERE o.paid_at > now() - interval '60 days'
		  AND ((sp.auto_feedback = 'paid' AND o.status IN ('paid', 'shipped', 'delivered'))
		    OR (sp.auto_feedback = 'delivered' AND o.status = 'delivered'))
		  AND NOT EXISTS (SELECT 1 FROM feedback f WHERE f.order_id = o.id AND f.from_user = o.seller_id)
		LIMIT 200`)
	if err != nil {
		return err
	}
	type job struct{ order, seller, buyer, comment string }
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.order, &j.seller, &j.buyer, &j.comment); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	for _, j := range jobs {
		c := strings.TrimSpace(j.comment)
		if inbox.Screen(c) || Extortion(c) || utf8.RuneCountInString(c) > CommentMax {
			c = "Great buyer. Thank you!"
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO feedback (order_id, from_user, to_user, role, rating, comment, auto)
			VALUES ($1, $2, $3, 'seller_to_buyer', 1, $4, true) ON CONFLICT (order_id, from_user) DO NOTHING`,
			j.order, j.seller, j.buyer, c); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE feedback SET visible_at = now() WHERE order_id = $1 AND visible_at IS NULL
			AND (SELECT count(*) FROM feedback WHERE order_id = $1 AND removed_at IS NULL) >= 2`, j.order); err != nil {
			return err
		}
	}
	return nil
}

// nudge reminds buyers, once, to rate an order three days after it arrived.
func (s *Service) nudge(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `
		UPDATE orders o SET feedback_nudged_at = now()
		WHERE o.feedback_nudged_at IS NULL AND o.status IN ('shipped', 'delivered')
		  AND coalesce(o.delivered_at, o.shipped_at + interval '7 days') < now() - interval '3 days'
		  AND o.paid_at > now() - interval '55 days'
		  AND NOT EXISTS (SELECT 1 FROM feedback f WHERE f.order_id = o.id AND f.from_user = o.buyer_id)
		  AND NOT EXISTS (SELECT 1 FROM cases c WHERE c.order_id = o.id AND c.status IN ('open', 'waiting_seller', 'waiting_buyer'))
		RETURNING o.id, o.number, o.buyer_id, o.seller_id`)
	if err != nil {
		return err
	}
	type n struct {
		id, buyer, seller string
		number            int64
	}
	var list []n
	for rows.Next() {
		var x n
		if err := rows.Scan(&x.id, &x.number, &x.buyer, &x.seller); err != nil {
			rows.Close()
			return err
		}
		list = append(list, x)
	}
	rows.Close()
	for _, x := range list {
		var sales int
		var name string
		if err := s.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM orders WHERE seller_id = $1 AND paid_at IS NOT NULL),
			(SELECT username FROM users WHERE id = $1)`, x.seller).Scan(&sales, &name); err != nil {
			return err
		}
		body := fmt.Sprintf("How did order #%d go? A quick rating helps other collectors.", x.number)
		if sales <= 25 {
			body = fmt.Sprintf("How did order #%d go? %s is a newer seller here, so your rating really helps them get started.", x.number, name)
		}
		if err := inbox.Notify(ctx, s.DB, x.buyer, "order", "Rate your purchase", body, "/purchases/"+x.id); err != nil {
			return err
		}
	}
	return nil
}
