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
