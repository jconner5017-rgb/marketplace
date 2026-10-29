package photos

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/jconner5017-rgb/marketplace/internal/db"
)

func pngOf(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestProcess(t *testing.T) {
	out, w, h, err := process(pngOf(3200, 1000))
	if err != nil {
		t.Fatal(err)
	}
	if w != 1600 || h != 500 {
		t.Fatalf("got %dx%d", w, h)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatal("output isn't a JPEG")
	}
	if _, _, _, err := process(pngOf(200, 900)); err != ErrTooSmall {
		t.Fatalf("small: %v", err)
	}
	if _, _, _, err := process([]byte("<svg></svg>")); err != ErrNotImage {
		t.Fatalf("svg: %v", err)
	}
	// Transparent pixels flatten to white, not black.
	img := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	var b bytes.Buffer
	png.Encode(&b, img)
	out, _, _, err = process(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	d, _ := jpeg.Decode(bytes.NewReader(out))
	if r, _, _, _ := d.At(10, 10).RGBA(); r < 0xf000 {
		t.Fatalf("transparent became %x", r)
	}
}

// The GET example from AWS's SigV4 documentation for S3.
func TestSigV4KnownVector(t *testing.T) {
	s := &S3{Region: "us-east-1", AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	s.sign(req, nil, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); !strings.HasSuffix(got, want) ||
		!strings.Contains(got, "SignedHeaders=host;range;x-amz-content-sha256;x-amz-date") {
		t.Fatalf("got %s", got)
	}
}

func TestUploadAndSweep(t *testing.T) {
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
	if err := d.QueryRow(`INSERT INTO users (email, username, password_hash) VALUES ('photo-test@example.test', 'photo_test', '!x')
		ON CONFLICT (lower(email)) DO UPDATE SET updated_at = now() RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM users WHERE id = $1`, uid)
	s := &Service{DB: d, BaseURL: "https://api.example.test"}
	p, err := s.Upload(ctx, uid, pngOf(800, 1100))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.URL, "https://api.example.test/photos/") || p.Width != 800 {
		t.Fatalf("photo %+v", p)
	}
	data, ct, err := s.Get(ctx, p.ID)
	if err != nil || ct != "image/jpeg" || len(data) != p.Bytes {
		t.Fatalf("get: %v %s %d", err, ct, len(data))
	}
	used, _ := s.Upload(ctx, uid, pngOf(600, 600))
	// Age both, reference one from a listing row.
	d.Exec(`UPDATE photos SET created_at = now() - interval '2 days' WHERE owner_id = $1`, uid)
	if _, err := d.Exec(`INSERT INTO listings (seller_id, category, game_or_sport, set_name, title, condition, photo_urls, price_cents, quantity)
		VALUES ($1, 'tcg', 'Pokemon', 'Base', 'Photo test', 'NM', ARRAY[$2], 100, 1)`, uid, used.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get(ctx, p.ID); err != ErrNotFound {
		t.Fatalf("unused photo kept: %v", err)
	}
	if _, _, err := s.Get(ctx, used.ID); err != nil {
		t.Fatalf("used photo swept: %v", err)
	}
}
