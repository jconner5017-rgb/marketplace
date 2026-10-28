// Package auth handles accounts, passwords and login sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountDisabled    = errors.New("account is suspended or banned")
	ErrNoSession          = errors.New("not logged in")
	ErrNotFound           = errors.New("user not found")
)

// ValidationError lists problems with submitted fields, keyed by field name.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid input" }

type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Username      string    `json:"username"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}

func (u User) IsAdmin() bool { return u.Role == "admin" }

type Service struct {
	DB         *sql.DB
	SessionTTL time.Duration
	// AdminEmails become admins as soon as they register (ADMIN_EMAILS).
	AdminEmails []string
}

func (s *Service) isAdminEmail(email string) bool {
	for _, a := range s.AdminEmails {
		if strings.EqualFold(strings.TrimSpace(a), email) {
			return true
		}
	}
	return false
}

// PromoteAdmins makes every registered ADMIN_EMAILS account an admin.
func (s *Service) PromoteAdmins(ctx context.Context) error {
	for _, a := range s.AdminEmails {
		if a = strings.TrimSpace(a); a == "" {
			continue
		}
		if err := s.SetRole(ctx, a, "admin"); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_]{3,30}$`)

const (
	minPasswordLen = 10
	maxPasswordLen = 128
)

func validateSignup(email, username, password string) error {
	f := map[string]string{}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
		f["email"] = "enter a valid email address"
	}
	if !usernameRE.MatchString(username) {
		f["username"] = "3-30 characters: letters, numbers, underscore"
	}
	if n := utf8.RuneCountInString(password); n < minPasswordLen || n > maxPasswordLen {
		f["password"] = "must be 10 to 128 characters"
	}
	if len(f) > 0 {
		return &ValidationError{Fields: f}
	}
	return nil
}

const userCols = `id, email, username, role, status, email_verified_at IS NOT NULL, created_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Role, &u.Status, &u.EmailVerified, &u.CreatedAt)
	return u, err
}

// Signup creates a buyer account.
func (s *Service) Signup(ctx context.Context, email, username, password string) (User, error) {
	email = strings.TrimSpace(email)
	username = strings.TrimSpace(username)
	if err := validateSignup(email, username, password); err != nil {
		return User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	u, err := scanUser(s.DB.QueryRowContext(ctx, `
		INSERT INTO users (email, username, password_hash)
		VALUES ($1, $2, $3)
		RETURNING `+userCols, email, username, hash))
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" { // unique_violation
			switch pqErr.Constraint {
			case "users_email_key":
				return User{}, ErrEmailTaken
			case "users_username_key":
				return User{}, ErrUsernameTaken
			}
		}
		return User{}, err
	}
	if s.isAdminEmail(u.Email) {
		if err := s.SetRole(ctx, u.Email, "admin"); err != nil {
			return User{}, err
		}
		u.Role = "admin"
	}
	return u, nil
}

// dummyHash is checked when an email isn't found, so a failed login takes
// the same time whether or not the account exists.
var dummyHash, _ = HashPassword("not-a-real-password")

// Login checks credentials and returns the user plus a new session token.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (User, string, error) {
	var hash string
	var u User
	err := s.DB.QueryRowContext(ctx, `
		SELECT `+userCols+`, password_hash FROM users WHERE lower(email) = lower($1)`,
		strings.TrimSpace(email),
	).Scan(&u.ID, &u.Email, &u.Username, &u.Role, &u.Status, &u.EmailVerified, &u.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		VerifyPassword(password, dummyHash)
		return User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	if !VerifyPassword(password, hash) {
		return User{}, "", ErrInvalidCredentials
	}
	if u.Status != "active" {
		return User{}, "", ErrAccountDisabled
	}
	token, err := s.createSession(ctx, u.ID, ip, userAgent)
	if err != nil {
		return User{}, "", err
	}
	return u, token, nil
}

// StartSession logs a user in directly (used right after signup).
func (s *Service) StartSession(ctx context.Context, userID, ip, userAgent string) (string, error) {
	return s.createSession(ctx, userID, ip, userAgent)
}

func (s *Service) createSession(ctx context.Context, userID, ip, userAgent string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5)`,
		hashToken(token), userID, time.Now().Add(s.SessionTTL), ip, userAgent)
	if err != nil {
		return "", err
	}
	return token, nil
}

// UserForSession returns the logged-in user for a session token.
func (s *Service) UserForSession(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNoSession
	}
	u, err := scanUser(s.DB.QueryRowContext(ctx, `
		SELECT `+prefixed("u.", userCols)+`
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND u.status = 'active'`,
		hashToken(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoSession
	}
	return u, err
}

// Logout ends one session.
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

// LogoutEverywhere ends every session for a user (used on ban or password change).
func (s *Service) LogoutEverywhere(ctx context.Context, userID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

// PurgeExpiredSessions removes expired rows; run periodically.
func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetRole changes a user's role ("user" or "admin").
func (s *Service) SetRole(ctx context.Context, email, role string) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE users SET role = $2, updated_at = now() WHERE lower(email) = lower($1)`, email, role)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUsers returns the newest accounts first, for the admin view.
func (s *Service) ListUsers(ctx context.Context, limit int) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+userCols+` FROM users ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// prefixed qualifies each column in a comma list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ", ")
	for i, p := range parts {
		parts[i] = alias + p
	}
	return strings.Join(parts, ", ")
}
