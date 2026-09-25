// Package faq cerca la risposta a una domanda sulle regole nel forum Rules
// di BoardGameGeek, al volo: una ricerca web trova i thread, il JSON di
// geekdo li legge. Niente si salva.
package faq

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/websearch"
)

type ThreadFetcher interface {
	ThreadInfo(ctx context.Context, threadID string) (bgg.Thread, error)
	ThreadArticles(ctx context.Context, threadID string) ([]bgg.Article, error)
}

// Hit è un commento del forum pronto per il modello. CommentURL, ThreadURL
// e ThreadID non escono al modello: servono all'handler per i link e per
// disambiguare la reference fra più chiamate dello stesso tool nella
// stessa richiesta.
type Hit struct {
	Reference       string
	ReferenceDetail string
	Text            string
	CommentURL      string
	ThreadURL       string
	ThreadID        string
}

const (
	// SearchResults è largo: circa metà dei risultati cade sui filtri
	// (espansioni, Variants, General). Misurato il 2026-09-25.
	SearchResults = 8
	// MaxThreads e ThreadCharBudget tengono il risultato del tool nella
	// stessa taglia di una ricerca nei manuali.
	MaxThreads       = 2
	ThreadCharBudget = 6000
	// callTimeout vale per ogni chiamata esterna: la domanda intera ha un
	// tetto di 60 secondi e non può passarli ad aspettare BGG.
	callTimeout = 10 * time.Second
	// overallTimeout è il tetto dell'INTERA Search (una Tavily più fino a
	// MaxThreads*qualcheTentativo Thread, tutte sequenziali): senza, un
	// geekdo lento consuma da solo tutto il budget della domanda
	// (askTimeout, 60s) e la risposta finale del modello fallisce. Derivato
	// dal contesto del chiamante: se quello scade prima, vince comunque
	// prima (context.WithTimeout prende il minimo).
	overallTimeout = 20 * time.Second
)

var threadIDPattern = regexp.MustCompile(`boardgamegeek\.com/thread/(\d+)`)

// validCommentURL accetta come link di un commento solo un URL BGG senza
// spazi né ")" (che romperebbe la sintassi markdown del link): un
// Article.Link malformato o ostile (es. "javascript:alert(1)") non deve
// arrivare nella risposta al modello, dove poi finisce dentro un link
// markdown cliccabile.
func validCommentURL(link string) bool {
	if !strings.HasPrefix(link, "https://boardgamegeek.com/") {
		return false
	}
	return !strings.ContainsAny(link, " \t\n)")
}

// Search è legata al gioco dal chiamante: bggID non è un parametro del
// tool, così il modello non può leggere il forum di un altro gioco.
func Search(ctx context.Context, s websearch.Searcher, f ThreadFetcher, gameName, bggID, query string) ([]Hit, error) {
	ctx, cancel := context.WithTimeout(ctx, overallTimeout)
	defer cancel()

	sctx, cancel2 := context.WithTimeout(ctx, callTimeout)
	results, err := s.Search(sctx, fmt.Sprintf("%q %s rules", gameName, query), []string{"boardgamegeek.com/thread"}, SearchResults)
	cancel2()
	if err != nil {
		return nil, err
	}

	var ids []string
	seen := map[string]bool{}
	for _, r := range results {
		m := threadIDPattern.FindStringSubmatch(r.URL)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		ids = append(ids, m[1])
	}

	var hits []Hit
	accepted := 0
	// fetchedAny distingue, alla fine, "nessuna lettura è mai riuscita" (un
	// errore da segnalare) da "qualche lettura è riuscita ma è stata
	// scartata dai filtri" (un array vuoto legittimo).
	fetchedAny := false
	var lastErr error
	for _, id := range ids {
		if accepted >= MaxThreads {
			break
		}
		if ctx.Err() != nil {
			// Il tetto complessivo è scaduto: si smette di leggere e si
			// restituisce quel che si è raccolto finora.
			break
		}
		tctx, cancel := context.WithTimeout(ctx, callTimeout)
		th, err := f.ThreadInfo(tctx, id)
		cancel()
		if err != nil {
			log.Printf("faq: thread %s: %v", id, err)
			lastErr = err
			continue
		}
		// Un thread di un'espansione o di un gioco omonimo, o fuori dal
		// forum Rules (Variants sono regole della casa), non è una FAQ di
		// questo gioco: si scarta senza spendere la seconda chiamata che
		// legge i commenti.
		if th.ObjectType != "things" || th.ObjectID != bggID || th.Forum != "Rules" {
			fetchedAny = true
			continue
		}

		actx, cancel := context.WithTimeout(ctx, callTimeout)
		articles, err := f.ThreadArticles(actx, id)
		cancel()
		if err != nil {
			log.Printf("faq: thread %s articles: %v", id, err)
			lastErr = err
			continue
		}
		fetchedAny = true
		if len(articles) == 0 {
			continue
		}
		accepted++

		ref := "BGG: " + th.Subject
		threadURL := "https://boardgamegeek.com/thread/" + th.ID

		budget := ThreadCharBudget
		for i, a := range articles {
			text := a.Body
			if len(text) > budget {
				if i > 0 {
					break
				}
				// Il primo post è la domanda: se da solo sfora, si tronca
				// invece di perdere il thread.
				text = strings.ToValidUTF8(text[:budget], "")
			}
			budget -= len(text)
			commentURL := a.Link
			if !validCommentURL(commentURL) {
				commentURL = ""
			}
			hits = append(hits, Hit{
				Reference:       ref,
				ReferenceDetail: "commento del " + a.PostDate.Format("02/01/2006"),
				Text:            text,
				CommentURL:      commentURL,
				ThreadURL:       threadURL,
				ThreadID:        th.ID,
			})
		}
	}

	if len(hits) > 0 {
		return hits, nil
	}
	if ctx.Err() != nil {
		// Scaduto il tetto complessivo prima di raccogliere nulla: è
		// l'errore da riportare, non "nessun thread leggibile" (che
		// implicherebbe un errore per thread, non un timeout globale).
		return nil, ctx.Err()
	}
	if lastErr != nil && !fetchedAny {
		// Almeno un thread era stato trovato, ma NESSUNA lettura è
		// riuscita: è un guasto esterno (Tavily ok, geekdo giù), non
		// "nessuna FAQ pertinente". Il chiamante lo trasforma nel
		// messaggio di indisponibilità per il modello.
		return nil, fmt.Errorf("faq: nessun thread leggibile: %w", lastErr)
	}
	return hits, nil
}
