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

func TestTranscribe_SendsTheImageAsAnImageURLPart(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"## Fase di Upkeep\n\nOgni giocatore paga una moneta."}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp")
	out, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8, 0xFF, 0xD9}, 4)
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if !strings.Contains(out, "Upkeep") {
		t.Fatalf("trascrizione inattesa: %q", out)
	}

	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("il body non è JSON valido: %v", err)
	}
	// Il modello di trascrizione è quello vision, NON quello di chat: il
	// modello di chat configurato può essere solo-testo.
	if sent.Model != "deepseek-v4-flash-vision-exp" {
		t.Fatalf("atteso il modello vision, inviato %q", sent.Model)
	}
	if len(sent.Messages) != 2 {
		t.Fatalf("attesi system + user, inviati %d messaggi", len(sent.Messages))
	}
	// Il contenuto utente deve essere un array di parti, con una parte
	// image_url che porta un data URI base64.
	if !strings.Contains(gotBody, `"type":"image_url"`) {
		t.Fatalf("manca la parte image_url:\n%s", gotBody)
	}
	if !strings.Contains(gotBody, "data:image/jpeg;base64,") {
		t.Fatalf("l'immagine non è un data URI base64:\n%s", gotBody)
	}
	// Il numero di pagina serve al modello per non inventare intestazioni.
	// Non basta cercare "4" nel body: ci compare comunque dentro
	// "deepseek-v4-flash-vision-exp" anche se il prompt non lo contenesse.
	if !strings.Contains(gotBody, "Pagina 4 del regolamento") {
		t.Fatalf("il numero di pagina non è nel prompt:\n%s", gotBody)
	}
}

func TestTranscribe_VuotaIsNotAnError(t *testing.T) {
	// Una pagina di sola illustrazione è un esito legittimo, non un
	// guasto: il chiamante deve poterla distinguere da un errore per
	// salvarla vuota invece di riproporla per un retry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"VUOTA"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "m", "mv")
	out, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8}, 2)
	if err != nil {
		t.Fatalf("VUOTA non deve essere un errore: %v", err)
	}
	if out != "" {
		t.Fatalf("attesa stringa vuota, ottenuto %q", out)
	}
}

func TestTranscribe_WithoutAVisionModelIsNotConfigured(t *testing.T) {
	// Provider configurato ma senza modello vision: è il caso dell'admin
	// che ha l'AI per le traduzioni e non ha impostato il campo nuovo.
	client := ai.NewHTTPClientWithVision("https://example.invalid", "sk-test", "deepseek-v4-flash", "")
	_, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8}, 1)
	if !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}

func TestTranscribe_EmptyAnswerIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"   "}}]}`)
	}))
	defer srv.Close()
	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "m", "mv")
	if _, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8}, 1); err == nil {
		t.Fatal("una trascrizione vuota deve essere un errore: l'admin deve poter riprovare quella pagina")
	}
}
