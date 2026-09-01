package bgg

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const DefaultBaseURL = "https://boardgamegeek.com/xmlapi2"

type SearchResult struct {
	ID   string
	Name string
	Year int
}

type ThingDetail struct {
	ID          string
	Name        string
	Description string
	Year        int
	MinPlayers  int
	MaxPlayers  int
	PlayingTime int
	ImageURL    string
}

type Client interface {
	Search(ctx context.Context, token, query string) ([]SearchResult, error)
	GetThing(ctx context.Context, token, id string) (ThingDetail, error)
}

type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{},
	}
}

type searchResponseXML struct {
	Items []struct {
		ID   string `xml:"id,attr"`
		Name struct {
			Value string `xml:"value,attr"`
		} `xml:"name"`
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
		out = append(out, SearchResult{ID: item.ID, Name: item.Name.Value, Year: year})
	}
	return out, nil
}

type thingResponseXML struct {
	Items []struct {
		ID    string `xml:"id,attr"`
		Image string `xml:"image"`
		Names []struct {
			Type  string `xml:"type,attr"`
			Value string `xml:"value,attr"`
		} `xml:"name"`
		Description   string `xml:"description"`
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
	} `xml:"item"`
}

func (c *HTTPClient) GetThing(ctx context.Context, token, id string) (ThingDetail, error) {
	q := url.Values{}
	q.Set("id", id)

	body, err := c.doRequest(ctx, token, "/thing", q)
	if err != nil {
		return ThingDetail{}, err
	}

	var parsed thingResponseXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return ThingDetail{}, fmt.Errorf("parse bgg thing response: %w", err)
	}
	if len(parsed.Items) == 0 {
		return ThingDetail{}, fmt.Errorf("bgg: no item found for id %s", id)
	}

	item := parsed.Items[0]
	primaryName := item.ID
	for _, n := range item.Names {
		if n.Type == "primary" {
			primaryName = n.Value
			break
		}
	}

	year, _ := strconv.Atoi(item.YearPublished.Value)
	minPlayers, _ := strconv.Atoi(item.MinPlayers.Value)
	maxPlayers, _ := strconv.Atoi(item.MaxPlayers.Value)
	playingTime, _ := strconv.Atoi(item.PlayingTime.Value)

	return ThingDetail{
		ID: item.ID, Name: primaryName, Description: item.Description,
		Year: year, MinPlayers: minPlayers, MaxPlayers: maxPlayers,
		PlayingTime: playingTime, ImageURL: item.Image,
	}, nil
}

// doRequest issues an authenticated GET request to the BGG XML API2.
//
// NOTE: BGG recently began requiring an application token for XML API
// access but does not clearly document the expected header format at the
// time this was written. "Authorization: Bearer <token>" is a best guess —
// if real requests fail with 401/403 once a real token is available, this
// is the single place to adjust (e.g. a different header name, or a raw
// token without the "Bearer " prefix).
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
