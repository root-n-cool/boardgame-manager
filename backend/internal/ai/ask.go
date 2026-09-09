package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
		Model: c.VisionModel,
		// temperature 0: una trascrizione non deve cambiare a ogni
		// tentativo, e una che varia tra un retry e l'altro sarebbe
		// peggio di una semplicemente imperfetta.
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
// Ask deve ispezionare finish_reason e tool_calls, non solo il testo di
// una scelta.
//
// Il timeout è governato dal contesto passato qui, non da
// http.Client.Timeout: quel campo è fissato una volta per tutte in
// NewHTTPClient (60s, pensato per Translate) e su un client condiviso
// vincerebbe silenziosamente su qualunque timeout più lungo richiesto da
// una singola chiamata — è esattamente quello che succedeva ai 120s di
// Transcribe prima di questo fix.
func (c *HTTPClient) postRaw(ctx context.Context, payload []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	} else if httpClient.Timeout != 0 {
		// Non lasciare che il Timeout fisso del client condiviso tagli
		// corto il timeout appena impostato sul contesto: qui a decidere
		// deve essere solo quest'ultimo.
		clientCopy := *httpClient
		clientCopy.Timeout = 0
		httpClient = &clientCopy
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
// restituisce il contenuto della prima scelta. Usata da Transcribe; Ask usa
// postRaw direttamente perché deve ispezionare finish_reason e tool_calls,
// non solo il testo. Translate resta a parte: il suo codice HTTP inline è
// precedente a questo file e non tocca né l'uno né l'altro helper.
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

// InlineCorpusMaxChars è la soglia sotto la quale il manuale entra intero
// nel contesto e il tool non viene nemmeno dichiarato. ~6.000 token,
// stimati a 3 caratteri per token (conservativo per l'italiano).
//
// È la leva più efficace per ridurre le chiamate al tool: non offrirlo. Un
// manuale di 4 pagine (il regolamento vero di questo progetto) sta
// largamente sotto.
const InlineCorpusMaxChars = 18000

// MaxToolIterations ferma un modello che si incarta a richiamare lo stesso
// tool in ciclo. Non è un limite sull'utente: è protezione da un bug del
// modello, e quando scatta si forza una risposta togliendo il tool invece
// di restituire un errore.
const MaxToolIterations = 5

// askTimeout è il tetto complessivo di una domanda, giri del tool
// compresi. Un handler HTTP senza timeout tiene una goroutine occupata per
// sempre.
const askTimeout = 60 * time.Second

// Turn è un messaggio della conversazione così come arriva dal browser. Lo
// storico non è persistito da nessuna parte: vive nel componente di chat
// del frontend e torna indietro a ogni domanda.
type Turn struct {
	Role string
	Text string
}

// SearchFunc cerca nel manuale e restituisce il payload già formattato per
// il modello, come stringa. È una funzione e non un'interfaccia sui tipi di
// manuals: così questo pacchetto non conosce SQLite né manuals.Hit, e il
// loop si testa con una closure di due righe.
type SearchFunc func(ctx context.Context, keywords []string) (string, error)

// Asker è l'astrazione che serve all'handler pubblico. HTTPClient la
// implementa; i test iniettano un finto.
type Asker interface {
	Ask(ctx context.Context, req AskRequest) (string, error)
}

type AskRequest struct {
	GameName string
	Turns    []Turn
	// CorpusChars decide inline vs tool; CorpusText è il manuale intero
	// (serve solo sotto soglia); CorpusIndex è l'indice dei titoli (serve
	// sempre quando c'è, ed è quel che evita la chiamata esplorativa).
	CorpusChars int
	CorpusText  string
	CorpusIndex string
	Search      SearchFunc
}

const searchToolName = "cerca_nel_manuale"

// searchToolSchema è dove sta il lavoro di "far fare al modello una sola
// chiamata": la descrizione del parametro chiede le varianti tutte insieme,
// con un esempio. Non c'è nessuna regola nel prompt che vieti la seconda
// chiamata — si rende inutile, non si proibisce: vietarla negherebbe il
// riprovare proprio quando la prima ricerca non ha trovato niente.
const searchToolSchema = `{
  "type": "object",
  "properties": {
    "parole_chiave": {
      "type": "array",
      "items": {"type": "string"},
      "maxItems": 8,
      "description": "Da 3 a 8 varianti della stessa cosa: sinonimi, il termine tecnico e quello colloquiale, singolare e plurale. Esempio: [\"pareggio\", \"stesso punteggio\", \"parità\", \"spareggio\"]. Supporta \"frase esatta\", OR, AND e i prefissi con *."
    }
  },
  "required": ["parole_chiave"]
}`

type toolFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolDef struct {
	Type     string          `json:"type"`
	Function toolFunctionDef `json:"function"`
}

type askRequestBody struct {
	Model       string            `json:"model"`
	Temperature float64           `json:"temperature"`
	Messages    []json.RawMessage `json:"messages"`
	Tools       []toolDef         `json:"tools,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type askChoice struct {
	FinishReason string          `json:"finish_reason"`
	Message      json.RawMessage `json:"message"`
}

type askResponseBody struct {
	Choices []askChoice `json:"choices"`
}

// assistantMessage legge quel che serve dal messaggio assistant di una
// risposta: Content può arrivare null quando c'è un tool_calls, e un
// errore di unmarshal sul Content non deve far perdere i ToolCalls già
// letti — per questo il chiamante ignora l'errore di json.Unmarshal e
// guarda solo cosa è arrivato a buon fine nei campi.
type assistantMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

type toolResultMessage struct {
	Role       string `json:"role"`
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// Ask risponde a una domanda sulle regole di un gioco. Se il manuale sta
// sotto InlineCorpusMaxChars entra intero nel prompt e il giro è uno solo;
// altrimenti il modello riceve il tool di ricerca e l'indice del manuale,
// e il loop prosegue finché non risponde o non scatta MaxToolIterations.
func (c *HTTPClient) Ask(ctx context.Context, req AskRequest) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()

	inline := req.CorpusChars > 0 && req.CorpusChars <= InlineCorpusMaxChars
	// toolsDeclared è la sola condizione che decide se il tool compare
	// nella richiesta E se il prompt promette di poterlo usare: calcolata
	// una volta, usata da entrambi, così le due cose non possono
	// disallinearsi. Senza questo, un manuale sopra soglia ma senza
	// Search (req.Search == nil — non dovrebbe succedere nell'uso reale,
	// ma è difensivo) produrrebbe un prompt che dice "usa lo strumento di
	// ricerca" mentre la richiesta non dichiara nessun tool: il modello
	// annasperebbe dietro un'istruzione impossibile da eseguire.
	toolsDeclared := !inline && req.Search != nil

	system, err := json.Marshal(chatMessage{Role: "system", Content: askSystemPrompt(req, inline, toolsDeclared)})
	if err != nil {
		return "", err
	}
	messages := []json.RawMessage{system}
	for _, t := range req.Turns {
		role := "user"
		if t.Role == "assistant" || t.Role == "ai" {
			role = "assistant"
		}
		raw, err := json.Marshal(chatMessage{Role: role, Content: t.Text})
		if err != nil {
			return "", err
		}
		messages = append(messages, raw)
	}

	var tools []toolDef
	if toolsDeclared {
		tools = append(tools, toolDef{
			Type: "function",
			Function: toolFunctionDef{
				Name: searchToolName,
				Description: "Cerca nel regolamento del gioco. Passa in un'unica chiamata " +
					"tutte le varianti lessicali plausibili: la ricerca è lessicale, " +
					"quindi più varianti trovano più cose.",
				Parameters: json.RawMessage(searchToolSchema),
			},
		})
	}

	for iteration := 0; ; iteration++ {
		// Alla scadenza della guardia si rifà la richiesta senza tool: il
		// modello è costretto a rispondere con quello che ha, invece di
		// restare a girare.
		active := tools
		if iteration >= MaxToolIterations {
			active = nil
		}

		payload, err := json.Marshal(askRequestBody{
			Model:       c.Model,
			Temperature: 0.2,
			Messages:    messages,
			Tools:       active,
		})
		if err != nil {
			return "", err
		}

		raw, err := c.postRaw(ctx, payload, askTimeout)
		if err != nil {
			return "", err
		}
		var parsed askResponseBody
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", fmt.Errorf("parse ai response: %w", err)
		}
		if len(parsed.Choices) == 0 {
			return "", errors.New("ai provider returned no choices")
		}
		choice := parsed.Choices[0]

		var msg assistantMessage
		// Content può arrivare null quando ci sono tool_calls: un errore di
		// unmarshal qui non deve far perdere i tool_calls, quindi si ignora
		// e si guarda cosa si è riusciti a leggere.
		_ = json.Unmarshal(choice.Message, &msg)

		if len(msg.ToolCalls) == 0 {
			answer := strings.TrimSpace(msg.Content)
			if answer == "" {
				return "", errors.New("ai provider returned an empty answer")
			}
			return answer, nil
		}
		if active == nil {
			// Il modello insiste col tool in una richiesta che non ne
			// dichiara nessuno: non c'è altro da fare che dirlo.
			return "", errors.New("ai provider kept calling a tool that was not offered")
		}

		// Il messaggio assistant va rimandato verbatim (il json.RawMessage
		// grezzo della risposta, non una sua ri-serializzazione da
		// assistantMessage): altrimenti il provider non riconosce a cosa
		// risponde il tool_call_id che segue, o si perde un campo che non
		// abbiamo modellato ma che il provider si aspetta di rivedere.
		messages = append(messages, choice.Message)

		for _, call := range msg.ToolCalls {
			result := "Tool sconosciuto."
			if call.Function.Name == searchToolName && req.Search != nil {
				keywords := parseKeywords(call.Function.Arguments)
				out, err := req.Search(ctx, keywords)
				if err != nil {
					log.Printf("ask: manual search failed: %v", err)
					result = "La ricerca nel manuale non è disponibile in questo momento."
				} else {
					result = out
				}
			}
			resultRaw, err := json.Marshal(toolResultMessage{
				Role: "tool", ToolCallID: call.ID, Content: result,
			})
			if err != nil {
				return "", err
			}
			messages = append(messages, resultRaw)
		}

		if iteration+1 >= MaxToolIterations {
			log.Printf("ask: il modello ha chiamato %s %d volte: forzo la risposta senza tool", searchToolName, iteration+1)
		}
	}
}

// parseKeywords legge l'argomento del tool. Accetta sia l'array previsto
// dallo schema sia una stringa separata da virgole, perché i modelli
// economici a volte mandano la seconda: rifiutarla significherebbe perdere
// la domanda per un dettaglio di serializzazione.
func parseKeywords(arguments string) []string {
	var asArray struct {
		Keywords []string `json:"parole_chiave"`
	}
	if err := json.Unmarshal([]byte(arguments), &asArray); err == nil && len(asArray.Keywords) > 0 {
		return trimAll(asArray.Keywords)
	}
	var asString struct {
		Keywords string `json:"parole_chiave"`
	}
	if err := json.Unmarshal([]byte(arguments), &asString); err == nil && strings.TrimSpace(asString.Keywords) != "" {
		return trimAll(strings.Split(asString.Keywords, ","))
	}
	return nil
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// askSystemPrompt costruisce le istruzioni. La regola che conta è la terza:
// al tavolo una regola inventata fa più danno di un "non lo dice".
//
// toolsDeclared governa se si promette lo strumento di ricerca: deve
// essere la stessa condizione che decide se il tool compare nella
// richiesta (vedi Ask), altrimenti il prompt può promettere uno strumento
// che il modello non ha davvero a disposizione.
func askSystemPrompt(req AskRequest, inline, toolsDeclared bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sei l'assistente regole di %q per un'associazione di giochi da tavolo. ", req.GameName)
	b.WriteString("Chi ti scrive è in piedi a un tavolo, con le carte in mano: rispondi in italiano, breve, come si parla. ")
	b.WriteString("Rispondi SOLO con quello che c'è nel regolamento. ")
	b.WriteString("Se il regolamento non lo dice, dillo chiaramente invece di dedurre: al tavolo una regola inventata fa danno. ")
	b.WriteString("Cita sempre la pagina da cui viene la risposta, nella forma \"Regolamento base, pag. 7\". ")
	b.WriteString("Non inventare nomi di carte, valori o numeri che non hai letto.\n\n")

	if req.CorpusIndex != "" {
		fmt.Fprintf(&b, "Indice del regolamento: %s\n\n", req.CorpusIndex)
	}
	switch {
	case inline:
		b.WriteString("Il regolamento completo:\n\n")
		b.WriteString(req.CorpusText)
	case toolsDeclared:
		b.WriteString("Per leggere il regolamento usa lo strumento di ricerca. ")
		b.WriteString("Se una ricerca non trova nulla, riprova con altre parole prima di dire che il manuale non lo dice.")
	default:
		// Non dovrebbe succedere nell'uso reale (Task 9 passa sempre
		// Search sopra soglia), ma se capitasse non si deve promettere
		// uno strumento che non è stato dichiarato: meglio dire al
		// modello di limitarsi all'indice piuttosto che fargli credere
		// di poter cercare quando non può.
		b.WriteString("Non hai a disposizione né il testo completo né uno strumento di ricerca: rispondi solo se l'indice qui sopra basta, altrimenti di' che non puoi controllare il regolamento in questo momento.")
	}
	return b.String()
}
