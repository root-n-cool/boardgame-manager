// Package ai parla con un provider di modelli linguistici che espone il
// formato OpenAI (chat/completions). Nel progetto serve a tradurre le
// descrizioni scaricate da BoardGameGeek, che arrivano solo in inglese.
//
// Il formato OpenAI è qui l'astrazione sul provider: con base URL,
// chiave e modello configurabili la stessa implementazione parla con
// Google Gemini, OpenAI, OpenRouter, Groq o un Ollama in locale.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured dice che l'admin non ha (ancora) messo un provider.
// Non è un guasto: è l'app senza AI, e chi la usa così non deve vedere
// errori da nessuna parte.
var ErrNotConfigured = errors.New("ai provider not configured")

// requestTimeout è generoso di proposito: una descrizione BGG sono
// qualche migliaio di caratteri e i modelli economici non sono veloci.
const requestTimeout = 60 * time.Second

type Translator interface {
	Translate(ctx context.Context, text, targetLang string) (string, error)
}

type HTTPClient struct {
	// BaseURL è la radice OpenAI-compatible, senza /chat/completions.
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewHTTPClient(baseURL, apiKey, model string) *HTTPClient {
	return &HTTPClient{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:     strings.TrimSpace(apiKey),
		Model:      strings.TrimSpace(model),
		HTTPClient: &http.Client{Timeout: requestTimeout},
	}
}

// languageNames rende leggibile il codice ISO: "traduci in italiano"
// funziona su qualunque modello, "traduci in it" no. Una lingua fuori da
// questa tabella passa col suo codice invece di bloccare la traduzione.
var languageNames = map[string]string{
	"it": "italiano",
	"en": "inglese",
	"fr": "francese",
	"de": "tedesco",
	"es": "spagnolo",
}

func languageName(code string) string {
	if name, ok := languageNames[strings.ToLower(strings.TrimSpace(code))]; ok {
		return name
	}
	return code
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (c *HTTPClient) configured() bool {
	return c.BaseURL != "" && c.APIKey != "" && c.Model != ""
}

func (c *HTTPClient) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}

	system := fmt.Sprintf(
		"Traduci in %s il testo che ricevi. È la descrizione di un gioco da tavolo presa da BoardGameGeek. "+
			"Rispondi con il solo testo tradotto: nessun commento, nessun preambolo, nessuna virgoletta intorno. "+
			"Mantieni gli a capo e i paragrafi dell'originale. Lascia invariati i nomi propri, i titoli dei giochi e delle espansioni.",
		languageName(targetLang),
	)

	// temperature 0: una traduzione non deve cambiare a ogni tentativo.
	payload, err := json.Marshal(chatRequest{
		Model:       c.Model,
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: text},
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ai response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("ai provider returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse ai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("ai provider returned no choices")
	}
	out := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if out == "" {
		return "", errors.New("ai provider returned an empty translation")
	}
	return out, nil
}
