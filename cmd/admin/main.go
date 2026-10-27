// Command admin runs one-off account maintenance tasks.
//
//	go run ./cmd/admin promote you@example.com   # make a user an admin
//	go run ./cmd/admin demote  you@example.com   # back to a regular user
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jconner5017-rgb/marketplace/internal/auth"
	"github.com/jconner5017-rgb/marketplace/internal/config"
	"github.com/jconner5017-rgb/marketplace/internal/db"
)

func main() {
	if len(os.Args) != 3 || (os.Args[1] != "promote" && os.Args[1] != "demote") {
		fmt.Fprintln(os.Stderr, "usage: admin promote|demote <email>")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	defer database.Close()
	if err := db.Migrate(ctx, database); err != nil {
		fail(err)
	}

	role := "admin"
	if os.Args[1] == "demote" {
		role = "user"
	}
	svc := &auth.Service{DB: database, SessionTTL: cfg.SessionTTL}
	if err := svc.SetRole(ctx, os.Args[2], role); err != nil {
		fail(err)
	}
	fmt.Printf("%s is now %s\n", os.Args[2], role)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
