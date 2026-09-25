// Package websearch parla con un'API di ricerca web. Serve all'agente
// regole per trovare i thread del forum Rules su BoardGameGeek: l'API di
// BGG non ha una ricerca nei forum, e trovare il thread giusto è proprio
// la parte che manca.
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured dice che l'admin non ha messo una chiave. Non è un
// guasto: è l'app senza ricerca nelle FAQ.
var ErrNotConfigured = errors.New("web search not configured")

const DefaultTavilyBaseURL = "https://api.tavily.com"

// requestTimeout è stretto: la ricerca sta dentro una domanda fatta dal
// tavolo, che ha un tetto complessivo di 60 secondi.
const requestTimeout = 10 * time.Second

type Result struct {
	Title   string
	URL     string
	Content string
}

type Searcher interface {
	Search(ctx context.Context, query string, domains []string, max int) ([]Result, error)
}

// Tavily è il client di https://tavily.com. Scelto perché il piano
// gratuito (1.000 ricerche al mese) non chiede una carta di credito.
type Tavily struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewTavily(apiKey string) *Tavily {
	return &Tavily{
		BaseURL:    DefaultTavilyBaseURL,
		APIKey:     strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{Timeout: requestTimeout},
	}
}

type tavilyRequest struct {
	Query          string   `json:"query"`
	SearchDepth    string   `json:"search_depth"`
	MaxResults     int      `json:"max_results"`
	IncludeDomains []string `json:"include_domains,omitempty"`
}

type tavilyResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

// Search fa una ricerca "basic" (1 credito). include_domains accetta anche
// un path: "boardgamegeek.com/thread" tiene fuori blog e geeklist.
func (t *Tavily) Search(ctx context.Context, query string, domains []string, max int) ([]Result, error) {
	if strings.TrimSpace(t.APIKey) == "" {
		return nil, ErrNotConfigured
	}
	payload, err := json.Marshal(tavilyRequest{
		Query: query, SearchDepth: "basic", MaxResults: max, IncludeDomains: domains,
	})
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(t.BaseURL, "/")
	if base == "" {
		base = DefaultTavilyBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/search", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := t.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read tavily response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Un pezzo del body distingue nei log una chiave revocata da
		// crediti esauriti: entrambi sarebbero altrimenti lo stesso "status
		// 401"/"status 402" indistinguibile.
		excerpt := string(body)
		if len(excerpt) > 200 {
			excerpt = excerpt[:200]
		}
		return nil, fmt.Errorf("tavily returned status %d: %s", resp.StatusCode, excerpt)
	}
	var parsed tavilyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse tavily response: %w", err)
	}
	out := make([]Result, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		out = append(out, Result{Title: r.Title, URL: r.URL, Content: r.Content})
	}
	return out, nil
}
