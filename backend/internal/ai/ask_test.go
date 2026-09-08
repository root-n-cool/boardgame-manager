package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// askServer è un provider finto scriptato: ogni richiesta consuma la
// prossima risposta della lista, e le richieste ricevute restano
// ispezionabili.
type askServer struct {
	t         *testing.T
	responses []string
	requests  []string
}

func (a *askServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		a.requests = append(a.requests, string(body))
		if len(a.responses) == 0 {
			a.t.Fatalf("il provider finto ha ricevuto %d richieste ma non ha più risposte", len(a.requests))
		}
		next := a.responses[0]
		a.responses = a.responses[1:]
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, next)
	}
}

const answerOnly = `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"La partita finisce subito. Regolamento base, pag. 7."}}]}`

func toolCallResponse(args string) string {
	return `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,` +
		`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"cerca_nel_manuale","arguments":` +
		strconv.Quote(args) + `}}]}}]}`
}

func TestAsk_CallsTheToolThenAnswers(t *testing.T) {
	srv := &askServer{t: t, responses: []string{
		toolCallResponse(`{"parole_chiave":["pila pesca","fine partita","pareggio"]}`),
		answerOnly,
	}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	var gotKeywords []string
	client := ai.NewHTTPClient(ts.URL, "sk-test", "deepseek-v4-flash")
	out, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "finite le carte che si fa?"}},
		CorpusChars: 90000, // sopra soglia: il tool serve
		CorpusIndex: "Fase di Upkeep p.4 · Fine partita p.7",
		Search: func(ctx context.Context, kw []string) (string, error) {
			gotKeywords = kw
			return "[1] Regolamento base (it), pag. 7  ·  trovato con: pila pesca\nLa partita termina...", nil
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !strings.Contains(out, "pag. 7") {
		t.Fatalf("risposta inattesa: %q", out)
	}
	if len(gotKeywords) != 3 || gotKeywords[0] != "pila pesca" {
		t.Fatalf("le parole chiave non sono arrivate alla ricerca: %v", gotKeywords)
	}
	if len(srv.requests) != 2 {
		t.Fatalf("attese 2 richieste al provider, fatte %d", len(srv.requests))
	}

	// Prima richiesta: il tool va dichiarato e l'indice va nel prompt.
	if !strings.Contains(srv.requests[0], "cerca_nel_manuale") {
		t.Fatalf("il tool non è dichiarato nella prima richiesta:\n%s", srv.requests[0])
	}
	if !strings.Contains(srv.requests[0], "Fine partita p.7") {
		t.Fatalf("l'indice del manuale non è nel prompt:\n%s", srv.requests[0])
	}
	// Seconda richiesta: deve contenere il messaggio assistant con i
	// tool_calls E il risultato con il suo tool_call_id, altrimenti il
	// provider rifiuta la conversazione.
	if !strings.Contains(srv.requests[1], `"tool_call_id":"call_1"`) {
		t.Fatalf("il risultato del tool non è legato alla chiamata:\n%s", srv.requests[1])
	}
	if !strings.Contains(srv.requests[1], "La partita termina") {
		t.Fatalf("il risultato della ricerca non è stato rimandato al modello:\n%s", srv.requests[1])
	}
}

func TestAsk_ShortManualGoesInlineWithNoTool(t *testing.T) {
	srv := &askServer{t: t, responses: []string{answerOnly}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	searched := false
	client := ai.NewHTTPClient(ts.URL, "sk-test", "deepseek-v4-flash")
	_, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "come finisce?"}},
		CorpusChars: 4000, // sotto InlineCorpusMaxChars
		CorpusText:  "=== Regolamento base (it) ===\n\n--- pag. 7 ---\nLa partita termina subito.",
		CorpusIndex: "Fine partita p.7",
		Search: func(ctx context.Context, kw []string) (string, error) {
			searched = true
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if searched {
		t.Fatal("con un manuale corto la ricerca non va nemmeno sfiorata")
	}
	if len(srv.requests) != 1 {
		t.Fatalf("attesa 1 sola richiesta, fatte %d", len(srv.requests))
	}
	if strings.Contains(srv.requests[0], "cerca_nel_manuale") {
		t.Fatalf("il tool NON va dichiarato quando il manuale è inline:\n%s", srv.requests[0])
	}
	if !strings.Contains(srv.requests[0], "La partita termina subito") {
		t.Fatalf("il testo del manuale non è nel prompt:\n%s", srv.requests[0])
	}
}

