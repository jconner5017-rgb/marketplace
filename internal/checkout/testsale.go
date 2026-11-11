package checkout

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jconner5017-rgb/marketplace/internal/shipping"
)

// Test sales let staff walk through the seller side of an order (print the
// tracked envelope, mark it shipped) on the live site without anyone paying.
// The order is marked test, has no listing, costs nothing, and is "bought" by
// an internal account nobody can sign in to. It ships to the seller's own
// ship-from address, so a test envelope that does get mailed comes back.
const (
	TestBuyerEmail    = "test-buyer@example.invalid"
	TestBuyerUsername = "sns_test_buyer"
	testSaleTitle     = "TEST SALE - sample card (not a real order)"
	testSaleCents     = 100
)

var (
	ErrTestNoShipFrom = errors.New("add your ship-from address in Seller settings first; the test envelope is addressed to you")
	ErrNotSellerYet   = errors.New("this account isn't set up to sell yet")
)

// TestSale is one open test order.
type TestSale struct {
	ID     string `json:"id"`
	Number int64  `json:"number"`
	Status string `json:"status"`
}

func (s *Service) testBuyer(ctx context.Context, tx *sql.Tx) (string, error) {
	var pw [24]byte
	rand.Read(pw[:])
	var id string
	// "!" + random can never match a password, so nobody can sign in as it.
	err := tx.QueryRowContext(ctx, `
		INSERT INTO users (email, username, password_hash, email_verified_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (lower(email)) DO UPDATE SET updated_at = now()
		RETURNING id`, TestBuyerEmail, TestBuyerUsername, "!"+hex.EncodeToString(pw[:])).Scan(&id)
	return id, err
}

// CreateTestSale adds a paid, envelope-shipped test order to sellerID's orders.
func (s *Service) CreateTestSale(ctx context.Context, sellerID string) (TestSale, error) {
	var from sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT ship_from FROM seller_profiles WHERE user_id = $1`, sellerID).Scan(&from)
	if errors.Is(err, sql.ErrNoRows) {
		return TestSale{}, ErrNotSellerYet
	}
	if err != nil {
		return TestSale{}, err
	}
	var addr shipping.Address
	if !from.Valid || json.Unmarshal([]byte(from.String), &addr) != nil || addr.Street1 == "" || addr.Zip == "" {
		return TestSale{}, ErrTestNoShipFrom
	}
	addr.Name = strings.TrimSpace("Test Buyer " + addr.Name)
	addr.Phone, addr.Email = "", ""
	shipTo, _ := json.Marshal(addr)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TestSale{}, err
	}
	defer tx.Rollback()
	buyer, err := s.testBuyer(ctx, tx)
	if err != nil {
		return TestSale{}, fmt.Errorf("test buyer: %w", err)
	}
	var t TestSale
	err = tx.QueryRowContext(ctx, `
		INSERT INTO orders (buyer_id, seller_id, status, subtotal_cents, total_cents, fee_cents, ship_method, ship_to,
		                    ship_by, paid_at, test)
		VALUES ($1, $2, 'paid', $3, $3, 0, 'envelope', $4, now() + interval '3 days', now(), true)
		RETURNING id, number, status`, buyer, sellerID, testSaleCents, shipTo).Scan(&t.ID, &t.Number, &t.Status)
	if err != nil {
		return TestSale{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO order_items (order_id, source, title, unit_cents, quantity) VALUES ($1, 'cart', $2, $3, 1)`,
		t.ID, testSaleTitle, testSaleCents); err != nil {
		return TestSale{}, err
	}
	return t, tx.Commit()
}

// TestSales lists sellerID's test sales that haven't been deleted.
func (s *Service) TestSales(ctx context.Context, sellerID string) ([]TestSale, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT o.id, o.number, o.status FROM orders o JOIN users b ON b.id = o.buyer_id
		WHERE o.seller_id = $1 AND o.test AND lower(b.email) = lower($2) ORDER BY o.created_at DESC`, sellerID, TestBuyerEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TestSale{}
	for rows.Next() {
		var t TestSale
		if err := rows.Scan(&t.ID, &t.Number, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTestSale removes one of sellerID's test sales and everything hanging
// off it (barcode, shipment, feedback). Real orders can't be removed here.
func (s *Service) DeleteTestSale(ctx context.Context, sellerID, orderID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT o.id FROM orders o JOIN users b ON b.id = o.buyer_id
		WHERE o.id::text = $1 AND o.seller_id = $2 AND o.test AND lower(b.email) = lower($3)`,
		orderID, sellerID, TestBuyerEmail).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var ledger int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ledger_entries WHERE order_id = $1`, id).Scan(&ledger); err != nil {
		return err
	}
	steps := []string{
		`DELETE FROM feedback WHERE order_id = $1`,
		`DELETE FROM cases WHERE order_id = $1`,
		`DELETE FROM oversells WHERE order_id = $1`,
		`DELETE FROM order_promo_uses WHERE order_id = $1`,
	}
	if ledger > 0 {
		// No money moved on a test sale; the append-only lock is lifted only
		// inside this transaction and put back before commit.
		steps = append(steps, `ALTER TABLE ledger_entries DISABLE TRIGGER ledger_no_update`,
			`DELETE FROM ledger_entries WHERE order_id = $1`, `ALTER TABLE ledger_entries ENABLE TRIGGER ledger_no_update`)
	}
	steps = append(steps, `DELETE FROM orders WHERE id = $1`)
	for _, q := range steps {
		if strings.HasPrefix(q, "ALTER") {
			_, err = tx.ExecContext(ctx, q)
		} else {
			_, err = tx.ExecContext(ctx, q, id)
		}
		if err != nil {
			return fmt.Errorf("delete test sale: %w", err)
		}
	}
	return tx.Commit()
}
