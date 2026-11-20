package photos

import (
	"bytes"
	"context"
	"database/sql"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/db"
)

func TestThumbnail(t *testing.T) {
	full, _, _, err := process(pngOf(1200, 1680))
	if err != nil {
		t.Fatal(err)
	}
	small, err := thumbnail(full)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(small))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 560 {
		t.Fatalf("thumb %v", b)
	}
	if len(small)*3 > len(full) {
		t.Fatalf("thumb %d bytes vs full %d", len(small), len(full))
	}
	if thumbKey("p/2026/11/a.jpg") != "t/2026/11/a.jpg" {
		t.Fatal(thumbKey("p/2026/11/a.jpg"))
	}
}

// fakeBucket is an in-memory S3 bucket.
type fakeBucket struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func (f *fakeBucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.TrimPrefix(r.URL.Path, "/b/")
	switch r.Method {
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		f.objs[key] = b
	case http.MethodGet:
		b, ok := f.objs[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	case http.MethodDelete:
		delete(f.objs, key)
	}
}

func TestThumbsAndMove(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip()
	}
	ctx := context.Background()
	d, _ := sql.Open("postgres", url)
	defer d.Close()
	if err := db.Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	var uid string
	if err := d.QueryRow(`INSERT INTO users (email, username, password_hash) VALUES ('thumb-test@example.test', 'thumb_test', '!x')
		ON CONFLICT (lower(email)) DO UPDATE SET updated_at = now() RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM users WHERE id = $1`, uid)
	defer d.Exec(`DELETE FROM photo_url_aliases`)

	bucket := &fakeBucket{objs: map[string][]byte{}}
	srv := httptest.NewServer(bucket)
	defer srv.Close()
	s3 := &S3{Endpoint: srv.URL, Bucket: "b", Region: "auto", AccessKey: "k", SecretKey: "s", PublicURL: "https://pub-old.r2.dev"}
	s := &Service{DB: d, S3: s3}

	// New uploads get a small copy right away; banners don't need one.
	p, err := s.Upload(ctx, uid, pngOf(800, 1120))
	if err != nil {
		t.Fatal(err)
	}
	var thumb sql.NullString
	d.QueryRow(`SELECT thumb_url FROM photos WHERE id = $1`, p.ID).Scan(&thumb)
	if !strings.HasPrefix(thumb.String, "https://pub-old.r2.dev/t/") || bucket.objs[strings.TrimPrefix(thumb.String, "https://pub-old.r2.dev/")] == nil {
		t.Fatalf("thumb %v", thumb)
	}

	// Older photos (made before thumbnails) are filled in by the backfill;
	// one whose file is gone is marked so it isn't retried.
	old, _ := s.Upload(ctx, uid, pngOf(600, 840))
	gone, _ := s.Upload(ctx, uid, pngOf(600, 840))
	d.Exec(`UPDATE photos SET thumb_url = NULL WHERE id IN ($1, $2)`, old.ID, gone.ID)
	var goneKey string
	d.QueryRow(`SELECT object_key FROM photos WHERE id = $1`, gone.ID).Scan(&goneKey)
	delete(bucket.objs, goneKey)
	if n, err := s.BackfillThumbs(ctx, 10); err != nil || n != 2 {
		t.Fatalf("backfill: %d %v", n, err)
	}
	if n, _ := s.BackfillThumbs(ctx, 10); n != 0 {
		t.Fatalf("second backfill: %d", n)
	}
	d.QueryRow(`SELECT thumb_url FROM photos WHERE id = $1`, old.ID).Scan(&thumb)
	if !strings.Contains(thumb.String, "/t/") {
		t.Fatalf("old thumb %v", thumb)
	}
	d.QueryRow(`SELECT thumb_url FROM photos WHERE id = $1`, gone.ID).Scan(&thumb)
	if !thumb.Valid || thumb.String != "" {
		t.Fatalf("missing file thumb %v", thumb)
	}

	// A listing and a store use the photos on the old address.
	var lid string
	if err := d.QueryRow(`INSERT INTO listings (seller_id, category, game_or_sport, set_name, title, condition, photo_urls, price_cents, quantity)
		VALUES ($1, 'tcg', 'Pokemon', 'Base', 'Move test', 'NM', ARRAY[$2, $3, 'https://elsewhere.example/x.jpg'], 100, 1) RETURNING id`,
		uid, p.URL, old.URL).Scan(&lid); err != nil {
		t.Fatal(err)
	}

	// Nothing to move while the address is unchanged.
	if n, err := s.MovePublicURL(ctx); err != nil || n != 0 {
		t.Fatalf("no-op move: %d %v", n, err)
	}
	// The address changes: everything moves over, keeping photo order.
	s3.PublicURL = "https://photos.example.test"
	n, err := s.MovePublicURL(ctx)
	if err != nil || n != 3 {
		t.Fatalf("move: %d %v", n, err)
	}
	var urls []string
	var row *sql.Row = d.QueryRow(`SELECT array_to_string(photo_urls, ' ') FROM listings WHERE id = $1`, lid)
	var joined string
	row.Scan(&joined)
	urls = strings.Fields(joined)
	if len(urls) != 3 || urls[0] != strings.Replace(p.URL, "https://pub-old.r2.dev", "https://photos.example.test", 1) ||
		!strings.HasPrefix(urls[1], "https://photos.example.test/p/") || urls[2] != "https://elsewhere.example/x.jpg" {
		t.Fatalf("listing photos %v", urls)
	}
	d.QueryRow(`SELECT thumb_url FROM photos WHERE id = $1`, p.ID).Scan(&thumb)
	if !strings.HasPrefix(thumb.String, "https://photos.example.test/t/") {
		t.Fatalf("moved thumb %v", thumb)
	}
	// An app re-sending an old address gets the new one.
	if got := s.Canonical(old.URL); got != strings.Replace(old.URL, "https://pub-old.r2.dev", "https://photos.example.test", 1) {
		t.Fatalf("canonical %s", got)
	}
	if got := s.Canonical("https://elsewhere.example/x.jpg"); got != "https://elsewhere.example/x.jpg" {
		t.Fatalf("canonical other %s", got)
	}
	if n, _ := s.MovePublicURL(ctx); n != 0 {
		t.Fatalf("second move: %d", n)
	}
	// The sweep still sees the photos as used (no deletion of in-use files).
	d.Exec(`UPDATE photos SET created_at = now() - interval '2 days' WHERE owner_id = $1`, uid)
	if _, err := s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	var left int
	d.QueryRow(`SELECT count(*) FROM photos WHERE id IN ($1, $2)`, p.ID, old.ID).Scan(&left)
	if left != 2 {
		t.Fatalf("in-use photos swept: %d left", left)
	}
	d.Exec(`DELETE FROM listings WHERE id = $1`, lid)
}
