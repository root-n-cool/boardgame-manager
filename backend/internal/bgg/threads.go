package bgg

import (
	"context"
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

// ThreadInfo legge i metadati di un thread (senza commenti): a quale gioco
// appartiene (Source) e in che forum sta (l'ultimo crumb). faq.Search la usa
// per scartare i thread di un'espansione, di un gioco omonimo o fuori dal
// forum Rules PRIMA di spendere la seconda chiamata che legge i commenti
// (ThreadArticles).
func (c *HTTPClient) ThreadInfo(ctx context.Context, threadID string) (Thread, error) {
	var meta threadJSON
	if err := c.getJSON(ctx, c.forumsBase(), "/threads/"+url.PathEscape(threadID), nil, &meta); err != nil {
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
	return out, nil
}

// ThreadArticles legge la prima pagina di commenti (25) di un thread: il
// primo post più le prime risposte è dove sta quasi sempre la risposta a
// una domanda sulle regole.
func (c *HTTPClient) ThreadArticles(ctx context.Context, threadID string) ([]Article, error) {
	q := url.Values{}
	q.Set("threadid", threadID)
	q.Set("pageid", "1")
	var arts articlesJSON
	if err := c.getJSON(ctx, c.forumsBase(), "/articles", q, &arts); err != nil {
		return nil, err
	}

	var out []Article
	for _, a := range arts.Articles {
		body := cleanForumMarkup(a.Body)
		if body == "" {
			continue
		}
		posted, _ := time.Parse(time.RFC3339, a.PostDate)
		out = append(out, Article{ID: a.ID, Body: body, Link: a.CanonicalLink, PostDate: posted})
	}
	return out, nil
}

// forumsBase risolve ForumsBaseURL al suo default, come faceva prima
// getForumsJSON: ThreadInfo e ThreadArticles condividono la stessa base.
func (c *HTTPClient) forumsBase() string {
	base := strings.TrimRight(c.ForumsBaseURL, "/")
	if base == "" {
		base = DefaultForumsBaseURL
	}
	return base
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
