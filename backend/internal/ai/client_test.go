package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
)

func TestTranslate_SendsAWellFormedRequest(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"Un gioco sugli uccelli."}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "gemini-flash-lite-latest")
	out, err := client.Translate(context.Background(), "A game about birds.", "it")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if out != "Un gioco sugli uccelli." {
		t.Fatalf("unexpected translation: %q", out)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("expected the OpenAI-compatible path, got %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("expected a bearer token, got %q", gotAuth)
	}

	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("the request body is not valid JSON: %v", err)
	}
	if sent.Model != "gemini-flash-lite-latest" {
		t.Fatalf("expected the configured model, got %q", sent.Model)
	}
	if len(sent.Messages) != 2 || sent.Messages[0].Role != "system" || sent.Messages[1].Role != "user" {
		t.Fatalf("expected a system prompt plus the text as the user message, got %+v", sent.Messages)
	}
	// Il codice ISO da solo confonde i modelli piccoli: nel prompt deve
	// finire il nome esteso della lingua.
	if !strings.Contains(strings.ToLower(sent.Messages[0].Content), "italiano") {
		t.Fatalf("expected the language spelled out in the system prompt, got %q", sent.Messages[0].Content)
	}
	if sent.Messages[1].Content != "A game about birds." {
		t.Fatalf("expected the source text as the user message, got %q", sent.Messages[1].Content)
	}
}

func TestTranslate_NotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	if _, err := client.Translate(context.Background(), "text", "it"); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}

	partial := ai.NewHTTPClient("https://api.example.org/v1", "sk-test", "")
	if _, err := partial.Translate(context.Background(), "text", "it"); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("expected a missing model to count as not configured, got %v", err)
	}
}

func TestTranslate_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"quota exceeded"}}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	_, err := client.Translate(context.Background(), "text", "it")
	if err == nil {
		t.Fatal("expected an error on a non-2xx response")
	}
	if errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("a provider failure is not a missing configuration: %v", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected the status code in the error, got %v", err)
	}
}

func TestTranslate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Translate(context.Background(), "text", "it"); err == nil {
		t.Fatal("expected an error when the provider returns no choices")
	}
}

func TestTranslate_TrimsTrailingSlashInBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL+"/", "sk-test", "m")
	if _, err := client.Translate(context.Background(), "text", "it"); err != nil {
		t.Fatalf("translate: %v", err)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("expected a single slash before the path, got %q", gotPath)
	}
}

func TestTranslate_UnknownLanguageCodeUsesTheCodeItself(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Translate(context.Background(), "text", "sv"); err != nil {
		t.Fatalf("translate: %v", err)
	}
	// Una lingua fuori dalla tabella non deve bloccare la traduzione: il
	// codice finisce nel prompt così com'è.
	if !strings.Contains(gotBody, "sv") {
		t.Fatalf("expected the raw code in the prompt, got %q", gotBody)
	}
}

// I test qui sotto riguardano `reasoning_effort`. Il perché sta in
// client.go accanto alla costante: sul provider del club il modello
// configurato è un modello di ragionamento, e senza chiedergli di non
// ragionare una traduzione di una frase ha misurato 73 secondi contro i
// 2,2 con il parametro — cioè oltre il tetto di 60s di questo client, da
// cui i 502 in produzione.

func TestTranslate_AsksTheModelNotToReason(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		io.WriteString(w, `{"choices":[{"message":{"content":"Un gioco."}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "deepseek-v4-flash")
	if _, err := client.Translate(context.Background(), "A game.", "it"); err != nil {
		t.Fatalf("translate: %v", err)
	}

	var sent struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("the request body is not valid JSON: %v", err)
	}
	if sent.ReasoningEffort != "none" {
		t.Fatalf(`expected reasoning_effort "none", got %q`, sent.ReasoningEffort)
	}
}

func TestTranscribe_AsksTheModelNotToReason(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		io.WriteString(w, `{"choices":[{"message":{"content":"# Pagina"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "m", "mv")
	if _, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8}, 1); err != nil {
		t.Fatalf("transcribe: %v", err)
	}

	var sent struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("the request body is not valid JSON: %v", err)
	}
	if sent.ReasoningEffort != "none" {
		t.Fatalf(`expected reasoning_effort "none" on the vision request, got %q`, sent.ReasoningEffort)
	}
}

// Un provider che non conosce il campo lo nomina nel suo 400: la richiesta
// si rifà una volta sola senza il campo, così l'app resta agnostica sul
// provider invece di rompersi su quelli che non lo accettano.
func TestTranslate_RetriesWithoutReasoningEffortWhenTheProviderRejectsIt(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if strings.Contains(string(body), "reasoning_effort") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":{"message":"Unrecognized request argument supplied: reasoning_effort"}}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"Un gioco."}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "gpt-4o-mini")
	out, err := client.Translate(context.Background(), "A game.", "it")
	if err != nil {
		t.Fatalf("expected the retry without reasoning_effort to succeed, got %v", err)
	}
	if out != "Un gioco." {
		t.Fatalf("unexpected translation: %q", out)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected exactly two requests (one refused, one retried), got %d", len(bodies))
	}
	if strings.Contains(bodies[1], "reasoning_effort") {
		t.Fatalf("expected the retry to drop the field, got %s", bodies[1])
	}
	// Il resto della richiesta non deve cambiare: si toglie un campo, non
	// si ricostruisce la domanda.
	var first, second map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &first); err != nil {
		t.Fatalf("first body: %v", err)
	}
	if err := json.Unmarshal([]byte(bodies[1]), &second); err != nil {
		t.Fatalf("second body: %v", err)
	}
	delete(first, "reasoning_effort")
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatalf("expected only reasoning_effort to differ:\n%v\n%v", first, second)
	}
}

// Una volta scoperto che il provider lo rifiuta, il campo non si manda
// più: pagare un 400 di andata e ritorno a ogni pagina di un manuale da
// trenta pagine sarebbe trenta round-trip buttati.
func TestClient_StopsSendingReasoningEffortAfterARefusal(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if strings.Contains(string(body), "reasoning_effort") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"unknown field reasoning_effort"}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"Un gioco."}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Translate(context.Background(), "A game.", "it"); err != nil {
		t.Fatalf("first translate: %v", err)
	}
	if _, err := client.Translate(context.Background(), "Another game.", "it"); err != nil {
		t.Fatalf("second translate: %v", err)
	}
	if len(bodies) != 3 {
		t.Fatalf("expected 3 requests (refused, retried, then one clean), got %d", len(bodies))
	}
	if strings.Contains(bodies[2], "reasoning_effort") {
		t.Fatalf("expected the second call to skip the field entirely, got %s", bodies[2])
	}
}

// Un 400 che non parla di reasoning_effort è un errore vero: rifare la
// richiesta senza il campo nasconderebbe la causa e raddoppierebbe il
// traffico su ogni richiesta malformata.
func TestTranslate_DoesNotRetryOnAnUnrelatedBadRequest(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"message":"model not found"}}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "nope")
	if _, err := client.Translate(context.Background(), "A game.", "it"); err == nil {
		t.Fatal("expected the bad request to surface as an error")
	}
	if calls != 1 {
		t.Fatalf("expected a single request, got %d", calls)
	}
}
