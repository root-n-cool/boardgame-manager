package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// transcribeTimeout è più generoso di requestTimeout: una pagina di manuale
// a 2000x3000 pixel è molta immagine da leggere, e un modello economico non
// è veloce.
const transcribeTimeout = 120 * time.Second

// contentPart è una parte del contenuto di un messaggio nel formato
// OpenAI: o testo, o un'immagine come data URI.
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// multipartMessage è un messaggio il cui contenuto è un array di parti.
// chatMessage non basta: il suo Content è una stringa.
type multipartMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type visionRequest struct {
	Model       string            `json:"model"`
	Temperature float64           `json:"temperature"`
	Messages    []json.RawMessage `json:"messages"`
}

const transcribeSystemPrompt = "Trascrivi in markdown il testo della pagina di regolamento che ricevi come immagine. " +
	"Riporta TUTTO il testo leggibile, nell'ordine di lettura, senza riassumere e senza commentare. " +
	"Conserva titoli, elenchi e tabelle. Ignora le illustrazioni che non contengono testo. " +
	"Se la pagina è illeggibile o non contiene testo, rispondi con la sola parola VUOTA."

// Transcribe legge una pagina scansionata di manuale e ne restituisce il
// testo in markdown. Si paga una volta per pagina, all'ingestione: è il
// motivo per cui a domanda non si mandano più immagini al modello.
func (c *HTTPClient) Transcribe(ctx context.Context, jpeg []byte, pageNumber int) (string, error) {
	if c.BaseURL == "" || c.APIKey == "" || c.VisionModel == "" {
		return "", ErrNotConfigured
	}

	system, err := json.Marshal(chatMessage{Role: "system", Content: transcribeSystemPrompt})
	if err != nil {
		return "", err
	}
	user, err := json.Marshal(multipartMessage{
		Role: "user",
		Content: []contentPart{
			{Type: "text", Text: fmt.Sprintf("Pagina %d del regolamento.", pageNumber)},
			{Type: "image_url", ImageURL: &imageURL{
				URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg),
			}},
		},
	})
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(visionRequest{
		Model:       c.VisionModel,
		Temperature: 0,
		Messages:    []json.RawMessage{system, user},
	})
	if err != nil {
		return "", err
	}

	out, err := c.postChat(ctx, payload, transcribeTimeout)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(strings.TrimSpace(out), "VUOTA") {
		// Una pagina di sole illustrazioni: non è un errore, ma non è
		// nemmeno testo. Il chiamante la salva vuota.
		return "", nil
	}
	if strings.TrimSpace(out) == "" {
		// Un guasto del modello (risposta bianca) è diverso da una pagina
		// vuota (risposta "VUOTA"): qui l'admin deve poter riprovare.
		return "", fmt.Errorf("il modello non ha restituito testo per la pagina %d", pageNumber)
	}
	return strings.TrimSpace(out), nil
}

// postRaw manda una richiesta già serializzata a /chat/completions e
// restituisce il body grezzo della risposta. Separato da postChat perché
// Ask (task successivo) deve ispezionare finish_reason e tool_calls, non
// solo il testo di una scelta.
func (c *HTTPClient) postRaw(ctx context.Context, payload []byte, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ai response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("ai provider returned status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// postChat manda una richiesta già serializzata a /chat/completions e
// restituisce il contenuto della prima scelta. Estratto perché Translate,
// Transcribe e Ask fanno la stessa danza di HTTP ed errori.
func (c *HTTPClient) postChat(ctx context.Context, payload []byte, timeout time.Duration) (string, error) {
	body, err := c.postRaw(ctx, payload, timeout)
	if err != nil {
		return "", err
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse ai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("ai provider returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
