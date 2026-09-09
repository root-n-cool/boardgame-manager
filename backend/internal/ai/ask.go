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
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// transcribeTimeout è il tetto per UN tentativo di trascrizione, non per
// la pagina: la pagina ne ha fino a tre (vedi transcribeBackoff), quindi
// il tempo massimo speso su una pagina sfortunata è tre volte questo più
// le attese fra i tentativi.
//
// Era 120s, quando il tentativo era uno solo. Sessanta perché con i retry
// un tetto alto è una trappola: sul manuale reale del club due pagine si
// sono fermate per 120 secondi interi senza che il provider rispondesse,
// e più il tetto è alto più tardi si scopre che quel tentativo era da
// buttare. Una pagina a 2000x3000 che un modello economico legge davvero
// risponde molto prima; oltre il minuto, riprovare rende più che
// aspettare.
const transcribeTimeout = 60 * time.Second

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

	out, err := c.postChatRetrying(ctx, payload, transcribeTimeout)
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

// transcribeBackoff è l'attesa fra i tentativi di trascrizione di UNA
// pagina, quando il provider non dice lui quanto aspettare. La sua
// lunghezza è anche il numero di tentativi in più oltre il primo: tre
// tentativi in tutto, cioè al più 3 secondi d'attesa aggiunti a una
// pagina, contro i 120 di transcribeTimeout.
//
// Due retry e non di più perché la parallelizzazione li rende frequenti,
// non rari: se cinque pagine insieme prendono un 429, riprovarle a
// scaglioni distanziati risolve; se il provider è davvero in ginocchio,
// insistere non lo rimette in piedi e l'admin deve vedere l'errore.
var transcribeBackoff = [...]time.Duration{1 * time.Second, 2 * time.Second}

// maxRetryAfter limita quanto si onora un Retry-After. È un numero che
// arriva dalla rete: un provider confuso che chiede un'ora terrebbe
// occupata la request HTTP dell'admin fino al timeout, con
// l'indicizzazione ferma e nessuna spiegazione a schermo.
const maxRetryAfter = 20 * time.Second

// retryDelay è quanto aspettare dopo il tentativo numero attempt (0-based)
// fallito con err. È una funzione pura, separata dal ciclo che dorme,
// così la politica si verifica senza aspettare i secondi veri.
//
// Il Retry-After del provider vince sull'attesa di base in ENTRAMBE le
// direzioni: se lui sa dire quando riprovare, ne sa più di noi.
func retryDelay(attempt int, err error) time.Duration {
	delay := transcribeBackoff[attempt]
	var status *StatusError
	if errors.As(err, &status) && status.RetryAfter > 0 {
		delay = status.RetryAfter
		if delay > maxRetryAfter {
			delay = maxRetryAfter
		}
	}
	return delay
}

