// Package ai parla con un provider di modelli linguistici che espone il
// formato OpenAI (chat/completions). Nel progetto serve a tradurre le
// descrizioni scaricate da BoardGameGeek, che arrivano solo in inglese.
//
// Il formato OpenAI è qui l'astrazione sul provider: con base URL,
// chiave e modello configurabili la stessa implementazione parla con
// Google Gemini, OpenAI, OpenRouter, Groq o un Ollama in locale.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
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

// Transcriber è l'astrazione che serve agli handler: leggere una pagina
// scansionata. HTTPClient la implementa (Transcribe è in ask.go).
type Transcriber interface {
	Transcribe(ctx context.Context, jpeg []byte, pageNumber int) (string, error)
}

type HTTPClient struct {
	// BaseURL è la radice OpenAI-compatible, senza /chat/completions.
	BaseURL string
	APIKey  string
	Model   string
	// VisionModel è il modello per leggere le pagine scansionate. Separato
	// da Model perché un modello di chat può essere solo-testo:
	// deepseek-v4-flash non accetta immagini, deepseek-v4-flash-vision-exp
	// sì. Vuoto = nessuna trascrizione automatica, e non è un guasto.
	VisionModel string
	HTTPClient  *http.Client

	// reasoningEffortRefused ricorda che questo provider ha risposto 4xx
	// nominando `reasoning_effort`: da lì in poi il campo non si manda
	// più (vedi postRaw). Atomico perché un client è condiviso da tutte
	// le pagine di un manuale, trascritte cinque alla volta.
	reasoningEffortRefused atomic.Bool
}

func NewHTTPClient(baseURL, apiKey, model string) *HTTPClient {
	return &HTTPClient{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:     strings.TrimSpace(apiKey),
		Model:      strings.TrimSpace(model),
		HTTPClient: &http.Client{Timeout: requestTimeout},
	}
}

// NewHTTPClientWithVision è NewHTTPClient più il modello di trascrizione.
// NewHTTPClient resta per i chiamanti che traducono e non leggono manuali.
func NewHTTPClientWithVision(baseURL, apiKey, model, visionModel string) *HTTPClient {
	c := NewHTTPClient(baseURL, apiKey, model)
	c.VisionModel = strings.TrimSpace(visionModel)
	return c
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

// reasoningEffortNone chiede al modello di rispondere senza ragionare ad
// alta voce prima. Va su OGNI richiesta, non solo dove sembra pesante: i
// modelli di ragionamento pensano anche per tradurre una frase.
//
// Misurato sul provider del club (opencode.ai/zen, deepseek-v4-flash) con
// la stessa richiesta che manda Translate: 73,5s e 323 token di completion
// senza il parametro — di cui 1.190 caratteri di `reasoning_content` e 113
// di traduzione — contro 2,2s e 38 token con. Sulla descrizione intera di
// un gioco: 56,6s, con 12.988 caratteri di ragionamento contro 1.720 di
// testo tradotto. Il client non fa streaming, quindi quel tempo scorre
// prima che arrivi il primo byte di header, e i tetti di questo pacchetto
// (60s qui, 30s per le domande suggerite) scattavano prima del modello: in
// produzione erano 502 sulla traduzione e pagine saltate
// nell'indicizzazione.
//
// Il campo non è universale — OpenAI lo accetta ma il suo valore minimo è
// `minimal`, e un server OpenAI-compatible che non lo conosce può
// rispondere 400 — quindi postRaw sa toglierlo e rifare la richiesta una
// volta sola (vedi refusesReasoningEffort in ask.go). L'app resta
// agnostica sul provider e chi installa non deve configurare niente.
const reasoningEffortNone = "none"

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	// omitempty perché postRaw ricostruisce il payload senza il campo
	// quando il provider lo rifiuta: un `""` esplicito sarebbe un valore
	// non valido da mandare.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
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
			"Mantieni gli a capo e i paragrafi dell'originale. Lascia invariati i nomi propri, i titoli dei giochi e delle espansioni. "+
			"Puoi usare Markdown semplice (grassetto, corsivo, titoli, elenchi puntati o numerati) se aiuta a "+
			"rendere leggibile la struttura del testo originale, ma non è un obbligo: se l'originale è un unico "+
			"paragrafo di prosa, resta un unico paragrafo di prosa. Non racchiudere mai l'intera risposta in un "+
			"blocco di codice.",
		languageName(targetLang),
	)

	// temperature 0: una traduzione non deve cambiare a ogni tentativo.
	payload, err := json.Marshal(chatRequest{
		Model:           c.Model,
		Temperature:     0,
		ReasoningEffort: reasoningEffortNone,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: text},
		},
	})
	if err != nil {
		return "", err
	}

	// postChat e non un giro HTTP proprio: era l'unica chiamata del
	// pacchetto con la sua copia di quel codice, e restarne fuori
	// significava restare fuori anche dalla ricaduta su
	// `reasoning_effort` rifiutato — che è esattamente la chiamata su cui
	// il problema si è visto. Il tetto passa dal contesto, come per tutte
	// le altre.
	out, err := c.postChat(ctx, payload, requestTimeout)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", errors.New("ai provider returned an empty translation")
	}
	return out, nil
}
