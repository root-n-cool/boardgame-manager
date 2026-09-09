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

func TestSuggestQuestions_UsesTheTextModelAndTheHeadings(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		io.WriteString(w, `{"choices":[{"message":{"content":"Come si piazza una tessera?\nQuando finisce la partita?\nCome si contano i punti?"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp")
	got, err := client.SuggestQuestions(context.Background(), "Carcassonne",
		[]string{"Preparazione", "Piazzare le tessere", "Conteggio dei punti"})
	if err != nil {
		t.Fatalf("suggest questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande, ottenute %d: %v", len(got), got)
	}
	if got[0] != "Come si piazza una tessera?" {
		t.Fatalf("prima domanda inattesa: %q", got[0])
	}

	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("il body non è JSON valido: %v", err)
	}
	// Qui non c'è nessuna immagine: deve usare il modello di testo.
	if sent.Model != "deepseek-v4-flash" {
		t.Fatalf("atteso il modello di testo, inviato %q", sent.Model)
	}
	// I titoli devono arrivare al modello, altrimenti inventa.
	if !strings.Contains(gotBody, "Piazzare le tessere") {
		t.Fatalf("i titoli di sezione non sono nel prompt:\n%s", gotBody)
	}
	if !strings.Contains(gotBody, "Carcassonne") {
		t.Fatalf("il nome del gioco non è nel prompt:\n%s", gotBody)
	}
}

func TestSuggestQuestions_RejectsAPreamble(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"Ecco tre domande:\nUno?\nDue?\nTre?"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	_, err := client.SuggestQuestions(context.Background(), "Gioco", []string{"Preparazione"})
	if !errors.Is(err, ai.ErrSuggestionsRejected) {
		t.Fatalf("atteso ErrSuggestionsRejected, ottenuto %v", err)
	}
}

func TestSuggestQuestions_WithoutHeadingsIsRejectedWithoutCallingTheProvider(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `{"choices":[{"message":{"content":"Uno?\nDue?\nTre?"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	_, err := client.SuggestQuestions(context.Background(), "Gioco", nil)
	if !errors.Is(err, ai.ErrSuggestionsRejected) {
		t.Fatalf("senza titoli è un rifiuto, ottenuto %v", err)
	}
	if calls != 0 {
		t.Fatalf("senza titoli non c'è niente da chiedere: fatte %d chiamate", calls)
	}
}

func TestSuggestQuestions_WithoutProviderIsNotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	_, err := client.SuggestQuestions(context.Background(), "Gioco", []string{"Preparazione"})
	if !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}