// retryable dice se err merita un altro tentativo.
//
// Il caso che ha allargato questa decisione oltre il solo StatusError:
// sul manuale reale del club, con le pagine in volo insieme, due su
// quattro sono morte con "context deadline exceeded" — il provider non
// aveva risposto affatto — e restavano due pagine perse in silenzio
// dall'indice, perché un errore di trasporto non è una risposta HTTP e
// non passava da Temporary().
//
// Un timeout si riprova: il provider non ha risposto, non ha risposto
// "no". Una connessione rifiutata no: è un host sbagliato o un servizio
// spento, e riprovarlo dà lo stesso esito — lo stesso argomento del 4xx.
func retryable(ctx context.Context, err error) bool {
	// Prima di tutto: se è il contesto del CHIAMANTE a essere finito, non
	// c'è niente da riprovare. È anche la distinzione che separa i due
	// "deadline exceeded" possibili — quello del singolo tentativo, che si
	// riprova, da quello di chi ha annullato tutto, che no.
	if ctx.Err() != nil {
		return false
	}

	var status *StatusError
	if errors.As(err, &status) {
		return status.Temporary()
	}

	// Il timeout del singolo tentativo arriva come *url.Error che avvolge
	// context.DeadlineExceeded. Il controllo su net.Error copre anche il
	// timeout imposto da http.Client.Timeout, che postRaw azzera ma che un
	// altro chiamante potrebbe lasciare in piedi.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// postChatRetrying riprova una richiesta di trascrizione sugli errori
// transitori del provider. È usata SOLO da
// Transcribe, e deliberatamente: è la sola chiamata che si fa N volte per
// un unico documento, quindi la sola in cui un 429 è un esito atteso
// invece di un'eccezione. Segment si chiama una volta per documento, e Ask
// sta su una rotta pubblica dove tre tentativi in serie sarebbero tre
// volte l'attesa di chi è al tavolo con la domanda in sospeso.
func (c *HTTPClient) postChatRetrying(ctx context.Context, payload []byte, timeout time.Duration) (string, error) {
	for attempt := 0; ; attempt++ {
		out, err := c.postChat(ctx, payload, timeout)
		if err == nil {
			return out, nil
		}

		if attempt >= len(transcribeBackoff) || !retryable(ctx, err) {
			return "", err
		}

		// Un'attesa non deve sopravvivere all'annullamento del contesto:
		// se l'admin ha chiuso la pagina, o se un'altra pagina ha già
		// scoperto che il modello vision non è configurato, dormire qui
		// terrebbe in vita una richiesta che non interessa più a nessuno.
		select {
		case <-time.After(retryDelay(attempt, err)):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
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
		return nil, &StatusError{
			Status:     resp.StatusCode,
			Body:       string(body),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	return body, nil
}

// StatusError è una risposta non-2xx del provider con lo status
// CONSERVATO. Prima era un fmt.Errorf e lo status restava solo dentro il
// messaggio: nessun chiamante poteva distinguere un guasto transitorio
// (429, 5xx) da uno definitivo (400, 401), e con la trascrizione delle
// pagine in parallelo quella distinzione è diventata necessaria — un 429
// da rate limit è un esito normale quando si mandano cinque pagine
// insieme, e senza riprovare diventa un buco silenzioso nell'indice.
//
// Il testo del messaggio è identico a quello di prima: finisce nel log
// "index: transcribe page N: ..." che l'admin legge, e cambiarlo avrebbe
// reso illeggibili i log già raccolti senza nessun guadagno.
type StatusError struct {
	Status int
	Body   string
	// RetryAfter è l'header omonimo tradotto in durata, 0 quando il
	// provider non lo manda (o manda qualcosa che non è un numero di
	// secondi).
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("ai provider returned status %d: %s", e.Status, e.Body)
}

// Temporary dice se ha senso riprovare la stessa richiesta. Un 429 è il
// rate limit del provider e passa; un 5xx è un suo guasto e di solito
// passa. Un 4xx no: una chiave sbagliata o una richiesta malformata danno
// lo stesso esito quante volte le si riprovi, e su un manuale di trenta
// pagine riprovarle triplica solo l'attesa prima dell'errore.
func (e *StatusError) Temporary() bool {
	return e.Status == http.StatusTooManyRequests || (e.Status >= 500 && e.Status <= 599)
}

// parseRetryAfter legge la forma a secondi dell'header Retry-After, la
// sola che i provider OpenAI-compatibili usino in pratica. La forma a data
// HTTP prevista dallo standard non è gestita: in sua assenza si ripiega
// sull'attesa di base, che è un esito corretto e non un guasto.
func parseRetryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
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

// SearchFunc cerca nelle fonti del gioco (manuali, FAQ) e restituisce il
// payload già formattato per il modello, come stringa. È una funzione e
// non un'interfaccia sui tipi di manuals: così questo pacchetto non
// conosce SQLite né manuals.SourceHit, e il loop si testa con una closure
// di due righe.
type SearchFunc func(ctx context.Context, keywords []string) (string, error)

// Asker è l'astrazione che serve all'handler pubblico. HTTPClient la
// implementa; i test iniettano un finto.
type Asker interface {
	Ask(ctx context.Context, req AskRequest) (string, error)
}

type AskRequest struct {
	GameName string
	Turns    []Turn
	// CorpusIndex è l'indice dei titoli di sezione delle fonti (serve
	// sempre quando c'è, ed è quel che evita al modello la chiamata
	// esplorativa al tool). CorpusChars non decide più niente qui: il tool
	// si dichiara sempre quando c'è Search, non esiste più una soglia
	// sotto la quale un corpus piccolo lo rende superfluo.
	CorpusChars int
	CorpusIndex string
	Search      SearchFunc
}

// SearchToolName è esportato perché l'handler pubblico (Task 7) deve
// riconoscere nella cronologia dei tool_calls rimandata indietro quale
// chiamata è la ricerca nelle fonti.
const SearchToolName = "cerca_nelle_fonti"

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

// Ask risponde a una domanda sulle regole di un gioco. Il tool di ricerca
// si dichiara SEMPRE quando c'è una funzione di ricerca (req.Search !=
// nil): non esiste più una soglia sotto la quale un corpus piccolo lo
// giudica superfluo — un manuale corto (il caso vero di questo progetto: 4
// pagine) deve restare interrogabile tanto quanto uno lungo. Il modello
// riceve anche l'indice delle fonti quando c'è, e il loop prosegue finché
// non risponde o non scatta MaxToolIterations.
func (c *HTTPClient) Ask(ctx context.Context, req AskRequest) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()

	// toolsDeclared è la sola condizione che decide se il tool compare
	// nella richiesta E se il prompt promette di poterlo usare: calcolata
	// una volta, usata da entrambi, così le due cose non possono
	// disallinearsi.
	toolsDeclared := req.Search != nil

	system, err := json.Marshal(chatMessage{Role: "system", Content: askSystemPrompt(req, toolsDeclared)})
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
				Name: SearchToolName,
				Description: "Cerca nelle fonti del gioco (manuali, FAQ). Passa in un'unica " +
					"chiamata tutte le varianti lessicali plausibili: la ricerca è " +
					"lessicale, quindi più varianti trovano più cose.",
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
			if call.Function.Name == SearchToolName && req.Search != nil {
				keywords := parseKeywords(call.Function.Arguments)
				out, err := req.Search(ctx, keywords)
				if err != nil {
					log.Printf("ask: manual search failed: %v", err)
					result = "La ricerca nelle fonti non è disponibile in questo momento."
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
			log.Printf("ask: il modello ha chiamato %s %d volte: forzo la risposta senza tool", SearchToolName, iteration+1)
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

// askSystemPrompt costruisce le istruzioni. Due regole contano più delle
// altre: non inventare quando le fonti non dicono niente, e citare
// reference/reference_detail esattamente come arrivano dal risultato del
// tool — è su quella stringa che il server costruisce il link della
// citazione (Task 7), e un riferimento alterato lo rompe.
//
// toolsDeclared governa se si promette lo strumento di ricerca: deve
// essere la stessa condizione che decide se il tool compare nella
// richiesta (vedi Ask), altrimenti il prompt può promettere uno strumento
// che il modello non ha davvero a disposizione.
func askSystemPrompt(req AskRequest, toolsDeclared bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sei l'assistente regole di %q per un'associazione di giochi da tavolo. ", req.GameName)
	b.WriteString("Chi ti scrive è in piedi a un tavolo, con le carte in mano: rispondi in italiano, breve, come si parla. ")
	b.WriteString("Rispondi SOLO con quello che c'è nelle fonti del gioco (manuali, FAQ). ")
	b.WriteString("Se le fonti non lo dicono, dillo chiaramente invece di dedurre: al tavolo una regola inventata fa danno. ")
	b.WriteString("Quando citi una fonte, riporta ESATTAMENTE i valori \"reference\" e \"reference_detail\" così come li hai ricevuti dal risultato della ricerca, uniti da una virgola (esempio: reference \"Regolamento base\" e reference_detail \"pagina 7\" diventano \"Regolamento base, pagina 7\"). ")
	b.WriteString("Non abbreviarli, non tradurli e non inventarli: è su quella stringa esatta che si costruisce il link alla fonte, e un riferimento alterato punta a un file sbagliato o a nessun file. ")
	b.WriteString("Non inventare nomi di carte, valori o numeri che non hai letto.\n\n")

	if req.CorpusIndex != "" {
		fmt.Fprintf(&b, "Indice delle fonti: %s\n\n", req.CorpusIndex)
	}
	if toolsDeclared {
		b.WriteString("Per leggere le fonti usa lo strumento di ricerca. ")
		b.WriteString("Se una ricerca non trova nulla, riprova con altre parole prima di dire che le fonti non lo dicono.")
	} else {
		// Non dovrebbe succedere nell'uso reale (il chiamante passa
		// sempre Search), ma se capitasse non si deve promettere uno
		// strumento che non è stato dichiarato: meglio dire al modello
		// di limitarsi all'indice piuttosto che fargli credere di poter
		// cercare quando non può.
		b.WriteString("Non hai a disposizione nessuno strumento di ricerca: rispondi solo se l'indice qui sopra basta, altrimenti di' che non puoi controllare le fonti in questo momento.")
	}
	return b.String()
}
