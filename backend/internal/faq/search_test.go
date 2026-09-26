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

// fakeFetcher tiene threads con gli Articles già dentro (come rulesThread li
// costruisce): ThreadInfo restituisce il thread SENZA Articles (come farebbe
// bgg.HTTPClient.ThreadInfo, che non chiama /articles), ThreadArticles
// restituisce solo gli Articles. infoFetched e articlesFetched sono liste
// separate: un test (S2) verifica che un thread scartato dal filtro
// gioco/forum non generi mai una chiamata a ThreadArticles.
type fakeFetcher struct {
	threads         map[string]bgg.Thread
	infoFetched     []string
	articlesFetched []string
}

func (f *fakeFetcher) ThreadInfo(ctx context.Context, id string) (bgg.Thread, error) {
	f.infoFetched = append(f.infoFetched, id)
	th, ok := f.threads[id]
	if !ok {
		return bgg.Thread{}, errors.New("not found")
	}
	th.Articles = nil
	return th, nil
}

func (f *fakeFetcher) ThreadArticles(ctx context.Context, id string) ([]bgg.Article, error) {
	f.articlesFetched = append(f.articlesFetched, id)
	th, ok := f.threads[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return th.Articles, nil
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

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "refresh birdfeeder")
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

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
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

	hits, _ := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
	if strings.Join(f.infoFetched, ",") != "1,2" {
		t.Fatalf("expected threads 1 and 2 fetched once each, got %v", f.infoFetched)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %+v", hits)
	}
}

// TestSearch_SkipsArticlesCallForAThreadFilteredOutByForum: bgg.Thread fa
// una seconda chiamata (ThreadArticles) che costa quanto la prima. Un
// thread scartato dal filtro gioco/forum di faq.Search non deve mai
// arrivare a quella seconda chiamata.
func TestSearch_SkipsArticlesCallForAThreadFilteredOutByForum(t *testing.T) {
	variants := rulesThread("300", "House rule for birdfeeder", "x")
	variants.Forum = "Variants"
	s := &fakeSearcher{results: []websearch.Result{threadURL("300")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{"300": variants}}

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits from a Variants thread, got %+v", hits)
	}
	if len(f.articlesFetched) != 0 {
		t.Fatalf("a thread filtered out by forum must not fetch its articles: %v", f.articlesFetched)
	}
}

func TestSearch_RespectsTheCharBudgetPerThread(t *testing.T) {
	long := strings.Repeat("x", faq.ThreadCharBudget-10)
	s := &fakeSearcher{results: []websearch.Result{threadURL("1")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"1": rulesThread("1", "A", long, "questo non ci sta più", "neanche questo"),
	}}

	hits, _ := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
	if len(hits) != 1 {
		t.Fatalf("expected only the first comment within budget, got %d hits", len(hits))
	}
}

func TestSearch_TruncatesAFirstPostLongerThanTheBudget(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("1")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{
		"1": rulesThread("1", "A", strings.Repeat("y", faq.ThreadCharBudget*2)),
	}}

	hits, _ := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
	if len(hits) != 1 || len(hits[0].Text) > faq.ThreadCharBudget {
		t.Fatalf("the first post must survive, truncated to the budget: %d hits", len(hits))
	}
}

func TestSearch_SkipsAThreadThatFailsToLoad(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("404"), threadURL("1")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{"1": rulesThread("1", "A", "a")}}

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
	if err != nil {
		t.Fatalf("a single failing thread must not fail the search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected the loadable thread, got %+v", hits)
	}
}

func TestSearch_WebSearchErrorIsReturned(t *testing.T) {
	s := &fakeSearcher{err: errors.New("boom")}
	if _, err := faq.Search(context.Background(), s, &fakeFetcher{}, "Wingspan", "266192", faq.ForumRules, "q"); err == nil {
		t.Fatal("expected the web search error")
	}
}

