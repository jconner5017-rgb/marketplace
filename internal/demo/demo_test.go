package demo

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/jconner5017-rgb/marketplace/internal/db"
	"github.com/jconner5017-rgb/marketplace/internal/listings"
	"github.com/jconner5017-rgb/marketplace/internal/sellers"
)

// A won sample auction and its buyer's hold survive the next seed (which
// runs on every API start) while a fresh auction takes its place.
func TestSeedKeepsWonAuctions(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip()
	}
	ctx := context.Background()
	d, _ := sql.Open("postgres", url)
	defer d.Close()
	if err := db.Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	svc := &listings.Service{DB: d, Sellers: &sellers.Service{DB: d}}
	if _, err := Seed(ctx, d, svc, "https://example.test"); err != nil {
		t.Fatal(err)
	}
	var buyer string
	if err := d.QueryRow(`INSERT INTO users (email, username, password_hash) VALUES ('demo-winner@example.test', 'demo_winner', '!x')
		ON CONFLICT (lower(email)) DO UPDATE SET updated_at = now() RETURNING id`).Scan(&buyer); err != nil {
		t.Fatal(err)
	}
	var lid, seller string
	if err := d.QueryRow(`SELECT id, seller_id FROM listings WHERE external_id = 'DEMO-AUC-001'`).Scan(&lid, &seller); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE listings SET auction_state = 'won', winner_id = $2, status = 'ended', quantity = 0 WHERE id = $1`, lid, buyer); err != nil {
		t.Fatal(err)
	}
	var hold string
	if err := d.QueryRow(`INSERT INTO holds (listing_id, buyer_id, seller_id, source, quantity, unit_cents, due_at)
		VALUES ($1, $2, $3, 'auction', 1, 500, now() + interval '4 days') RETURNING id`, lid, buyer, seller).Scan(&hold); err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, d, svc, "https://example.test"); err != nil {
		t.Fatal(err)
	}
	var n int
	d.QueryRow(`SELECT count(*) FROM holds WHERE id = $1`, hold).Scan(&n)
	if n != 1 {
		t.Fatal("seed deleted the winner's hold")
	}
	var live bool
	if err := d.QueryRow(`SELECT auction_state = 'live' FROM listings WHERE external_id = 'DEMO-AUC-001'`).Scan(&live); err != nil || !live {
		t.Fatalf("no fresh sample auction: %v", err)
	}
	d.Exec(`DELETE FROM users WHERE id = $1`, buyer)
}
