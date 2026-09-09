package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
)

// Il brief dà questo test asserendo solo "err != nil": così com'è
// passerebbe anche per un motivo qualunque (per esempio una risposta che
// il client non riesce a deserializzare), che è esattamente la forma di
// difetto già vista nel Task 3. Qui si asserisce con errors.Is sul
// sentinel, cosa che il Task 5 userà per distinguere "risposta inaffidabile,
// riprova" da un guasto generico.
func TestSegment_RejectsAResponseThatRewritesTheContent(t *testing.T) {
	// Il modello restituisce metà del testo: è il caso in cui ha riassunto
	// invece di segmentare, ed è quello che introdurrebbe regole inventate.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"## Preparazione\nSi distribuiscono."}}]}`)
	}))
	defer srv.Close()

	long := "Si distribuiscono cinque carte a ciascun giocatore. " +
		strings.Repeat("Poi si mescola il mazzo e si posa al centro del tavolo. ", 20)

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Segment(context.Background(), long); !errors.Is(err, ai.ErrSegmentationRejected) {
		t.Fatalf("expected ErrSegmentationRejected, got %v", err)
	}
}

func TestSegment_WithoutAProviderIsNotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	if _, err := client.Segment(context.Background(), "testo"); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}

// Un test che passasse un testo lungo e contasse solo le richieste al
// finto provider verificherebbe il numero di chiamate, non la continuità
// fra finestre. La proprietà che conta è che l'ultimo titolo di una
// finestra arrivi come contesto alla successiva: qui si asserisce sul
// corpo della SECONDA richiesta, cercando un titolo abbastanza
// distintivo da non poter comparire per caso nel prompt di sistema (nella
// fase precedente un test cercò "4" e fu soddisfatto da
// "deepseek-v4-flash").
func TestSegment_PassesTheLastHeadingOfAWindowAsContextToTheNext(t *testing.T) {
	var requestBodies []string
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requestBodies = append(requestBodies, string(body))
		callCount++

		var sent struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatalf("request body is not valid JSON: %v", err)
		}
		var userContent string
		for _, m := range sent.Messages {
			if m.Role == "user" {
				userContent = m.Content
			}
		}

		// La risposta echeggia il testo ricevuto con un titolo davanti:
		// così il confronto di lunghezza (titoli esclusi) resta sempre
		// entro soglia, qualunque sia la finestra, e il test non dipende
		// dalla mitigazione dello Step 1 per passare.
		heading := "## Seconda Finestra Qualsiasi\n"
		if callCount == 1 {
			heading = "## Fase Di Preparazione Coi Dadi Rossi\n"
		}
		resp, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": heading + userContent}},
			},
		})
		w.Write(resp)
	}))
	defer srv.Close()

	// Un solo, enorme paragrafo ripetuto ma separato da righe vuote:
	// abbastanza lungo da superare SegmentWindowMaxChars e produrre
	// esattamente due finestre, con molti confini di paragrafo lungo la
	// strada perché ogni finto paragrafo è breve.
	paragraph := "Questo è un paragrafo di prova ripetuto molte volte per riempire la finestra oltre il limite consentito. "
	long := strings.Repeat(paragraph+"\n\n", 900)
	if len(long) <= ai.SegmentWindowMaxChars || len(long) >= 2*ai.SegmentWindowMaxChars {
		t.Fatalf("fixture non calibrata: serve un testo che produca esattamente due finestre, lunghezza %d", len(long))
	}

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Segment(context.Background(), long); err != nil {
		t.Fatalf("segment: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected exactly 2 windows/requests, got %d", callCount)
	}
	if !strings.Contains(requestBodies[1], "Fase Di Preparazione Coi Dadi Rossi") {
		t.Fatalf("expected the last heading of the first window as context in the second request, got %q", requestBodies[1])
	}
}
