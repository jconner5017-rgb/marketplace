package admin

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestFraudSQL runs every check against the test database to catch SQL errors.
func TestFraudSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip()
	}
	d, _ := sql.Open("postgres", url)
	defer d.Close()
	if _, err := (&Fraud{DB: d}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
