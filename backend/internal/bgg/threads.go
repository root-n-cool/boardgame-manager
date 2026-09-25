package bgg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Article struct {
	ID       string
	Body     string
	Link     string
	PostDate time.Time
}

type Thread struct {
	ID         string
	Subject    string
	ObjectType string // "things" per un gioco
	ObjectID   string // il bggId del gioco
	Forum      string // nome dell'ultimo crumb, es. "Rules"
	Articles   []Article
}

type threadJSON struct {
	Subject string `json:"subject"`
	Source  struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"source"`
	Crumbs []struct {
		Name string `json:"name"`
	} `json:"crumbs"`
}

type articlesJSON struct {
	Articles []struct {
		ID            string `json:"id"`
		PostDate      string `json:"postdate"`
		Body          string `json:"body"`
		CanonicalLink string `json:"canonical_link"`
	} `json:"articles"`
}

// Thread legge un thread e la sua prima pagina di commenti (25): il primo
// post più le prime risposte è dove sta quasi sempre la risposta a una
// domanda sulle regole.
func (c *HTTPClient) Thread(ctx context.Context, threadID string) (Thread, error) {
	var meta threadJSON
	if err := c.getForumsJSON(ctx, "/threads/"+url.PathEscape(threadID), nil, &meta); err != nil {
		return Thread{}, err
	}
	q := url.Values{}
	q.Set("threadid", threadID)
	q.Set("pageid", "1")
	var arts articlesJSON
	if err := c.getForumsJSON(ctx, "/articles", q, &arts); err != nil {
		return Thread{}, err
	}

	out := Thread{
		ID:         threadID,
		Subject:    strings.TrimSpace(meta.Subject),
		ObjectType: meta.Source.Type,
		ObjectID:   meta.Source.ID,
	}
	if n := len(meta.Crumbs); n > 0 {
		out.Forum = meta.Crumbs[n-1].Name
	}
	for _, a := range arts.Articles {
		body := cleanForumMarkup(a.Body)
		if body == "" {
			continue
		}
		posted, _ := time.Parse(time.RFC3339, a.PostDate)
		out.Articles = append(out.Articles, Article{ID: a.ID, Body: body, Link: a.CanonicalLink, PostDate: posted})
	}
	return out, nil
}

func (c *HTTPClient) getForumsJSON(ctx context.Context, path string, q url.Values, into any) error {
	base := strings.TrimRight(c.ForumsBaseURL, "/")
	if base == "" {
		base = DefaultForumsBaseURL
	}
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
		return fmt.Errorf("bgg forums request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read bgg forums response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bgg forums returned status %d for %s", resp.StatusCode, path)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("parse bgg forums response: %w", err)
	}
	return nil
}

// forumTag riconosce i tag BBCode di BGG: [b], [/b], [q="utente"], [url=…],
// [imageid=123 medium], [/url]… Si tolgono i tag e si tiene il testo che
// racchiudono: una citazione ripete un pezzo di domanda, ma toglierla
// intera rischierebbe di perdere il contesto della risposta.
var forumTag = regexp.MustCompile(`\[/?[a-zA-Z]+[^\]]*\]`)

var blankLines = regexp.MustCompile(`\n{3,}`)

func cleanForumMarkup(s string) string {
	s = forumTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
