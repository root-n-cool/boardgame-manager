package faq_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/faq"
	"boardgames-manager/internal/websearch"
)

type fakeSearcher struct {
	results []websearch.Result
	err     error
	query   string
	domains []string
	max     int
}

func (f *fakeSearcher) Search(ctx context.Context, q string, d []string, max int) ([]websearch.Result, error) {
	f.query, f.domains, f.max = q, d, max
	return f.results, f.err
}

type fakeFetcher struct {
	threads map[string]bgg.Thread
	fetched []string
}

func (f *fakeFetcher) Thread(ctx context.Context, id string) (bgg.Thread, error) {
	f.fetched = append(f.fetched, id)
	th, ok := f.threads[id]
	if !ok {
		return bgg.Thread{}, errors.New("not found")
	}
	return th, nil
}

func day(d string) time.Time {
	t, _ := time.Parse("2006-01-02", d)
	return t
}

func rulesThread(id, subject string, bodies ...string) bgg.Thread {
	th := bgg.Thread{ID: id, Subject: subject, ObjectType: "things", ObjectID: "266192", Forum: "Rules"}
	for i, b := range bodies {
		th.Articles = append(th.Articles, bgg.Article{
			ID: id + "-" + string(rune('a'+i)), Body: b,
			Link:     "https://boardgamegeek.com/thread/" + id + "/article/" + string(rune('a'+i)),
			PostDate: day("2019-01-0" + string(rune('6'+i))),
		})
	}
	return th
}

func threadURL(id string) websearch.Result {
	return websearch.Result{URL: "https://boardgamegeek.com/thread/" + id + "/some-slug"}
}

func TestSearch_BuildsTheQueryAndReturnsCitableHits(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("100")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"100": rulesThread("100", "Refreshing the birdfeeder", "What happens?", "You reroll."),
	}}

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", "refresh birdfeeder")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if s.query != `"Wingspan" refresh birdfeeder rules` {
		t.Fatalf("unexpected query %q", s.query)
	}
	if len(s.domains) != 1 || s.domains[0] != "boardgamegeek.com/thread" || s.max != faq.SearchResults {
		t.Fatalf("unexpected search params %v %d", s.domains, s.max)
	}
	if len(hits) != 2 {
		t.Fatalf("expected one hit per comment, got %+v", hits)
	}
	h := hits[1]
	if h.Reference != "BGG: Refreshing the birdfeeder" || h.ReferenceDetail != "commento del 07/01/2019" {
		t.Fatalf("unexpected citation %+v", h)
	}
	if h.Text != "You reroll." || h.CommentURL != "https://boardgamegeek.com/thread/100/article/b" ||
		h.ThreadURL != "https://boardgamegeek.com/thread/100" {
		t.Fatalf("unexpected hit %+v", h)
	}
}

func TestSearch_KeepsOnlyRulesThreadsOfTheGame(t *testing.T) {
	expansion := rulesThread("200", "Oceania birdfeeder", "x")
	expansion.ObjectID = "300580"
	variants := rulesThread("300", "House rule for birdfeeder", "x")
	variants.Forum = "Variants"
	s := &fakeSearcher{results: []websearch.Result{
		threadURL("200"), threadURL("300"),
		{URL: "https://boardgamegeek.com/blog/1/blogpost/2"}, // non è un thread
		threadURL("400"),
	}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"200": expansion, "300": variants,
		"400": rulesThread("400", "Birdfeeder with one die", "ok"),
	}}

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", "q")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Reference != "BGG: Birdfeeder with one die" {
		t.Fatalf("expected only the Rules thread of the game, got %+v", hits)
	}
}

func TestSearch_StopsAtMaxThreadsAndDeduplicates(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{
		threadURL("1"), {URL: "https://boardgamegeek.com/thread/1/article/55#55"}, threadURL("2"), threadURL("3"),
	}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"1": rulesThread("1", "A", "a"), "2": rulesThread("2", "B", "b"), "3": rulesThread("3", "C", "c"),
	}}

	hits, _ := faq.Search(context.Background(), s, f, "Wingspan", "266192", "q")
	if strings.Join(f.fetched, ",") != "1,2" {
		t.Fatalf("expected threads 1 and 2 fetched once each, got %v", f.fetched)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %+v", hits)
	}
}

func TestSearch_RespectsTheCharBudgetPerThread(t *testing.T) {
	long := strings.Repeat("x", faq.ThreadCharBudget-10)
	s := &fakeSearcher{results: []websearch.Result{threadURL("1")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"1": rulesThread("1", "A", long, "questo non ci sta più", "neanche questo"),
	}}

	hits, _ := faq.Search(context.Background(), s, f, "Wingspan", "266192", "q")
	if len(hits) != 1 {
		t.Fatalf("expected only the first comment within budget, got %d hits", len(hits))
	}
}

func TestSearch_TruncatesAFirstPostLongerThanTheBudget(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("1")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"1": rulesThread("1", "A", strings.Repeat("y", faq.ThreadCharBudget*2)),
	}}

	hits, _ := faq.Search(context.Background(), s, f, "Wingspan", "266192", "q")
	if len(hits) != 1 || len(hits[0].Text) > faq.ThreadCharBudget {
		t.Fatalf("the first post must survive, truncated to the budget: %d hits", len(hits))
	}
}

func TestSearch_SkipsAThreadThatFailsToLoad(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("404"), threadURL("1")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{"1": rulesThread("1", "A", "a")}}

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", "q")
	if err != nil {
		t.Fatalf("a single failing thread must not fail the search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected the loadable thread, got %+v", hits)
	}
}

func TestSearch_DisambiguatesThreadsWithTheSameSubject(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("1"), threadURL("2")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"1": rulesThread("1", "Question", "a"), "2": rulesThread("2", "Question", "b"),
	}}

	hits, _ := faq.Search(context.Background(), s, f, "Wingspan", "266192", "q")
	if len(hits) != 2 || hits[0].Reference == hits[1].Reference {
		t.Fatalf("expected distinct references, got %+v", hits)
	}
	if hits[1].Reference != "BGG: Question #2" {
		t.Fatalf("unexpected disambiguated reference %q", hits[1].Reference)
	}
}

func TestSearch_WebSearchErrorIsReturned(t *testing.T) {
	s := &fakeSearcher{err: errors.New("boom")}
	if _, err := faq.Search(context.Background(), s, &fakeFetcher{}, "Wingspan", "266192", "q"); err == nil {
		t.Fatal("expected the web search error")
	}
}
