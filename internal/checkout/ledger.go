package checkout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jconner5017-rgb/marketplace/internal/inbox"
)

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// ledger appends one money event. A repeat of the same Stripe object and
// kind is ignored, so webhook retries are safe.
func ledger(ctx context.Context, ex execer, orderID, sellerID, kind string, cents int, stripeObj, note string) error {
	var obj any
	if stripeObj != "" {
		obj = stripeObj
	}
	_, err := ex.ExecContext(ctx, `INSERT INTO ledger_entries (order_id, seller_id, kind, amount_cents, stripe_object, note)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (kind, stripe_object) DO NOTHING`,
		orderID, sellerID, kind, cents, obj, note)
	return err
}

// LedgerEntry is one row of an order's money history.
type LedgerEntry struct {
	Kind         string `json:"kind"`
	AmountCents  int    `json:"amount_cents"`
	StripeObject string `json:"stripe_object"`
	Note         string `json:"note"`
	CreatedAt    string `json:"created_at"`
}

// Stripe object shapes we read from webhooks.
type stripeCharge struct {
	ID             string `json:"id"`
	PaymentIntent  string `json:"payment_intent"`
	Amount         int    `json:"amount"`
	AmountRefunded int    `json:"amount_refunded"`
}

type stripeDispute struct {
	ID            string `json:"id"`
	Amount        int    `json:"amount"`
	Charge        string `json:"charge"`
	PaymentIntent string `json:"payment_intent"`
	Reason        string `json:"reason"`
	Status        string `json:"status"`
}

type stripeAppFee struct {
	ID             string `json:"id"`
	Charge         string `json:"charge"`
	AmountRefunded int    `json:"amount_refunded"`
}

type orderRef struct {
	id, buyer, seller, status string
	number                    int64
	total, refunded, fee      int
}

func (s *Service) orderBy(ctx context.Context, tx *sql.Tx, where string, args ...any) (orderRef, error) {
	var o orderRef
	err := tx.QueryRowContext(ctx, `SELECT id, buyer_id, seller_id, status, number, total_cents, refunded_cents, fee_cents
		FROM orders WHERE `+where+` FOR UPDATE`, args...).
		Scan(&o.id, &o.buyer, &o.seller, &o.status, &o.number, &o.total, &o.refunded, &o.fee)
	return o, err
}

// HandleMoneyEvent records refunds, application fee refunds and chargebacks
// from Stripe. It reports whether the event type was one of these.
func (s *Service) HandleMoneyEvent(ctx context.Context, eventType, account string, obj json.RawMessage) (bool, error) {
	switch eventType {
	case "charge.refunded":
		var c stripeCharge
		if err := json.Unmarshal(obj, &c); err != nil || c.ID == "" {
			return true, fmt.Errorf("read charge: %v", err)
		}
		return true, s.recordRefund(ctx, account, c)
	case "application_fee.refunded":
		var f stripeAppFee
		if err := json.Unmarshal(obj, &f); err != nil || f.ID == "" {
			return true, fmt.Errorf("read application fee: %v", err)
		}
		return true, s.recordFeeRefund(ctx, f)
	case "charge.dispute.created", "charge.dispute.closed":
		var d stripeDispute
		if err := json.Unmarshal(obj, &d); err != nil || d.ID == "" {
			return true, fmt.Errorf("read dispute: %v", err)
		}
		return true, s.recordDispute(ctx, account, eventType == "charge.dispute.created", d)
	}
	return false, nil
}

