package bgg_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/bgg"
)

const threadJSON = `{"type":"threads","id":"2125946","source":{"type":"things","id":"266192"},
"crumbs":[{"href":"/boardgame/266192/wingspan/forums/0","name":"Forums"},{"href":"/boardgame/266192/wingspan/forums/66","name":"Rules"}],
"subject":"Refreshing the birdfeeder during an action","numposts":3}`

const articlesJSON = `{"articles":[
 {"id":"30895619","postdate":"2019-01-06T15:20:14+00:00","canonical_link":"https://boardgamegeek.com/thread/2125946/article/30895619#30895619",
  "body":"I have a question about the [b]dice[/b] in birdfeeder.\n\n\n\nWhat happens?"},
 {"id":"30895700","postdate":"2019-01-07T09:00:00+00:00","canonical_link":"https://boardgamegeek.com/thread/2125946/article/30895700#30895700",
  "body":"[q=\"someone\"]What happens?[/q]\nYou [url=https://example.org]reroll[/url] only if all dice match."}
]}`

func TestThread_ReadsSourceForumAndArticles(t *testing.T) {
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/threads/2125946":
			io.WriteString(w, threadJSON)
		case r.URL.Path == "/articles" && r.URL.Query().Get("threadid") == "2125946":
			io.WriteString(w, articlesJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := bgg.NewHTTPClient()
	c.ForumsBaseURL = ts.URL
	th, err := c.Thread(context.Background(), "2125946")
	if err != nil {
		t.Fatalf("thread: %v (requests: %v)", err, paths)
	}
	if th.ID != "2125946" || th.ObjectType != "things" || th.ObjectID != "266192" || th.Forum != "Rules" {
		t.Fatalf("unexpected thread meta %+v", th)
	}
	if th.Subject != "Refreshing the birdfeeder during an action" {
		t.Fatalf("unexpected subject %q", th.Subject)
	}
	if len(th.Articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(th.Articles))
	}
	a := th.Articles[0]
	if a.Link != "https://boardgamegeek.com/thread/2125946/article/30895619#30895619" {
		t.Fatalf("unexpected link %q", a.Link)
	}
	if a.PostDate.Format("02/01/2006") != "06/01/2019" {
		t.Fatalf("unexpected date %v", a.PostDate)
	}
	// Il markup BGG sparisce, il testo resta; le righe vuote multiple si
	// comprimono.
	if a.Body != "I have a question about the dice in birdfeeder.\n\nWhat happens?" {
		t.Fatalf("unexpected cleaned body %q", a.Body)
	}
	if b := th.Articles[1].Body; !strings.Contains(b, "You reroll only if all dice match.") || strings.Contains(b, "[") {
		t.Fatalf("unexpected cleaned body %q", b)
	}
}

func TestThread_NotFoundIsAnError(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()

	c := bgg.NewHTTPClient()
	c.ForumsBaseURL = ts.URL
	if _, err := c.Thread(context.Background(), "1"); err == nil {
		t.Fatal("expected an error for a missing thread")
	}
}
