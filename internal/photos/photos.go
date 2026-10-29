// Package photos stores seller photo uploads: each one is checked,
// resized, stripped of metadata and kept in the database or in
// S3-compatible storage, then referenced from listings by URL.
package photos

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// DailyLimit caps uploads per seller per 24 hours.
const DailyLimit = 400

// Service handles uploads. With S3 nil, photo bytes live in Postgres and
// are served by the API at BaseURL + /photos/{id}.jpg.
type Service struct {
	DB      *sql.DB
	S3      *S3
	BaseURL string // the API's public URL (IMAGE_BASE_URL)
}

type Photo struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
}

// Store names where new uploads go.
func (s *Service) Store() string {
	if s.S3 != nil {
		return "s3"
	}
	return "db"
}

// Upload processes and stores one photo for ownerID.
func (s *Service) Upload(ctx context.Context, ownerID string, data []byte) (Photo, error) {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM photos WHERE owner_id = $1 AND created_at > now() - interval '1 day'`,
		ownerID).Scan(&n); err != nil {
		return Photo{}, err
	}
	if n >= DailyLimit {
		return Photo{}, ErrTooMany
	}
	out, w, h, err := process(data)
	if err != nil {
		return Photo{}, err
	}
	var idb [16]byte
	rand.Read(idb[:])
	// A random UUID-shaped id, so URLs can't be guessed or enumerated.
	idb[6] = idb[6]&0x0f | 0x40
	idb[8] = idb[8]&0x3f | 0x80
	hx := hex.EncodeToString(idb[:])
	id := hx[0:8] + "-" + hx[8:12] + "-" + hx[12:16] + "-" + hx[16:20] + "-" + hx[20:]

	p := Photo{ID: id, Width: w, Height: h, Bytes: len(out)}
	if s.S3 != nil {
		key := "p/" + time.Now().UTC().Format("2006/01") + "/" + id + ".jpg"
		u, err := s.S3.Put(ctx, key, "image/jpeg", out)
		if err != nil {
			return Photo{}, err
		}
		p.URL = u
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO photos (id, owner_id, store, object_key, url, content_type, width, height, size_bytes)
			VALUES ($1, $2, 's3', $3, $4, 'image/jpeg', $5, $6, $7)`, id, ownerID, key, u, w, h, len(out))
		return p, err
	}
	p.URL = s.BaseURL + "/photos/" + id + ".jpg"
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO photos (id, owner_id, store, url, content_type, width, height, size_bytes, data)
		VALUES ($1, $2, 'db', $3, 'image/jpeg', $4, $5, $6, $7)`, id, ownerID, p.URL, w, h, len(out), out)
	return p, err
}

// Get returns a database-stored photo's bytes.
func (s *Service) Get(ctx context.Context, id string) ([]byte, string, error) {
	var data []byte
	var ct string
	err := s.DB.QueryRowContext(ctx, `SELECT data, content_type FROM photos WHERE id = $1 AND store = 'db'`, id).Scan(&data, &ct)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return data, ct, err
}

// Sweep deletes uploads older than a day that no listing uses, including
// photos dropped from a listing later. main runs it hourly.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id, p.store, coalesce(p.object_key, '') FROM photos p
		WHERE p.created_at < now() - interval '1 day'
		  AND NOT EXISTS (SELECT 1 FROM listings l WHERE l.photo_urls @> ARRAY[p.url])
		LIMIT 500`)
	if err != nil {
		return 0, err
	}
	type gone struct{ id, store, key string }
	var list []gone
	for rows.Next() {
		var g gone
		if err := rows.Scan(&g.id, &g.store, &g.key); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, g)
	}
	rows.Close()
	n := 0
	for _, g := range list {
		if g.store == "s3" {
			if s.S3 == nil {
				continue // storage not configured right now; try again later
			}
			if err := s.S3.Delete(ctx, g.key); err != nil {
				return n, err
			}
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM photos WHERE id = $1`, g.id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