func TestAsk_SendsTheConversationHistory(t *testing.T) {
	srv := &askServer{t: t, responses: []string{answerOnly}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	client := ai.NewHTTPClient(ts.URL, "sk-test", "m")
	_, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		CorpusChars: 100,
		CorpusText:  "manuale",
		Turns: []ai.Turn{
			{Role: "user", Text: "come si contano i punti?"},
			{Role: "assistant", Text: "Ogni edificio vale i punti stampati."},
			{Role: "user", Text: "e se siamo pari?"},
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	for _, want := range []string{"come si contano i punti?", "Ogni edificio vale", "e se siamo pari?"} {
		if !strings.Contains(srv.requests[0], want) {
			t.Fatalf("lo storico non è arrivato al provider, manca %q:\n%s", want, srv.requests[0])
		}
	}
}

func TestAsk_StopsAStuckModelAndStillAnswers(t *testing.T) {
	// Un modello che chiama il tool all'infinito: la guardia deve fermarlo
	// e forzare una risposta togliendo il tool, non restituire un errore.
	// Non è un limite sull'utente, è protezione da un bug del modello.
	responses := []string{}
	for i := 0; i < 5; i++ {
		responses = append(responses, toolCallResponse(`{"parole_chiave":["x"]}`))
	}
	responses = append(responses, answerOnly)

	srv := &askServer{t: t, responses: responses}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	calls := 0
	client := ai.NewHTTPClient(ts.URL, "sk-test", "m")
	out, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "?"}},
		CorpusChars: 90000,
		Search: func(ctx context.Context, kw []string) (string, error) {
			calls++
			return "niente", nil
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if out == "" {
		t.Fatal("la guardia deve produrre una risposta, non il vuoto")
	}
	if calls != ai.MaxToolIterations {
		t.Fatalf("il tool doveva essere chiamato %d volte, chiamato %d", ai.MaxToolIterations, calls)
	}
	// L'ultima richiesta è quella senza tool: è così che si forza la
	// risposta invece di lasciare il modello a girare.
	//
	// Non si può cercare la sottostringa "cerca_nel_manuale" nell'intera
	// richiesta: lo storico dei messaggi contiene le tool_calls
	// precedenti, che DEVONO riportare quel nome verbatim (è così che il
	// provider le riconosce), quindi comparirebbe comunque anche quando i
	// tool non sono più dichiarati. Il segnale che discrimina davvero è la
	// chiave JSON "tools", assente (omitempty) solo quando non si
	// dichiara nessun tool in questa richiesta.
	last := srv.requests[len(srv.requests)-1]
	if strings.Contains(last, `"tools":`) {
		t.Fatalf("l'ultima richiesta doveva essere senza tool:\n%s", last)
	}
}

func TestAsk_AcceptsKeywordsSentAsAString(t *testing.T) {
	// I modelli economici a volte mandano una stringa dove lo schema dice
	// array. Rifiutarla significa perdere la domanda per un dettaglio di
	// serializzazione, quindi si accetta e si divide.
	srv := &askServer{t: t, responses: []string{
		toolCallResponse(`{"parole_chiave":"pareggio, stesso punteggio"}`),
		answerOnly,
	}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	var got []string
	client := ai.NewHTTPClient(ts.URL, "sk-test", "m")
	if _, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "?"}},
		CorpusChars: 90000,
		Search: func(ctx context.Context, kw []string) (string, error) {
			got = kw
			return "ok", nil
		},
	}); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if len(got) != 2 || got[0] != "pareggio" || got[1] != "stesso punteggio" {
		t.Fatalf("una stringa di parole chiave va divisa in due, ottenuto %v", got)
	}
}

func TestAsk_WithoutAProviderIsNotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	_, err := client.Ask(context.Background(), ai.AskRequest{GameName: "Wingspan"})
	if !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}
