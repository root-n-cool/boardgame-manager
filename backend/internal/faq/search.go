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
	Thread(ctx context.Context, threadID string) (bgg.Thread, error)
}

// Hit è un commento del forum pronto per il modello. CommentURL e
// ThreadURL non escono al modello: servono all'handler per i link.
type Hit struct {
	Reference       string
	ReferenceDetail string
	Text            string
	CommentURL      string
	ThreadURL       string
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
)

var threadIDPattern = regexp.MustCompile(`boardgamegeek\.com/thread/(\d+)`)

// Search è legata al gioco dal chiamante: bggID non è un parametro del
// tool, così il modello non può leggere il forum di un altro gioco.
func Search(ctx context.Context, s websearch.Searcher, f ThreadFetcher, gameName, bggID, query string) ([]Hit, error) {
	sctx, cancel := context.WithTimeout(ctx, callTimeout)
	results, err := s.Search(sctx, fmt.Sprintf("%q %s rules", gameName, query), []string{"boardgamegeek.com/thread"}, SearchResults)
	cancel()
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
	usedRefs := map[string]bool{}
	accepted := 0
	for _, id := range ids {
		if accepted >= MaxThreads {
			break
		}
		tctx, cancel := context.WithTimeout(ctx, callTimeout)
		th, err := f.Thread(tctx, id)
		cancel()
		if err != nil {
			log.Printf("faq: thread %s: %v", id, err)
			continue
		}
		// Un thread di un'espansione o di un gioco omonimo, o fuori dal
		// forum Rules (Variants sono regole della casa), non è una FAQ di
		// questo gioco.
		if th.ObjectType != "things" || th.ObjectID != bggID || th.Forum != "Rules" || len(th.Articles) == 0 {
			continue
		}
		accepted++

		ref := "BGG: " + th.Subject
		if usedRefs[ref] {
			ref += " #" + th.ID
		}
		usedRefs[ref] = true
		threadURL := "https://boardgamegeek.com/thread/" + th.ID

		budget := ThreadCharBudget
		for i, a := range th.Articles {
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
			hits = append(hits, Hit{
				Reference:       ref,
				ReferenceDetail: "commento del " + a.PostDate.Format("02/01/2006"),
				Text:            text,
				CommentURL:      a.Link,
				ThreadURL:       threadURL,
			})
		}
	}
	return hits, nil
}
