package websearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/websearch"
)

func TestTavily_SendsTheQueryAndParsesResults(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"query":"q","results":[
			{"title":"Refreshing the birdfeeder","url":"https://boardgamegeek.com/thread/2125946/refreshing","content":"What happens…","score":0.9}
		]}`)
	}))
	defer ts.Close()

	c := websearch.NewTavily("tvly-key")
	c.BaseURL = ts.URL
	got, err := c.Search(context.Background(), `"Wingspan" birdfeeder rules`, []string{"boardgamegeek.com/thread"}, 8)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotAuth != "Bearer tvly-key" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	if gotBody["query"] != `"Wingspan" birdfeeder rules` || gotBody["search_depth"] != "basic" || gotBody["max_results"] != float64(8) {
		t.Fatalf("unexpected body %v", gotBody)
	}
	domains, _ := gotBody["include_domains"].([]any)
	if len(domains) != 1 || domains[0] != "boardgamegeek.com/thread" {
		t.Fatalf("unexpected include_domains %v", gotBody["include_domains"])
	}
	if len(got) != 1 || got[0].URL != "https://boardgamegeek.com/thread/2125946/refreshing" || got[0].Title != "Refreshing the birdfeeder" {
		t.Fatalf("unexpected results %+v", got)
	}
}

func TestTavily_HTTPErrorIsAnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"invalid key"}`, http.StatusUnauthorized)
	}))
	defer ts.Close()

	c := websearch.NewTavily("tvly-bad")
	c.BaseURL = ts.URL
	_, err := c.Search(context.Background(), "q", nil, 5)
	if err == nil {
		t.Fatal("expected an error on 401")
	}
	// Il messaggio deve portare un pezzo del body: nei log è la differenza
	// fra una chiave revocata ("invalid key") e crediti esauriti, che
	// altrimenti sarebbero entrambi indistinguibili "status 401"/"status
	// 402".
	if !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("expected the error to carry a body excerpt, got %v", err)
	}
}

func TestTavily_WithoutAKeyIsNotConfigured(t *testing.T) {
	c := websearch.NewTavily("  ")
	if _, err := c.Search(context.Background(), "q", nil, 5); !errors.Is(err, websearch.ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
