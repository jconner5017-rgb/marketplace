// Package fcm connects sellers' inventory tools (FireCardManager) to the
// marketplace: API keys, idempotent writes, signed webhooks and nightly
// inventory reconciliation.
package fcm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"time"
)

var (
	ErrBadKey   = errors.New("invalid or revoked API key")
	ErrNotFound = errors.New("not found")
	ErrTooMany  = errors.New("you can have up to 5 active API keys; revoke one first")
)

const keyPrefix = "ss_key_"

// Key is an API key as listed (never the secret itself).
type Key struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

type Service struct {
	DB *sql.DB
	// HTTP sends webhooks and inventory requests; nil = SafeClient().
	HTTP HTTPDoer
	// AllowPrivate lets webhooks reach private and loopback addresses (tests only).
	AllowPrivate bool
	Now          func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func randomString(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[x.Int64()]
	}
	return string(b)
}

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

// CreateKey makes a new API key for a seller. The key is returned once.
func (s *Service) CreateKey(ctx context.Context, userID, name string) (string, Key, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "FireCardManager"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	var active int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM api_keys WHERE user_id = $1 AND revoked_at IS NULL`, userID).Scan(&active); err != nil {
		return "", Key{}, err
	}
	if active >= 5 {
		return "", Key{}, ErrTooMany
	}
	plain := keyPrefix + randomString(40)
	var k Key
	err := s.DB.QueryRowContext(ctx, `INSERT INTO api_keys (user_id, name, prefix, key_hash) VALUES ($1, $2, $3, $4)
		RETURNING id, name, prefix, created_at`, userID, name, plain[:len(keyPrefix)+6], hashKey(plain)).
		Scan(&k.ID, &k.Name, &k.Prefix, &k.CreatedAt)
	if err != nil {
		return "", Key{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE seller_profiles SET fcm_connected = true, updated_at = now() WHERE user_id = $1`, userID); err != nil {
		return "", Key{}, err
	}
	return plain, k, nil
}

func (s *Service) Keys(ctx context.Context, userID string) ([]Key, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, prefix, created_at, last_used_at, revoked_at FROM api_keys
		WHERE user_id = $1 ORDER BY revoked_at NULLS FIRST, created_at DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Service) RevokeKey(ctx context.Context, userID, id string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE api_keys SET revoked_at = now() WHERE id::text = $1 AND user_id = $2 AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE seller_profiles SET fcm_connected = EXISTS (
		SELECT 1 FROM api_keys WHERE user_id = $1 AND revoked_at IS NULL), updated_at = now() WHERE user_id = $1`, userID)
	return err
}

// Authenticate returns the seller an API key belongs to.
func (s *Service) Authenticate(ctx context.Context, token string) (string, error) {
	if !strings.HasPrefix(token, keyPrefix) || len(token) > 100 {
		return "", ErrBadKey
	}
	var userID string
	err := s.DB.QueryRowContext(ctx, `UPDATE api_keys SET last_used_at = now()
		WHERE key_hash = $1 AND revoked_at IS NULL RETURNING user_id`, hashKey(token)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrBadKey
	}
	return userID, err
}

// Remembered is a stored answer to an idempotent request.
type Remembered struct {
	Status int
	Body   []byte
}

// ErrKeyReused means the Idempotency-Key was used for a different request.
var ErrKeyReused = errors.New("this Idempotency-Key was already used for a different request")

// Recall returns the stored answer for an Idempotency-Key, if any.
func (s *Service) Recall(ctx context.Context, userID, key, method, path, reqHash string) (*Remembered, error) {
	var r Remembered
	var m, p, h string
	err := s.DB.QueryRowContext(ctx, `SELECT method, path, request_hash, status, response FROM idempotency_keys
		WHERE user_id = $1 AND key = $2 AND created_at > now() - interval '24 hours'`, userID, key).Scan(&m, &p, &h, &r.Status, &r.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if m != method || p != path || h != reqHash {
		return nil, ErrKeyReused
	}
	return &r, nil
}

// Remember stores the answer to an idempotent request for 24 hours.
func (s *Service) Remember(ctx context.Context, userID, key, method, path, reqHash string, status int, body []byte) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO idempotency_keys (user_id, key, method, path, request_hash, status, response)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, key) DO UPDATE SET method = $3, path = $4, request_hash = $5, status = $6, response = $7, created_at = now()
		WHERE idempotency_keys.created_at <= now() - interval '24 hours'`, userID, key, method, path, reqHash, status, body)
	return err
}

// PurgeIdempotency deletes stored answers older than a day.
func (s *Service) PurgeIdempotency(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE created_at < now() - interval '25 hours'`)
	return err
}
