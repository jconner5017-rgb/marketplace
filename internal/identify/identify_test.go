package identify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var choices = Choices{
	Kinds: map[string][]Option{
		"sports":   {{"baseball", "Baseball"}, {"football", "Football"}},
		"tcg":      {{"pokemon", "Pokémon TCG"}},
		"nonsport": {{"marvel", "Superheroes"}},
	},
	Graders:    []string{"PSA", "BGS", "SGC", "CGC", "TAG", "OTHER"},
	Conditions: []string{"NM", "LP", "MP", "HP", "DMG"},
	Features:   []Option{{"rookie", "Rookie"}, {"auto", "Autograph"}, {"numbered", "Serial numbered"}},
}

func fake(t *testing.T, reply string, check func(*http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		json.NewEncoder(w).Encode(map[string]any{"content": []map[string]any{{"type": "text", "text": reply}}})
	}))
}

func TestIdentifyCleansToCatalog(t *testing.T) {
	ts := fake(t, "```json\n"+`{"category":"sports","game_or_sport":"Baseball","title":"2016 Bowman Chrome Bryan Reynolds Auto /499","set_name":"Bowman Chrome","year":2016,"card_number":"#CDA-BRR","graded":true,"grader":"psa","grade":9.3,"cert_number":"12345678","condition":"NM","attributes":["Autograph","numbered","laser"],"notes":""}`+"\n```",
		func(r *http.Request) {
			if r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
				t.Errorf("missing headers")
			}
			var body struct {
				Messages []struct {
					Content []map[string]any `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if n := len(body.Messages[0].Content); n != 3 {
				t.Errorf("want 2 images + text, got %d parts", n)
			}
		})
	defer ts.Close()
	c := &Client{APIKey: "k", BaseURL: ts.URL}
	r, err := c.Identify(context.Background(), [][]byte{[]byte("jpg1"), []byte("jpg2")}, choices)
	if err != nil {
		t.Fatal(err)
	}
	if r.Category != "sports" || r.GameOrSport != "baseball" || r.CardNumber != "CDA-BRR" || *r.Year != 2016 {
		t.Fatalf("card fields: %+v", r)
	}
	if !r.Graded || r.Grader != "PSA" || *r.Grade != 9.5 || r.Condition != "" {
		t.Fatalf("grading: %+v", r)
	}
	if strings.Join(r.Attributes, ",") != "auto,numbered" {
		t.Fatalf("attributes: %v", r.Attributes)
	}
}

func TestIdentifyUnknownGameAndUnreadable(t *testing.T) {
	ts := fake(t, `{"category":"tcg","game_or_sport":"Digimon","title":"","set_name":"","graded":false,"condition":"mint","attributes":[]}`, nil)
	defer ts.Close()
	c := &Client{APIKey: "k", BaseURL: ts.URL}
	if _, err := c.Identify(context.Background(), [][]byte{[]byte("x")}, choices); err != ErrUnreadable {
		t.Fatalf("want ErrUnreadable, got %v", err)
	}
	r := Clean(Result{GameOrSport: "Pokémon TCG", Title: "Pikachu", Condition: "lp"}, choices)
	if r.Category != "tcg" || r.GameOrSport != "pokemon" || r.Condition != "LP" {
		t.Fatalf("category from game: %+v", r)
	}
}

func TestIdentifyOff(t *testing.T) {
	var c *Client
	if _, err := c.Identify(context.Background(), nil, choices); err != ErrOff {
		t.Fatal(err)
	}
}