func (s *Service) recordRefund(ctx context.Context, account string, c stripeCharge) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := s.orderBy(ctx, tx, `(stripe_payment_intent = $1 OR stripe_charge = $2) AND ($3 = '' OR stripe_account = $3)`,
		c.PaymentIntent, c.ID, account)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // not one of our orders
	}
	if err != nil {
		return err
	}
	delta := c.AmountRefunded - o.refunded
	if delta <= 0 {
		return tx.Commit()
	}
	ref := c.ID + ":" + strconv.Itoa(c.AmountRefunded)
	status, kind := "partially_refunded", "partial_refund"
	if c.AmountRefunded >= o.total {
		status, kind = "refunded", "refund"
	}
	if err := ledger(ctx, tx, o.id, o.seller, "refund", delta, ref, "Refunded by the seller in Stripe"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO refund_reports (order_id, source, kind, amount_cents, stripe_ref, note, status)
		VALUES ($1, 'stripe', $2, $3, $4, 'Recorded from Stripe', 'closed') ON CONFLICT DO NOTHING`,
		o.id, kind, delta, ref); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET refunded_cents = $2, status = $3, stripe_charge = $4, updated_at = now()
		WHERE id = $1`, o.id, c.AmountRefunded, status, c.ID); err != nil {
		return err
	}
	if err := inbox.Notify(ctx, tx, o.buyer, "order", fmt.Sprintf("Refund on order #%d", o.number),
		fmt.Sprintf("The seller refunded %s. It usually reaches your card in 5 to 10 days.", money(delta)),
		"/purchases/"+o.id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) recordFeeRefund(ctx context.Context, f stripeAppFee) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := s.orderBy(ctx, tx, `stripe_charge = $1`, f.Charge)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var already int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(sum(amount_cents), 0) FROM ledger_entries
		WHERE order_id = $1 AND kind = 'fee_refund'`, o.id).Scan(&already); err != nil {
		return err
	}
	if delta := f.AmountRefunded - already; delta > 0 {
		if err := ledger(ctx, tx, o.id, o.seller, "fee_refund", delta, f.ID+":"+strconv.Itoa(f.AmountRefunded),
			"Marketplace fee returned with the refund"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) recordDispute(ctx context.Context, account string, opened bool, d stripeDispute) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := s.orderBy(ctx, tx, `(stripe_payment_intent = $1 OR stripe_charge = $2) AND ($3 = '' OR stripe_account = $3)`,
		d.PaymentIntent, d.Charge, account)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if d.Charge != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET stripe_charge = $2 WHERE id = $1 AND stripe_charge IS NULL`, o.id, d.Charge); err != nil {
			return err
		}
	}
	if opened {
		if err := ledger(ctx, tx, o.id, o.seller, "chargeback_opened", d.Amount, d.ID, "Reason: "+d.Reason); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO refund_reports (order_id, source, kind, amount_cents, stripe_ref, note, status)
			VALUES ($1, 'stripe', 'chargeback', $2, $3, $4, 'open') ON CONFLICT DO NOTHING`,
			o.id, d.Amount, d.ID, "Card dispute: "+d.Reason); err != nil {
			return err
		}
		if err := inbox.Notify(ctx, tx, o.seller, "order", fmt.Sprintf("Chargeback on order #%d", o.number),
			fmt.Sprintf("The buyer disputed %s with their card issuer (%s). Download the evidence packet from the order and respond in your Stripe dashboard.",
				money(d.Amount), d.Reason), "/sell/orders/"+o.id); err != nil {
			return err
		}
		return tx.Commit()
	}

	kind, outcome := "", ""
	switch d.Status {
	case "won":
		kind, outcome = "chargeback_won", "won"
	case "lost":
		kind, outcome = "chargeback_lost", "lost"
	default: // warning_closed and the like: nothing changes hands
		if _, err := tx.ExecContext(ctx, `UPDATE refund_reports SET status = 'closed' WHERE kind = 'chargeback' AND stripe_ref = $1`, d.ID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err := ledger(ctx, tx, o.id, o.seller, kind, d.Amount, d.ID, ""); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE refund_reports SET status = $2 WHERE kind = 'chargeback' AND stripe_ref = $1`, d.ID, outcome); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO refund_reports (order_id, source, kind, amount_cents, stripe_ref, note, status)
		VALUES ($1, 'stripe', $2, $3, $4, 'Dispute closed', $5) ON CONFLICT DO NOTHING`, o.id, kind, d.Amount, d.ID, outcome)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 && outcome == "won" {
		// A chargeback the seller wins counts against the buyer.
		if _, err := tx.ExecContext(ctx, `UPDATE users SET buyer_strikes = buyer_strikes + 1 WHERE id = $1`, o.buyer); err != nil {
			return err
		}
	}
	msg := "The card issuer sided with you; the funds are returned to your Stripe balance."
	if outcome == "lost" {
		msg = "The card issuer sided with the buyer."
	}
	if err := inbox.Notify(ctx, tx, o.seller, "order", fmt.Sprintf("Chargeback %s on order #%d", outcome, o.number), msg,
		"/sell/orders/"+o.id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if outcome == "lost" && s.Rep != nil {
		_, _, err = s.Rep.Review(ctx, o.seller)
	}
	return err
}

// Ledger lists an order's money events, oldest first.
func (s *Service) Ledger(ctx context.Context, orderID string) ([]LedgerEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT kind, amount_cents, coalesce(stripe_object, ''), note, created_at::text
		FROM ledger_entries WHERE order_id = $1 ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LedgerEntry{}
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.Kind, &e.AmountCents, &e.StripeObject, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
