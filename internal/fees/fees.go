// Package fees holds the marketplace fee schedule and paid listing upgrades.
package fees

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Schedule: flat fee up to the threshold, then a percentage of the price,
// never below the flat fee or above the cap.
type Schedule struct {
	ID             int       `json:"id"`
	FlatCents      int       `json:"flat_cents"`
	ThresholdCents int       `json:"threshold_cents"`
	PercentBps     int       `json:"percent_bps"`
	CapCents       int       `json:"cap_cents"`
	EffectiveAt    time.Time `json:"effective_at"`
	Note           string    `json:"note"`
	CreatedBy      *string   `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Default matches the launch fees, used if the table can't be read.
var Default = Schedule{FlatCents: 150, ThresholdCents: 10000, PercentBps: 300, CapCents: 2500}

// For returns our fee on one card sold at priceCents.
func (s Schedule) For(priceCents int) int {
	if priceCents <= s.ThresholdCents {
		return min(s.FlatCents, priceCents)
	}
	f := priceCents * s.PercentBps / 10000
	return min(max(f, s.FlatCents), s.CapCents, priceCents)
}

type Service struct{ DB *sql.DB }

const cols = `id, flat_cents, threshold_cents, percent_bps, cap_cents, effective_at, note, created_by::text, created_at`

func scan(row interface{ Scan(...any) error }) (Schedule, error) {
	var s Schedule
	var by sql.NullString
	err := row.Scan(&s.ID, &s.FlatCents, &s.ThresholdCents, &s.PercentBps, &s.CapCents, &s.EffectiveAt, &s.Note, &by, &s.CreatedAt)
	if by.Valid {
		s.CreatedBy = &by.String
	}
	return s, err
}

// At returns the schedule in force at a time (a sale's time, for orders).
func (f *Service) At(ctx context.Context, t time.Time) (Schedule, error) {
	s, err := scan(f.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM fee_schedules WHERE effective_at <= $1
		ORDER BY effective_at DESC, id DESC LIMIT 1`, t))
	if errors.Is(err, sql.ErrNoRows) {
		return Default, nil
	}
	return s, err
}

// Current is the schedule in force now.
func (f *Service) Current(ctx context.Context) (Schedule, error) { return f.At(ctx, time.Now()) }

// Upcoming lists schedules that start in the future.
func (f *Service) Upcoming(ctx context.Context) ([]Schedule, error) {
	return f.list(ctx, `WHERE effective_at > now() ORDER BY effective_at`)
}

// History lists every schedule, newest first.
func (f *Service) History(ctx context.Context) ([]Schedule, error) {
	return f.list(ctx, `ORDER BY effective_at DESC, id DESC`)
}

func (f *Service) list(ctx context.Context, tail string) ([]Schedule, error) {
	rows, err := f.DB.QueryContext(ctx, `SELECT `+cols+` FROM fee_schedules `+tail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FieldError names a bad field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

// Add schedules new fees. They can't start in the past, so sales already
// made always keep the fees they were made under.
func (f *Service) Add(ctx context.Context, tx *sql.Tx, s Schedule, by string) (Schedule, error) {
	switch {
	case s.FlatCents < 0 || s.FlatCents > 10000:
		return s, &FieldError{"flat_cents", "flat fee must be $0 to $100"}
	case s.ThresholdCents < 0 || s.ThresholdCents > 100000:
		return s, &FieldError{"threshold_cents", "threshold must be $0 to $1,000"}
	case s.PercentBps < 0 || s.PercentBps > 5000:
		return s, &FieldError{"percent_bps", "percentage must be 0% to 50%"}
	case s.CapCents < s.FlatCents || s.CapCents > 100000:
		return s, &FieldError{"cap_cents", "cap must be at least the flat fee and at most $1,000"}
	case s.EffectiveAt.Before(time.Now().Add(-time.Minute)):
		return s, &FieldError{"effective_at", "new fees can't start in the past"}
	}
	s.Note = strings.TrimSpace(s.Note)
	var byArg any
	if by != "" {
		byArg = by
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO fee_schedules (flat_cents, threshold_cents, percent_bps, cap_cents, effective_at, note, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+cols,
		s.FlatCents, s.ThresholdCents, s.PercentBps, s.CapCents, s.EffectiveAt, s.Note, byArg).Scan(
		&s.ID, &s.FlatCents, &s.ThresholdCents, &s.PercentBps, &s.CapCents, &s.EffectiveAt, &s.Note, new(sql.NullString), &s.CreatedAt)
	return s, err
}

// CancelUpcoming removes a schedule that hasn't started yet.
func (f *Service) CancelUpcoming(ctx context.Context, tx *sql.Tx, id int) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM fee_schedules WHERE id = $1 AND effective_at > now()`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &FieldError{"id", "only fees that haven't started can be cancelled"}
	}
	return nil
}

// Upgrade is an optional paid listing upgrade.
type Upgrade struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	PriceCents  int    `json:"price_cents"`
	Active      bool   `json:"active"`
}

func (f *Service) Upgrades(ctx context.Context) ([]Upgrade, error) {
	rows, err := f.DB.QueryContext(ctx, `SELECT key, label, description, price_cents, active FROM fee_upgrades ORDER BY price_cents, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Upgrade{}
	for rows.Next() {
		var u Upgrade
		if err := rows.Scan(&u.Key, &u.Label, &u.Description, &u.PriceCents, &u.Active); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUpgrade changes an upgrade's price, wording or availability; it
// returns the old version.
func (f *Service) UpdateUpgrade(ctx context.Context, tx *sql.Tx, u Upgrade) (Upgrade, error) {
	var old Upgrade
	err := tx.QueryRowContext(ctx, `SELECT key, label, description, price_cents, active FROM fee_upgrades WHERE key = $1 FOR UPDATE`, u.Key).
		Scan(&old.Key, &old.Label, &old.Description, &old.PriceCents, &old.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return old, &FieldError{"key", "unknown upgrade"}
	}
	if err != nil {
		return old, err
	}
	if u.PriceCents < 0 || u.PriceCents > 10000 {
		return old, &FieldError{"price_cents", "price must be $0 to $100"}
	}
	if strings.TrimSpace(u.Label) == "" {
		u.Label = old.Label
	}
	_, err = tx.ExecContext(ctx, `UPDATE fee_upgrades SET label = $2, description = $3, price_cents = $4, active = $5, updated_at = now()
		WHERE key = $1`, u.Key, strings.TrimSpace(u.Label), strings.TrimSpace(u.Description), u.PriceCents, u.Active)
	return old, err
}
