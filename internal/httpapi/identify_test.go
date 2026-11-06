package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jconner5017-rgb/marketplace/internal/identify"
)

func TestIdentify(t *testing.T) {
	e := setupEnv(t)
	calls := 0
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{"content": []map[string]any{{"type": "text",
			"text": `{"category":"tcg","game_or_sport":"","title":"Charizard Holo Base Set","set_name":"Base Set","year":1999,"card_number":"4/102","graded":false,"condition":"LP","attributes":[]}`}}})
	}))
	defer claude.Close()

	c, _ := newSeller(t, e, "idseller", true)
	other, _ := newSeller(t, e, "idother", true)
	upload := func(c *http.Client) string {
		img := image.NewRGBA(image.Rect(0, 0, 900, 1200))
		var b bytes.Buffer
		png.Encode(&b, img)
		req, _ := http.NewRequest("POST", e.ts.URL+"/v1/photos", &b)
		req.Header.Set("Content-Type", "image/png")
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		json.NewDecoder(res.Body).Decode(&m)
		return m["photo"].(map[string]any)["url"].(string)
	}
	mine, theirs := upload(c), upload(other)

	// Off until a key is set.
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/identify", map[string]any{"photo_urls": []string{mine}}); code != 503 || errCode(body) != "identify_off" {
		t.Fatalf("off: %d %v", code, body)
	}
	e.srv.identify = &identify.Client{APIKey: "k", BaseURL: claude.URL}

	code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/identify", map[string]any{"photo_urls": []string{mine}})
	if code != 200 {
		t.Fatalf("identify: %d %v", code, body)
	}
	res := body["result"].(map[string]any)
	if res["title"] != "Charizard Holo Base Set" || res["condition"] != "LP" || res["category"] != "tcg" || body["left_today"].(float64) != 49 {
		t.Fatalf("result: %v", body)
	}

	// Someone else's photo is refused before anything is sent.
	before := calls
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/identify", map[string]any{"photo_urls": []string{theirs}}); code != 422 || errCode(body) != "bad_photo" || calls != before {
		t.Fatalf("other's photo: %d %v", code, body)
	}
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/identify", map[string]any{"photo_urls": []string{"http://169.254.169.254/latest"}}); code != 422 || errCode(body) != "bad_photo" {
		t.Fatalf("outside url: %d %v", code, body)
	}

	// The daily limit.
	if _, err := e.db.Exec(`INSERT INTO identify_usage (user_id) SELECT (SELECT user_id FROM identify_usage LIMIT 1) FROM generate_series(1, 49)`); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, c, "POST", e.ts.URL+"/v1/seller/identify", map[string]any{"photo_urls": []string{mine}}); code != 429 || errCode(body) != "identify_limit" {
		t.Fatalf("limit: %d %v", code, body)
	}
}
