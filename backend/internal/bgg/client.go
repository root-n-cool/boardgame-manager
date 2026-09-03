package bgg

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://boardgamegeek.com/xmlapi2"

type SearchResult struct {
	ID   string
	Name string
	Year int
}

type ThingDetail struct {
	ID           string
	Name         string
	Description  string
	Year         int
	MinPlayers   int
	MaxPlayers   int
	PlayingTime  int
	ImageURL     string
	ThumbnailURL string
	// Weight is BGG's average complexity, 1 (light) to 5 (heavy). It is 0
	// when nobody has rated the game yet: unknown, not "very light".
	Weight float64
}

type Client interface {
	Search(ctx context.Context, token, query string) ([]SearchResult, error)
	GetThing(ctx context.Context, token, id string) (ThingDetail, error)
	Details(ctx context.Context, token string, ids []string) (map[string]ThingDetail, error)
}

type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// nameXML matches BGG's repeated <name type="..." value="..."/> elements.
// Both /search and /thing responses can list multiple names (primary plus
// alternates); primaryName picks the one marked "primary" instead of
// relying on document order (BGG does not guarantee primary comes first).
type nameXML struct {
	Type  string `xml:"type,attr"`
	Value string `xml:"value,attr"`
}

// primaryName picks the name marked "primary", falling back to the first
// name of any type: plenty of search hits (fan expansions, localised
// editions) carry only an <name type="alternate">, and showing the numeric
// id instead of that name makes the row unreadable.
func primaryName(names []nameXML, fallback string) string {
	for _, n := range names {
		if n.Type == "primary" {
			return n.Value
		}
	}
	if len(names) > 0 && names[0].Value != "" {
		return names[0].Value
	}
	return fallback
}

type searchResponseXML struct {
	Items []struct {
		ID            string    `xml:"id,attr"`
		Names         []nameXML `xml:"name"`
		YearPublished struct {
			Value string `xml:"value,attr"`
		} `xml:"yearpublished"`
	} `xml:"item"`
}

func (c *HTTPClient) Search(ctx context.Context, token, query string) ([]SearchResult, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("type", "boardgame")

	body, err := c.doRequest(ctx, token, "/search", q)
	if err != nil {
		return nil, err
	}

	var parsed searchResponseXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse bgg search response: %w", err)
	}

	out := make([]SearchResult, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		year, _ := strconv.Atoi(item.YearPublished.Value)
		out = append(out, SearchResult{ID: item.ID, Name: primaryName(item.Names, item.ID), Year: year})
	}
	return out, nil
}

type thingResponseXML struct {
	Items []struct {
		ID            string    `xml:"id,attr"`
		Image         string    `xml:"image"`
		Thumbnail     string    `xml:"thumbnail"`
		Names         []nameXML `xml:"name"`
		Description   string    `xml:"description"`
		YearPublished struct {
			Value string `xml:"value,attr"`
		} `xml:"yearpublished"`
		MinPlayers struct {
			Value string `xml:"value,attr"`
		} `xml:"minplayers"`
		MaxPlayers struct {
			Value string `xml:"value,attr"`
		} `xml:"maxplayers"`
		PlayingTime struct {
			Value string `xml:"value,attr"`
		} `xml:"playingtime"`
		Statistics struct {
			Ratings struct {
				AverageWeight struct {
					Value string `xml:"value,attr"`
				} `xml:"averageweight"`
			} `xml:"ratings"`
		} `xml:"statistics"`
	} `xml:"item"`
}

// GetThing returns the full detail of a single game.
func (c *HTTPClient) GetThing(ctx context.Context, token, id string) (ThingDetail, error) {
	details, err := c.Details(ctx, token, []string{id})
	if err != nil {
		return ThingDetail{}, err
	}
	detail, ok := details[id]
	if !ok {
		return ThingDetail{}, fmt.Errorf("bgg: no item found for id %s", id)
	}
	return detail, nil
}

// Details fetches several games in one /thing call, keyed by BGG id.
//
// stats=1 is what makes <averageweight> show up; without it BGG answers with
// the same document minus the <statistics> block, so the weight is silently
// zero. Batching matters too: the search picker needs a thumbnail for every
// row it shows, and BGG rate-limits per request, not per id.
func (c *HTTPClient) Details(ctx context.Context, token string, ids []string) (map[string]ThingDetail, error) {
	out := make(map[string]ThingDetail, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	q := url.Values{}
	q.Set("id", strings.Join(ids, ","))
	q.Set("stats", "1")

	body, err := c.doRequest(ctx, token, "/thing", q)
	if err != nil {
		return nil, err
	}

	var parsed thingResponseXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse bgg thing response: %w", err)
	}

	for _, item := range parsed.Items {
		year, _ := strconv.Atoi(item.YearPublished.Value)
		minPlayers, _ := strconv.Atoi(item.MinPlayers.Value)
		maxPlayers, _ := strconv.Atoi(item.MaxPlayers.Value)
		playingTime, _ := strconv.Atoi(item.PlayingTime.Value)
		weight, _ := strconv.ParseFloat(item.Statistics.Ratings.AverageWeight.Value, 64)

		out[item.ID] = ThingDetail{
			ID: item.ID, Name: primaryName(item.Names, item.ID), Description: item.Description,
			Year: year, MinPlayers: minPlayers, MaxPlayers: maxPlayers,
			PlayingTime: playingTime, ImageURL: item.Image, ThumbnailURL: item.Thumbnail,
			Weight: weight,
		}
	}
	return out, nil
}

// doRequest issues an authenticated GET request to the BGG XML API2.
//
// BGG requires an application token: without the Authorization header the
// API answers 200 with the body "Unauthorized". The "Bearer <token>" form
// is confirmed working against the live API.
func (c *HTTPClient) doRequest(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	fullURL := c.BaseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bgg request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bgg response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bgg returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
