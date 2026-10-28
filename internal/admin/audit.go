package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

var (
	b64std = base64.StdEncoding
	b64url = base64.URLEncoding
)

// Entry is one audit log row.
type Entry struct {
	ID         int64          `json:"id"`
	At         time.Time      `json:"at"`
	ActorID    string         `json:"actor_id"`
	ActorEmail string         `json:"actor_email"`
	ActorRole  string         `json:"actor_role"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Reason     string         `json:"reason"`
	Details    map[string]any `json:"details"`
	IP         string         `json:"ip"`
}

// Audit writes and reads the append-only, hash-chained audit log.
type Audit struct {
	DB *sql.DB
}

const auditLock = 7240001 // advisory lock: one writer at a time keeps the chain in order

func canonical(d map[string]any) ([]byte, error) {
	if d == nil {
		d = map[string]any{}
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	// Round-trip so the bytes match what's re-read from the database.
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func chainHash(prev []byte, e Entry, details []byte) []byte {
	h := sha256.New()
	h.Write(prev)
	h.Write([]byte{'\n'})
	h.Write([]byte(strings.Join([]string{
		e.At.UTC().Format(time.RFC3339Nano), e.ActorID, e.ActorEmail, e.ActorRole, e.Action,
		e.TargetType, e.TargetID, e.Reason, string(details), e.IP,
	}, "\x1f")))
	return h.Sum(nil)
}

// Log appends an entry. Pass the caller's transaction so the entry is
// written only if the action itself commits; nil uses its own.
func (a *Audit) Log(ctx context.Context, tx *sql.Tx, e Entry) error {
	own := tx == nil
	if own {
		var err error
		if tx, err = a.DB.BeginTx(ctx, nil); err != nil {
			return err
		}
		defer tx.Rollback()
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auditLock); err != nil {
		return err
	}
	var prev []byte
	if err := tx.QueryRowContext(ctx, `SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prev); err != nil && err != sql.ErrNoRows {
		return err
	}
	e.At = time.Now().UTC().Truncate(time.Microsecond)
	details, err := canonical(e.Details)
	if err != nil {
		return err
	}
	hash := chainHash(prev, e, details)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_log (at, actor_id, actor_email, actor_role, action, target_type, target_id, reason, details, ip, prev_hash, hash)
		VALUES ($1, NULLIF($2, '')::uuid, $3, NULLIF($4, ''), $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, NULLIF($10, ''), $11, $12)`,
		e.At, e.ActorID, e.ActorEmail, e.ActorRole, e.Action, e.TargetType, e.TargetID, e.Reason, details, e.IP, prev, hash); err != nil {
		return err
	}
	if own {
		return tx.Commit()
	}
	return nil
}

const auditCols = `id, at, coalesce(actor_id::text, ''), actor_email, coalesce(actor_role, ''), action,
	coalesce(target_type, ''), coalesce(target_id, ''), reason, details, coalesce(ip, ''), coalesce(prev_hash, ''::bytea), hash`

func scanEntry(row interface{ Scan(...any) error }) (Entry, []byte, []byte, []byte, error) {
	var e Entry
	var details, prev, hash []byte
	err := row.Scan(&e.ID, &e.At, &e.ActorID, &e.ActorEmail, &e.ActorRole, &e.Action, &e.TargetType, &e.TargetID,
		&e.Reason, &details, &e.IP, &prev, &hash)
	if err == nil {
		json.Unmarshal(details, &e.Details)
	}
	return e, details, prev, hash, err
}

// Filter narrows an audit search.
type Filter struct {
	ActorID    string
	Action     string // exact, or a prefix ending in "." (e.g. "user.")
	TargetType string
	TargetID   string
	BeforeID   int64
	Limit      int
}

// List returns entries newest first.
func (a *Audit) List(ctx context.Context, f Filter) ([]Entry, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 100
	}
	where := []string{"true"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if f.ActorID != "" {
		add("actor_id::text = ?", f.ActorID)
	}
	if strings.HasSuffix(f.Action, ".") {
		add("action LIKE ? || '%'", f.Action)
	} else if f.Action != "" {
		add("action = ?", f.Action)
	}
	if f.TargetType != "" {
		add("target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		add("target_id = ?", f.TargetID)
	}
	if f.BeforeID > 0 {
		add("id < ?", f.BeforeID)
	}
	args = append(args, f.Limit)
	rows, err := a.DB.QueryContext(ctx, `SELECT `+auditCols+` FROM audit_log WHERE `+strings.Join(where, " AND ")+
		` ORDER BY id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		e, _, _, _, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// VerifyResult of the whole chain.
type ChainCheck struct {
	Checked  int   `json:"checked"`
	OK       bool  `json:"ok"`
	BrokenAt int64 `json:"broken_at,omitempty"` // first entry whose hash doesn't match
}

// Verify recomputes every hash from the first entry.
func (a *Audit) Verify(ctx context.Context) (ChainCheck, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT `+auditCols+` FROM audit_log ORDER BY id`)
	if err != nil {
		return ChainCheck{}, err
	}
	defer rows.Close()
	res := ChainCheck{OK: true}
	var prev []byte
	for rows.Next() {
		e, raw, storedPrev, hash, err := scanEntry(rows)
		if err != nil {
			return res, err
		}
		res.Checked++
		var v any
		json.Unmarshal(raw, &v)
		details, _ := json.Marshal(v)
		if !bytes.Equal(storedPrev, prev) || !bytes.Equal(chainHash(prev, e, details), hash) {
			res.OK, res.BrokenAt = false, e.ID
			return res, nil
		}
		prev = hash
	}
	return res, rows.Err()
}
