package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPhotoUpload(t *testing.T) {
	e := setupEnv(t)
	c, _ := newSeller(t, e, "photoseller", true)
	img := image.NewRGBA(image.Rect(0, 0, 900, 1200))
	var b bytes.Buffer
	png.Encode(&b, img)

	post := func(c *http.Client, ct string, body []byte) (int, map[string]any) {
		req, _ := http.NewRequest("POST", e.ts.URL+"/v1/photos", bytes.NewReader(body))
		req.Header.Set("Content-Type", ct)
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		json.NewDecoder(res.Body).Decode(&m)
		return res.StatusCode, m
	}

	code, body := post(c, "image/png", b.Bytes())
	if code != 201 {
		t.Fatalf("upload: %d %v", code, body)
	}
	p := body["photo"].(map[string]any)
	u := p["url"].(string)
	if !strings.HasPrefix(u, "https://api.market.test/photos/") {
		t.Fatalf("url %s", u)
	}
	// Served from the database as a cacheable JPEG.
	res, err := http.Get(e.ts.URL + strings.TrimPrefix(u, "https://api.market.test"))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" ||
		!strings.Contains(res.Header.Get("Cache-Control"), "immutable") || len(data) == 0 {
		t.Fatalf("serve: %d %v", res.StatusCode, res.Header)
	}

	// Not an image; wrong content type; not a seller.
	if code, body := post(c, "image/png", []byte("hello")); code != 422 || errCode(body) != "bad_photo" {
		t.Fatalf("junk: %d %v", code, body)
	}
	if code, _ := post(c, "text/plain", b.Bytes()); code != 415 {
		t.Fatalf("text/plain: %d", code)
	}
	buyer := client(t)
	do(t, buyer, "POST", e.ts.URL+"/v1/auth/signup", map[string]string{"email": "photobuyer@example.com", "username": "photobuyer", "password": "longenough1"})
	if code, _ := post(buyer, "image/png", b.Bytes()); code != 403 {
		t.Fatalf("buyer upload: %d", code)
	}
	if code, _ := post(client(t), "image/png", b.Bytes()); code != 401 {
		t.Fatalf("anonymous upload: %d", code)
	}
}
