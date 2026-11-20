package photos

import (
	"context"
	"strings"
	"sync"
)

// MovePublicURL moves stored photo addresses to the storage's current public
// address (PHOTO_PUBLIC_URL) after it changes, e.g. from the bucket's r2.dev
// address to photos.tcgcards.us. Both addresses serve the same files, so
// nothing breaks while it runs. It updates photos, listings, store logos
// and banners, and site banners, and remembers each old prefix so addresses
// sent later by apps are mapped too (Canonical). Safe to run at every start;
// it does nothing once everything is on the current address.
func (s *Service) MovePublicURL(ctx context.Context) (moved int, err error) {
	if s.S3 == nil || s.S3.PublicURL == "" {
		return 0, nil
	}
	cur := strings.TrimRight(s.S3.PublicURL, "/")
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT left(url, length(url) - length(object_key) - 1) FROM photos
		WHERE store = 's3' AND object_key IS NOT NULL AND url LIKE '%/' || object_key
		  AND url <> $1 || '/' || object_key`, cur)
	if err != nil {
		return 0, err
	}
	var olds []string
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			rows.Close()
			return 0, err
		}
		if o != "" && o != cur {
			olds = append(olds, o)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, old := range olds {
		n, err := s.moveOne(ctx, old, cur)
		if err != nil {
			return moved, err
		}
		moved += n
	}
	// A small copy made on the old address while the move ran (by the
	// instance being replaced) is moved too.
	if _, err := s.DB.ExecContext(ctx, `
		UPDATE photos p SET thumb_url = a.new_prefix || substr(p.thumb_url, length(a.old_prefix) + 1)
		FROM photo_url_aliases a
		WHERE p.thumb_url LIKE a.old_prefix || '/%'`); err != nil {
		return moved, err
	}
	return moved, s.loadAliases(ctx)
}

func (s *Service) moveOne(ctx context.Context, old, cur string) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// swap(x) is x with the old prefix replaced, for a value known to start with it.
	const swap = `$2 || substr(%s, length($1) + 1)`
	like := `$1 || '/%'`
	stmts := []string{
		`UPDATE photos SET url = ` + sprintf(swap, "url") + `,
		   thumb_url = CASE WHEN thumb_url LIKE ` + like + ` THEN ` + sprintf(swap, "thumb_url") + ` ELSE thumb_url END
		 WHERE url LIKE ` + like,
		`UPDATE listings SET photo_urls = (
		   SELECT array_agg(CASE WHEN u LIKE ` + like + ` THEN ` + sprintf(swap, "u") + ` ELSE u END ORDER BY i)
		   FROM unnest(photo_urls) WITH ORDINALITY AS x(u, i))
		 WHERE EXISTS (SELECT 1 FROM unnest(photo_urls) u WHERE u LIKE ` + like + `)`,
		`UPDATE stores SET logo_url = ` + sprintf(swap, "logo_url") + ` WHERE logo_url LIKE ` + like,
		`UPDATE stores SET banner_url = ` + sprintf(swap, "banner_url") + ` WHERE banner_url LIKE ` + like,
		`UPDATE site_banners SET image_url = ` + sprintf(swap, "image_url") + ` WHERE image_url LIKE ` + like,
	}
	total := 0
	for i, q := range stmts {
		r, err := tx.ExecContext(ctx, q, old, cur)
		if err != nil {
			return 0, err
		}
		if i == 0 {
			n, _ := r.RowsAffected()
			total = int(n)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO photo_url_aliases (old_prefix, new_prefix) VALUES ($1, $2)
		ON CONFLICT (old_prefix) DO UPDATE SET new_prefix = EXCLUDED.new_prefix, moved_at = now()`, old, cur); err != nil {
		return 0, err
	}
	// An address that was current earlier and pointed elsewhere now leads here too.
	if _, err := tx.ExecContext(ctx, `UPDATE photo_url_aliases SET new_prefix = $2 WHERE new_prefix = $1`, old, cur); err != nil {
		return 0, err
	}
	return total, tx.Commit()
}

func sprintf(f, col string) string { return strings.Replace(f, "%s", col, 1) }

type aliases struct {
	mu   sync.RWMutex
	list [][2]string // old prefix, new prefix
}

func (s *Service) loadAliases(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT old_prefix, new_prefix FROM photo_url_aliases`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var list [][2]string
	for rows.Next() {
		var o, n string
		if err := rows.Scan(&o, &n); err != nil {
			return err
		}
		list = append(list, [2]string{o, n})
	}
	s.aliases.mu.Lock()
	s.aliases.list = list
	s.aliases.mu.Unlock()
	return rows.Err()
}

// Canonical maps a photo address on an old storage address to the current
// one (unchanged otherwise), so an app re-sending the address it got at
// upload doesn't bring back the old one or leave the photo looking unused.
func (s *Service) Canonical(u string) string {
	if s == nil {
		return u
	}
	s.aliases.mu.RLock()
	defer s.aliases.mu.RUnlock()
	for _, a := range s.aliases.list {
		if strings.HasPrefix(u, a[0]+"/") {
			return a[1] + u[len(a[0]):]
		}
	}
	return u
}
