package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// suggestTimeout è corto rispetto a segmentTimeout: la richiesta è ~200
// token di titoli e la risposta sono tre righe. Se non torna in mezzo
// minuto non tornerà.
const suggestTimeout = 30 * time.Second

// MaxSuggestionChars è il tetto per domanda. Le domande vivono in tre
// bottoni su uno schermo di telefono: una domanda di duecento caratteri
// non è una domanda suggerita, è un paragrafo.
//
// Esportata perché il tetto è UNO: una domanda scritta a mano dall'admin
// vive negli stessi tre bottoni di una generata, e il validatore della
// rotta PUT (httpapi) deve usare questo valore, non una sua copia.
const MaxSuggestionChars = 120

// ErrSuggestionsRejected dice che la risposta del modello non è tre
// domande. È un errore distinto da un guasto di rete perché il chiamante
// deve poterlo raccontare diversamente: al pannello admin serve "il
// modello non ha risposto come doveva, riprova", non un errore generico.
var ErrSuggestionsRejected = errors.New("ai suggestions rejected: response is not three questions")

// QuestionSuggester è l'astrazione che serve all'indicizzazione e al
// pannello admin. HTTPClient la implementa; i test iniettano un finto.
// Stesso schema di Segmenter e Transcriber.
type QuestionSuggester interface {
	SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error)
}

// suggestSystemPrompt chiede tre domande e vieta tutto il resto. Non è una
// garanzia — la mitigazione vera è parseSuggestions — ma è ciò che rende
// il rifiuto raro invece che normale.
const suggestSystemPrompt = "Ricevi il nome di un gioco da tavolo e l'elenco dei titoli di sezione del suo regolamento. " +
	"Scrivi TRE domande che un giocatore farebbe al tavolo, in italiano, a cui il regolamento risponde.\n" +
	"Regole assolute:\n" +
	"1. Esattamente tre domande, una per riga. Nessuna numerazione, nessun elenco puntato, nessun preambolo, nessun commento.\n" +
	"2. Ogni riga deve finire con un punto di domanda.\n" +
	"3. Ogni domanda sta sotto i 120 caratteri: sono tre bottoni su uno schermo di telefono.\n" +
	"4. IGNORA i titoli che non sono regole: il nome dell'autore, il contenuto della scatola, l'indice, i ringraziamenti, i crediti.\n" +
	"5. Scrivi domande come le porrebbe un giocatore (\"Quando finisce la partita?\"), non come una ricerca nel manuale (\"Cosa dice il manuale sulla fine della partita?\").\n" +
	"6. Preferisci le domande che si fanno davvero durante una partita: preparazione, cosa si può fare nel proprio turno, come si contano i punti, quando finisce."

// SuggestQuestions chiede al modello tre domande per la scheda del gioco,
// partendo dai titoli di sezione del manuale già indicizzato.
//
// Usa il modello di TESTO (c.Model) e non quello vision: qui non c'è
// nessuna immagine. E non riprova sugli errori transitori — vale lo stesso
// confine di Segment: il retry vive in Transcribe, l'unica chiamata che si
// fa N volte per un solo documento.
func (c *HTTPClient) SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	if len(headings) == 0 {
		// Nessun titolo da cui partire: il chiamante deve dirlo all'admin
		// ("indicizza prima un manuale"), non ricevere tre domande
		// inventate dal nulla.
		return nil, ErrSuggestionsRejected
	}

	user := fmt.Sprintf("Gioco: %s\n\nTitoli delle sezioni del regolamento:\n- %s",
		gameName, strings.Join(headings, "\n- "))

	payload, err := json.Marshal(chatRequest{
		Model: c.Model,
		// temperature 0: le domande di un manuale non devono cambiare a
		// ogni indicizzazione. Se l'admin ne vuole altre, c'è "rigenera".
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: suggestSystemPrompt},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return nil, err
	}

	out, err := c.postChat(ctx, payload, suggestTimeout)
	if err != nil {
		return nil, err
	}
	return parseSuggestions(out)
}

// parseSuggestions valida la risposta del modello: esattamente tre righe
// non vuote, ciascuna una domanda e ciascuna abbastanza corta per un
// bottone. Qualunque altra cosa è ErrSuggestionsRejected.
//
// Le righe vuote si scartano prima di contare (un modello che separa le
// domande con una riga bianca non ha sbagliato niente di sostanziale), ma
// una riga di testo in più — un preambolo, un commento — fa sforare il
// conto ed è esattamente ciò che si vuole fermare.
func parseSuggestions(raw string) ([]string, error) {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}

	if len(out) != 3 {
		return nil, fmt.Errorf("%w: attese 3 righe, ricevute %d", ErrSuggestionsRejected, len(out))
	}
	for i, q := range out {
		if !strings.HasSuffix(q, "?") {
			return nil, fmt.Errorf("%w: la riga %d non è una domanda: %q", ErrSuggestionsRejected, i+1, q)
		}
		if len([]rune(q)) > MaxSuggestionChars {
			return nil, fmt.Errorf("%w: la riga %d supera %d caratteri", ErrSuggestionsRejected, i+1, MaxSuggestionChars)
		}
	}
	return out, nil
}
