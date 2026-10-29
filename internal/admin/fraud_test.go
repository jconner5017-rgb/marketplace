package admin

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/jconner5017-rgb/marketplace/internal/db"
)

// TestFraudSQL runs every check against the test database to catch SQL errors.
func TestFraudSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip()
	}
	d, _ := sql.Open("postgres", url)
	defer d.Close()
	// Packages are tested in alphabetical order, so on a fresh database this
	// one runs before anything else has created the tables.
	if err := db.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Fraud{DB: d}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
