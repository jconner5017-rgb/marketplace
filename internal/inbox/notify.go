package inbox

import (
	"context"
	"database/sql"
)

// Notify adds a bell notification inside the caller's transaction (or on the DB).
func Notify(ctx context.Context, ex interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, userID, kind, title, body, link string) error {
	_, err := ex.ExecContext(ctx, `
		INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, $2, $3, $4, NULLIF($5, ''))`,
		userID, kind, title, body, link)
	return err
}