// TestSearch_AllThreadsFailingIsAnError: quando almeno un thread è stato
// trovato ma NESSUNA lettura riesce, il modello non deve vedere
// "nessun_risultato_per" (che suggerisce "nessuna FAQ pertinente", falso):
// deve vedere il messaggio di indisponibilità (task ai/ask.go traduce
// questo errore in quel messaggio).
func TestSearch_AllThreadsFailingIsAnError(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("1"), threadURL("2")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{}} // ogni Thread() fallisce con "not found"

	_, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
	if err == nil {
		t.Fatal("quando tutte le letture falliscono Search deve restituire un errore")
	}
}

// blockingFetcher simula geekdo che non risponde mai entro il contesto:
// serve a verificare che Search abbia un tetto complessivo, non solo per
// singola chiamata.
type blockingFetcher struct{}

func (blockingFetcher) ThreadInfo(ctx context.Context, id string) (bgg.Thread, error) {
	<-ctx.Done()
	return bgg.Thread{}, ctx.Err()
}

func (blockingFetcher) ThreadArticles(ctx context.Context, id string) ([]bgg.Article, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestSearch_RespectsTheOverallDeadline(t *testing.T) {
	s := &fakeSearcher{results: []websearch.Result{threadURL("1")}}
	// Il contesto del chiamante scade a 50ms, molto sotto il tetto interno
	// di 20s: Search deve tornare intorno a quei 50ms, non aspettare 20s.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := faq.Search(ctx, s, blockingFetcher{}, "Wingspan", "266192", faq.ForumRules, "q")
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("Search non rispetta il tetto complessivo: tornata dopo %s", elapsed)
	}
	if err == nil {
		t.Fatal("nessuna lettura riuscita entro la scadenza: atteso un errore")
	}
}

func TestSearch_RejectsUnsafeCommentLinks(t *testing.T) {
	for _, link := range []string{"javascript:alert(1)", "https://evil.example/x"} {
		th := rulesThread("1", "A", "testo")
		th.Articles[0].Link = link
		s := &fakeSearcher{results: []websearch.Result{threadURL("1")}}
		f := &fakeFetcher{threads: map[string]bgg.Thread{"1": th}}

		hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(hits) != 1 || hits[0].CommentURL != "" {
			t.Fatalf("link %q: atteso CommentURL vuoto, ottenuto %+v", link, hits)
		}
	}
}

func TestSearch_StrategyForumKeepsOnlyStrategyThreads(t *testing.T) {
	rules := rulesThread("100", "Birdfeeder rule", "x")
	strategy := rulesThread("200", "Engine or points early?", "Go for food engine first.")
	strategy.Forum = "Strategy"
	s := &fakeSearcher{results: []websearch.Result{threadURL("100"), threadURL("200")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{"100": rules, "200": strategy}}

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumStrategy, "engine vs points")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Reference != "BGG: Engine or points early?" {
		t.Fatalf("expected only the Strategy thread, got %+v", hits)
	}
	if !strings.HasSuffix(s.query, " strategy") {
		t.Fatalf("the Strategy query must end with \"strategy\", got %q", s.query)
	}
	if s.max != faq.StrategySearchResults {
		t.Fatalf("expected %d results for Strategy, got %d", faq.StrategySearchResults, s.max)
	}
}

func TestSearch_RulesForumDropsStrategyThreads(t *testing.T) {
	strategy := rulesThread("200", "Engine or points early?", "x")
	strategy.Forum = "Strategy"
	s := &fakeSearcher{results: []websearch.Result{threadURL("200")}}
	f := &fakeFetcher{threads: map[string]bgg.Thread{"200": strategy}}

	hits, err := faq.Search(context.Background(), s, f, "Wingspan", "266192", faq.ForumRules, "q")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("a Strategy thread is not a rules FAQ, got %+v", hits)
	}
	if !strings.HasSuffix(s.query, " rules") {
		t.Fatalf("the Rules query must end with \"rules\", got %q", s.query)
	}
}
