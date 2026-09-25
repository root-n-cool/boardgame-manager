package bgg

import (
	"context"
	"encoding/json"
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

// DefaultFilesBaseURL è l'endpoint JSON che alimenta la sezione "Files"
// del sito. Non fa parte della XML API2 e non vuole token: risponde
// senza header Authorization. Serve solo la metadata (nome, lingua,
// voti, link alla filepage) — i byte dei file restano dietro il login
// e la bot protection di boardgamegeek.com.
const DefaultFilesBaseURL = "https://api.geekdo.com/api/files"

// DefaultForumsBaseURL è lo stesso JSON non ufficiale di DefaultFilesBaseURL.
// Risponde senza token e, a differenza di xmlapi2/thread, dice a quale
// gioco e a quale forum appartiene un thread: è quel che serve per
// scartare i risultati di ricerca che parlano di un'espansione o stanno
// nel forum Variants.
const DefaultForumsBaseURL = "https://api.geekdo.com/api"

// filesPageSize è il massimo che BGG serve per pagina: showcount più
// alti vengono comunque troncati a 50.
const filesPageSize = "50"

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

// FileEntry è un file della sezione "Files" di un gioco. PageURL punta
// alla filepage su BGG, non al file: il download richiede il login.
type FileEntry struct {
	Title    string
	Filename string
	Language string
	// LanguageID è l'id BGG della lingua del file: lo stesso con cui si
	// filtra la lista, e da cui si risale al codice lingua dell'app.
	LanguageID string
	Positive   int
	SizeBytes  int64
	PageURL    string
}

type Client interface {
	Search(ctx context.Context, token, query string) ([]SearchResult, error)
	GetThing(ctx context.Context, token, id string) (ThingDetail, error)
	Details(ctx context.Context, token string, ids []string) (map[string]ThingDetail, error)
	Files(ctx context.Context, bggID, languageID string) ([]FileEntry, error)
	ThreadInfo(ctx context.Context, threadID string) (Thread, error)
	ThreadArticles(ctx context.Context, threadID string) ([]Article, error)
}

type HTTPClient struct {
	BaseURL       string
	FilesBaseURL  string
	ForumsBaseURL string
	HTTPClient    *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		BaseURL:       DefaultBaseURL,
		FilesBaseURL:  DefaultFilesBaseURL,
		ForumsBaseURL: DefaultForumsBaseURL,
		HTTPClient:    &http.Client{Timeout: 15 * time.Second},
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

// getJSON esegue una GET su un endpoint JSON non ufficiale di geekdo (Files
// o Forums, non l'XML API2 autenticata di doRequest) e decodifica il body
// in into. base è già risolto dal chiamante (fallback al default incluso):
// Files() e il codice dei thread hanno basi ed endpoint diversi, qui c'è
// solo la plumbing comune a entrambi. Il body è limitato a 4 MiB: sono
// risposte JSON di metadata, non dovrebbero mai avvicinarsi a quella
// taglia.
func (c *HTTPClient) getJSON(ctx context.Context, base, path string, q url.Values, into any) error {
	full := base + path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bgg json request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read bgg json response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bgg json returned status %d for %s", resp.StatusCode, path)
	}

	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("parse bgg json response: %w", err)
	}
	return nil
}

// filesResponseJSON matcha la risposta di DefaultFilesBaseURL. I campi
// numerici arrivano come stringhe, e language/numpositive possono essere
// null (file "(neutral)", o mai votati): da qui i puntatori.
type filesResponseJSON struct {
	Files []struct {
		Filename    string  `json:"filename"`
		Title       string  `json:"title"`
		Size        *string `json:"size"`
		NumPositive *string `json:"numpositive"`
		Language    *string `json:"language"`
		LanguageID  *string `json:"languageid"`
		Href        string  `json:"href"`
	} `json:"files"`
}

// Files elenca i file che BGG ha per un gioco, opzionalmente filtrati
// per lingua (languageID è l'id BGG della lingua, "" per tutte).
func (c *HTTPClient) Files(ctx context.Context, bggID, languageID string) ([]FileEntry, error) {
	query := url.Values{}
	query.Set("ajax", "1")
	query.Set("nosession", "1")
	query.Set("objecttype", "thing")
	query.Set("objectid", bggID)
	query.Set("pageid", "1")
	query.Set("showcount", filesPageSize)
	// "hot" è l'unico ordinamento che BGG onora davvero: numpositive e
	// postdate restituiscono un ordine arbitrario.
	query.Set("sort", "hot")
	if languageID != "" {
		query.Set("languageid", languageID)
	}

	base := c.FilesBaseURL
	if base == "" {
		base = DefaultFilesBaseURL
	}
	var parsed filesResponseJSON
	if err := c.getJSON(ctx, base, "", query, &parsed); err != nil {
		return nil, err
	}

	out := make([]FileEntry, 0, len(parsed.Files))
	for _, f := range parsed.Files {
		entry := FileEntry{
			Title:    strings.TrimSpace(f.Title),
			Filename: strings.TrimSpace(f.Filename),
		}
		if entry.Title == "" {
			entry.Title = entry.Filename
		}
		if f.Language != nil {
			entry.Language = *f.Language
		}
		if f.LanguageID != nil {
			entry.LanguageID = *f.LanguageID
		}
		if f.NumPositive != nil {
			entry.Positive, _ = strconv.Atoi(*f.NumPositive)
		}
		if f.Size != nil {
			entry.SizeBytes, _ = strconv.ParseInt(*f.Size, 10, 64)
		}
		if f.Href != "" {
			entry.PageURL = "https://boardgamegeek.com" + f.Href
		}
		out = append(out, entry)
	}
	return out, nil
}
