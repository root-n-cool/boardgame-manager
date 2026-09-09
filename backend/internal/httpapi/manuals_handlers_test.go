package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/manuals"
	"boardgames-manager/internal/storage"
)

// fakeSegmenter sta al posto del provider AI per Segment: cattura l'ultimo
// testo ricevuto (per verificare che venga chiamato davvero, o non
// chiamato affatto), e per difetto restituisce il testo invariato (nessun
// titolo aggiunto), che è già una segmentazione valida secondo il
// contratto di Segment ("se il brano non ha sezioni distinte,
// restituiscilo invariato").
type fakeSegmenter struct {
	calls  int
	lastIn string
	out    string
	err    error
}

func (f *fakeSegmenter) Segment(ctx context.Context, text string) (string, error) {
	f.calls++
	f.lastIn = text
	if f.err != nil {
		return "", f.err
	}
	if f.out != "" {
		return f.out, nil
	}
	return text, nil
}

// pageTranscriber trascrive ogni pagina restituendo esattamente il
// markdown che il test le ha assegnato: serve al test che conta più di
// tutti (il confine di pagina), dove serve controllare parola per parola
// cosa "vede" ciascuna pagina per costruire una sezione che ne attraversa
// due.
type pageTranscriber struct {
	byPage map[int]string
	// calls è atomico perché il percorso vision trascrive le pagine in
	// parallelo (vedi transcribeConcurrency): un int normale qui sarebbe
	// una corsa segnalata da -race.
	calls atomic.Int64
}

func (p *pageTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	p.calls.Add(1)
	return p.byPage[page], nil
}

// fakeSuggester sta al posto del provider per SuggestQuestions: conta le
// chiamate (serve al test che verifica che NON venga chiamato) e cattura i
// titoli ricevuti.
type fakeSuggester struct {
	calls       atomic.Int64
	lastGame    string
	lastHeading []string
	out         []string
	err         error
}

func (f *fakeSuggester) SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error) {
	f.calls.Add(1)
	f.lastGame = gameName
	f.lastHeading = headings
	if f.err != nil {
		return nil, f.err
	}
	if f.out != nil {
		return f.out, nil
	}
	return []string{"Generata 1?", "Generata 2?", "Generata 3?"}, nil
}

// loginAsAdmin esegue il bootstrap del primo admin e restituisce il
// cookie di sessione: gli handler di questo file sono tutti protetti.
func loginAsAdmin(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	return bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
}

// seedGameWithFile crea un gioco con lingua "it" e un media "file" salvato
// su disco con filename (decide l'estensione, quindi il formato su cui
// l'ingestione instrada): passa direttamente per gli store, non per
// l'HTTP, così un test come TestIndexMedia_RequiresAuth può preparare il
// fixture senza nessuna sessione admin. title, se non vuoto, diventa
// game_media.title (la Reference di cui si parla nel piano); vuoto lascia
// il fallback del titolo di default.
func seedGameWithFile(t *testing.T, server *httpapi.Server, content []byte, filename, title string) (gameID, mediaID int64) {
	t.Helper()
	ctx := context.Background()

	game, err := server.Games.CreateGame(ctx, games.Game{Name: "Gioco di Prova"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	lang, err := server.Games.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "it", IsBaseLanguage: true, Name: game.Name,
	})
	if err != nil {
		t.Fatalf("create language: %v", err)
	}
	path, err := server.Storage.Save(storage.ManualCategory, bytes.NewReader(content), filename)
	if err != nil {
		t.Fatalf("save file: %v", err)
	}
	var titlePtr *string
	if title != "" {
		titlePtr = &title
	} else {
		defaultTitle := "Regolamento"
		titlePtr = &defaultTitle
	}
	media, err := server.Games.CreateMedia(ctx, games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeFile, URLOrPath: path, Title: titlePtr,
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}
	return game.ID, media.ID
}

func indexPath(gameID, mediaID int64) string {
	return fmt.Sprintf("/api/games/%d/languages/it/media/%d/index", gameID, mediaID)
}

func postIndex(cookie *http.Cookie, router http.Handler, gameID, mediaID int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, indexPath(gameID, mediaID), nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func deleteIndex(cookie *http.Cookie, router http.Handler, gameID, mediaID int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, indexPath(gameID, mediaID), nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestIndexMedia_RequiresAuth verifica che entrambe le rotte siano dietro
// il middleware di autenticazione: un'attenzione dalla fase precedente
// (vedi il brief) è che un test così può passare per il motivo sbagliato
// se una rotta risponde 400 prima di arrivare all'auth. Qui il gameID e il
// mediaID passati sono VALIDI (seedGameWithFile li ha davvero creati),
// quindi un'eventuale validazione dei parametri di rotta non può essere
// lei a produrre lo status: se il 401 arriva lo stesso, è il middleware,
// non un incidente di validazione.
func TestIndexMedia_RequiresAuth(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	router := httpapi.NewRouter(server)
	gameID, mediaID := seedGameWithFile(t, server, []byte("# Titolo\n\nTesto."), "regole.md", "")

	if rec := postIndex(nil, router, gameID, mediaID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST index senza sessione: atteso 401, ottenuto %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := deleteIndex(nil, router, gameID, mediaID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("DELETE index senza sessione: atteso 401, ottenuto %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestIndexMedia_WithoutAProviderIs404 è il gate del Task 5: senza un
// provider di testo configurato la rotta si comporta come inesistente,
// esattamente come askHandler senza Asker.
func TestIndexMedia_WithoutAProviderIs404(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, []byte("# Titolo\n\nTesto."), "regole.md", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("atteso 404 senza provider, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

// TestIndexMedia_MarkdownDoesNotCallVisionOrSegmentation copre il ramo più
// semplice del routing: un .md è già markdown, quindi non deve passare né
// da Segment né da Transcribe. I due finti qui sopra, se chiamati,
// falliscono il test da soli: err non-nil li farebbe fallire la richiesta,
// ma qui basta contare le chiamate per essere sicuri che il ramo giusto sia
// stato preso, non solo che non ci sia stato un errore.
func TestIndexMedia_MarkdownDoesNotCallVisionOrSegmentation(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	seg := &fakeSegmenter{}
	vis := &pageTranscriber{}
	server.Segmenter = seg
	server.Vision = vis
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	md := "## Preparazione\n\nOgni giocatore pesca cinque carte.\n\n## Fine partita\n\nSi vince con più punti."
	gameID, mediaID := seedGameWithFile(t, server, []byte(md), "regole.md", "Regolamento base")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if seg.calls != 0 {
		t.Fatalf("un .md non deve chiamare Segment, chiamato %d volte", seg.calls)
	}
	if vis.calls.Load() != 0 {
		t.Fatalf("un .md non deve chiamare Transcribe, chiamato %d volte", vis.calls.Load())
	}

	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"pesca"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("il .md doveva essere indicizzato: la ricerca non trova nulla")
	}
	if hits[0].ReferenceDetail != `sezione «Preparazione»` {
		t.Fatalf("reference_detail per un formato senza pagine: atteso 'sezione «Preparazione»', ottenuto %q", hits[0].ReferenceDetail)
	}
	if hits[0].Reference != "Regolamento base" {
		t.Fatalf("reference doveva venire dal titolo del media, ottenuto %q", hits[0].Reference)
	}
}

// TestIndexMedia_TxtCallsSegmentation è il caso simmetrico: un .txt è
// testo piatto senza titoli, quindi DEVE passare da Segment perché
// ParseSections possa trovarci delle sezioni.
func TestIndexMedia_TxtCallsSegmentation(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	seg := &fakeSegmenter{out: "## Regole\n\nOgni giocatore pesca due carte all'inizio del turno."}
	server.Segmenter = seg
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	raw := "Ogni giocatore pesca due carte all'inizio del turno."
	gameID, mediaID := seedGameWithFile(t, server, []byte(raw), "regole.txt", "Regole scritte a mano")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if seg.calls != 1 {
		t.Fatalf("un .txt deve chiamare Segment esattamente una volta, chiamato %d volte", seg.calls)
	}
	if seg.lastIn != raw {
		t.Fatalf("Segment doveva ricevere il testo grezzo del file, ha ricevuto %q", seg.lastIn)
	}

	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"pesca"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("il .txt doveva essere indicizzato dopo la segmentazione")
	}
}

// TestIndexMedia_DocxIsAccepted copre il quarto formato: un .docx passa da
// DocxToMarkdown, non da Segment né da Transcribe (esattamente come un
// .md, una volta convertito).
func TestIndexMedia_DocxIsAccepted(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	seg := &fakeSegmenter{}
	server.Segmenter = seg
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	docx := manuals.NewDocx([]manuals.DocxParagraph{
		{Style: "Heading1", Text: "Preparazione"},
		{Style: "", Text: "Si mescolano le carte e si distribuiscono a testa in giù."},
	})
	gameID, mediaID := seedGameWithFile(t, server, docx, "Regolamento.docx", "Regolamento Word")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if seg.calls != 0 {
		t.Fatalf("un .docx non deve chiamare Segment, chiamato %d volte", seg.calls)
	}

	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"mescolano"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("il .docx doveva essere indicizzato")
	}
	if hits[0].ReferenceDetail != `sezione «Preparazione»` {
		t.Fatalf("reference_detail: atteso 'sezione «Preparazione»', ottenuto %q", hits[0].ReferenceDetail)
	}
}

// TestIndexMedia_DocxWithoutTextIsAClearError copre il messaggio dedicato
// del brief: un .docx senza testo (una tabella vuota, un documento senza
// paragrafi) deve dirlo, non fingere un guasto generico.
func TestIndexMedia_DocxWithoutTextIsAClearError(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	empty := manuals.NewDocxRawBody("") // nessun paragrafo: DocxToMarkdown non produce testo
	gameID, mediaID := seedGameWithFile(t, server, empty, "vuoto.docx", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "non contiene testo") {
		t.Fatalf("il messaggio deve dire che il file non ha testo, non un guasto generico: %s", rec.Body.String())
	}
}

// TestIndexMedia_ScannedPDFWithoutVisionModelNamesTheSetting copre il
// primo messaggio della tabella: un PDF scansionato senza modello vision
// configurato deve nominare il campo delle impostazioni, non dare un
// errore generico.
func TestIndexMedia_ScannedPDFWithoutVisionModelNamesTheSetting(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{} // il provider di testo è configurato...
	server.Vision = &erroringTranscriber{err: ai.ErrNotConfigured} // ...ma non quello vision
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, manuals.NewScannedPDF(), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Modello per i manuali scansionati") {
		t.Fatalf("il messaggio deve nominare il campo delle impostazioni: %s", rec.Body.String())
	}
}

type erroringTranscriber struct{ err error }

func (e *erroringTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	return "", e.err
}

// partialFailTranscriber fallisce con un errore le pagine elencate in
// failPages e trascrive normalmente tutte le altre secondo byPage: serve al
// test del successo parziale (difetto 2), dove alcune pagine devono
// fallire per un errore del provider e altre riuscire — cosa che nessun
// finto qui sopra permette da solo (erroringTranscriber fallisce sempre,
// pageTranscriber non fallisce mai).
type partialFailTranscriber struct {
	byPage    map[int]string
	failPages map[int]bool
	calls     atomic.Int64 // atomico: vedi pageTranscriber.calls
}

func (p *partialFailTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	p.calls.Add(1)
	if p.failPages[page] {
		return "", fmt.Errorf("ai provider returned status 500: pagina %d", page)
	}
	return p.byPage[page], nil
}

// TestIndexMedia_PDFWithNeitherTextNorImagesSuggestsConverting copre il
// secondo messaggio: un PDF che non è né testo né immagini leggibili deve
// suggerire di convertire il file.
func TestIndexMedia_PDFWithNeitherTextNorImagesSuggestsConverting(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	// Un PDF valido (Catalog + Pages + una pagina), ma senza nessun testo
	// né nessuna immagine DCTDecode: né il percorso testo né quello vision
	// hanno niente da offrire.
	empty := manuals.BuildTestPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	})
	gameID, mediaID := seedGameWithFile(t, server, empty, "vuoto.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "convertirlo") {
		t.Fatalf("il messaggio deve suggerire di convertire il file: %s", rec.Body.String())
	}
}

// TestIndexMedia_ScannedPDFAllPagesFailingTranscriptionBlamesTheProvider è
// il difetto 1 dei log di produzione: un modello vision che il provider
// rifiuta per OGNI pagina (status 500 su ognuna) deve dire che è stata la
// lettura a non riuscire per un errore del modello, invitare a controllare
// il modello configurato per i manuali scansionati e a riprovare — SENZA
// suggerire di convertire il file, perché il documento non è affatto
// sospetto qui. Va confrontato con
// TestIndexMedia_ScannedPDFAllPagesEmptyTextSuggestsConverting: stesso
// esito HTTP (422), fixture diversa (errore contro stringa vuota),
// messaggio diverso — è la distinzione a essere la proprietà nuova, non i
// singoli casi presi da soli.
func TestIndexMedia_ScannedPDFAllPagesFailingTranscriptionBlamesTheProvider(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{} // il provider di testo è configurato: gate superato
	server.Vision = &erroringTranscriber{err: fmt.Errorf("ai provider returned status 500: {\"error\":\"model not found\"}")}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, manuals.NewScannedPDFPages(3), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "modello configurato per i manuali scansionati") {
		t.Fatalf("il messaggio deve invitare a controllare il modello configurato per i manuali scansionati: %s", body)
	}
	if !strings.Contains(body, "errore su ogni pagina") {
		t.Fatalf("il messaggio deve dire che il provider ha risposto con un errore su ogni pagina: %s", body)
	}
	if !strings.Contains(body, "riprova") {
		t.Fatalf("il messaggio deve invitare a riprovare: %s", body)
	}
	if strings.Contains(body, "convertirlo") {
		t.Fatalf("un guasto del provider non deve suggerire di convertire un file sano: %s", body)
	}
}

// TestIndexMedia_ScannedPDFAllPagesEmptyTextSuggestsConverting è il caso
// gemello: ogni pagina viene letta SENZA nessun errore ma non contiene
// testo (solo illustrazioni). Qui il documento È il sospetto, quindi il
// messaggio resta quello attuale — suggerire di convertire il file — e
// deve restare DIVERSO da quello del test gemello sopra.
func TestIndexMedia_ScannedPDFAllPagesEmptyTextSuggestsConverting(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	server.Vision = &pageTranscriber{} // byPage nil: restituisce "" per ogni pagina, senza nessun errore
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, manuals.NewScannedPDFPages(3), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "convertirlo") {
		t.Fatalf("pagine lette ma senza testo: il messaggio deve suggerire di convertire il file: %s", body)
	}
	if strings.Contains(body, "errore su ogni pagina") {
		t.Fatalf("nessun errore di provider è avvenuto qui: il messaggio non deve parlarne: %s", body)
	}
	if strings.Contains(body, "modello configurato per i manuali scansionati") {
		t.Fatalf("questo non è un guasto del provider: non deve invitare a controllare il modello: %s", body)
	}
}

// TestIndexMedia_ScannedPDFPartialTranscriptionFailureReportsPageCounts è
// il difetto 2: se alcune pagine falliscono la trascrizione ma le altre
// bastano a produrre almeno un chunk, l'indicizzazione deve restare un
// successo (2xx: un manuale a cui manca una pagina è comunque meglio di
// nessun manuale) MA la risposta deve dire quante pagine sono state
// indicizzate e quante saltate, così il pannello admin può segnalarlo
// invece di far credere a un manuale completo quando non lo è.
func TestIndexMedia_ScannedPDFPartialTranscriptionFailureReportsPageCounts(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	server.Vision = &partialFailTranscriber{
		byPage: map[int]string{
			1: "## Introduzione\n\nRegole di base per iniziare a giocare, testo pagina uno.",
			3: "## Turno\n\nDurante il turno si pesca una carta, testo pagina tre.",
			5: "## Fine\n\nLa partita finisce quando il mazzo termina, testo pagina cinque.",
		},
		failPages: map[int]bool{2: true, 4: true},
	}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, manuals.NewScannedPDFPages(5), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("un successo parziale resta un successo: atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non è JSON valido: %v (%s)", err, rec.Body.String())
	}
	indexed, ok := resp["pagesIndexed"].(float64)
	if !ok {
		t.Fatalf("la risposta deve avere pagesIndexed quando delle pagine sono state saltate: %s", rec.Body.String())
	}
	skipped, ok := resp["pagesSkipped"].(float64)
	if !ok {
		t.Fatalf("la risposta deve avere pagesSkipped quando delle pagine sono state saltate: %s", rec.Body.String())
	}
	if indexed != 3 {
		t.Fatalf("pagesIndexed: atteso 3 (pagine 1, 3, 5), ottenuto %v", indexed)
	}
	if skipped != 2 {
		t.Fatalf("pagesSkipped: atteso 2 (pagine 2 e 4 fallite), ottenuto %v", skipped)
	}

	// Le tre pagine riuscite devono comunque essere state indicizzate
	// davvero, non solo contate.
	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"pesca"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("il testo delle pagine riuscite doveva essere indicizzato")
	}
}

// TestIndexMedia_SegmentationRejectedIsAClearError copre il quarto
// messaggio: la segmentazione rifiutata (il modello ha riscritto invece di
// segmentare) deve dire che la lettura non è affidabile e invitare a
// riprovare, non un errore generico.
func TestIndexMedia_SegmentationRejectedIsAClearError(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{err: ai.ErrSegmentationRejected}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, []byte("Testo qualsiasi da segmentare."), "regole.txt", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "non è affidabile") || !strings.Contains(body, "iprova") {
		t.Fatalf("il messaggio deve dire che la lettura non è affidabile e invitare a riprovare: %s", body)
	}
}

// TestIndexMedia_DeleteRemovesTheChunks copre il DELETE.
func TestIndexMedia_DeleteRemovesTheChunks(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	md := "## Preparazione\n\nOgni giocatore pesca cinque carte."
	gameID, mediaID := seedGameWithFile(t, server, []byte(md), "regole.md", "")

	if rec := postIndex(cookie, router, gameID, mediaID); rec.Code != http.StatusOK {
		t.Fatalf("index: atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	has, err := server.Manuals.HasChunks(context.Background(), gameID)
	if err != nil || !has {
		t.Fatalf("il gioco doveva avere chunk dopo l'indicizzazione: has=%v err=%v", has, err)
	}

	rec := deleteIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("atteso 204, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	has, err = server.Manuals.HasChunks(context.Background(), gameID)
	if err != nil {
		t.Fatalf("has chunks: %v", err)
	}
	if has {
		t.Fatal("dopo il DELETE non devono restare chunk")
	}
}

// TestIndexMedia_ReindexingTheSameMediaDoesNotDisambiguateAgainstItself è
// il caso citato nel piano come motivo per cui Summary va letto DOPO aver
// cancellato le vecchie fonti di QUESTO stesso media: senza quell'ordine,
// re-indicizzare lo stesso file con lo stesso titolo si scontrerebbe con
// la propria vecchia voce e guadagnerebbe un suffisso di lingua alla
// seconda esecuzione, anche senza nessun secondo media coinvolto.
func TestIndexMedia_ReindexingTheSameMediaDoesNotDisambiguateAgainstItself(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	md := "## Preparazione\n\nOgni giocatore pesca cinque carte."
	gameID, mediaID := seedGameWithFile(t, server, []byte(md), "regole.md", "Regolamento base")

	for i := 0; i < 2; i++ {
		rec := postIndex(cookie, router, gameID, mediaID)
		if rec.Code != http.StatusOK {
			t.Fatalf("giro %d: atteso 200, ottenuto %d: %s", i, rec.Code, rec.Body.String())
		}
	}

	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"pesca"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("nessun risultato dopo la doppia indicizzazione")
	}
	if hits[0].Reference != "Regolamento base" {
		t.Fatalf("re-indicizzare lo stesso media non deve aggiungere un suffisso: reference = %q", hits[0].Reference)
	}
}

// TestIndexMedia_DisambiguatesACollidingReference è il cuore della
// disambiguazione descritta nel piano: due media DISTINTI dello stesso
// gioco con lo stesso titolo devono finire con reference diverse, o la
// mappa reference → percorso file (Task 7) punterebbe al file sbagliato
// per uno dei due.
func TestIndexMedia_DisambiguatesACollidingReference(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	md1 := "## Preparazione\n\nOgni giocatore pesca cinque carte."
	gameID, media1 := seedGameWithFile(t, server, []byte(md1), "regole.md", "Regolamento")

	// Un secondo media, STESSO gioco, STESSO titolo: creato direttamente
	// nella stessa lingua "it" (game_language_id), per collidere davvero
	// con il primo.
	var langID int64
	if err := conn.QueryRow(`SELECT id FROM game_languages WHERE game_id = ?`, gameID).Scan(&langID); err != nil {
		t.Fatalf("lang id: %v", err)
	}
	path2, err := server.Storage.Save(storage.ManualCategory,
		bytes.NewReader([]byte("## Fine partita\n\nSi vince con più punti.")), "regole2.md")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	title := "Regolamento"
	media2, err := server.Games.CreateMedia(context.Background(), games.GameMedia{
		GameLanguageID: langID, Type: games.MediaTypeFile, URLOrPath: path2, Title: &title,
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}

	if rec := postIndex(cookie, router, gameID, media1); rec.Code != http.StatusOK {
		t.Fatalf("index media1: atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if rec := postIndex(cookie, router, gameID, media2.ID); rec.Code != http.StatusOK {
		t.Fatalf("index media2: atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	hits1, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"pesca"})
	if err != nil || len(hits1) == 0 {
		t.Fatalf("search media1: hits=%d err=%v", len(hits1), err)
	}
	hits2, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"vince"})
	if err != nil || len(hits2) == 0 {
		t.Fatalf("search media2: hits=%d err=%v", len(hits2), err)
	}
	if hits1[0].Reference == hits2[0].Reference {
		t.Fatalf("due media con lo stesso titolo devono avere reference diverse, entrambe %q", hits1[0].Reference)
	}
	// Il primo mantiene il titolo puro; il secondo, arrivato dopo e in
	// collisione, guadagna il suffisso di lingua.
	if hits1[0].Reference != "Regolamento" {
		t.Fatalf("il primo media non doveva cambiare reference: %q", hits1[0].Reference)
	}
	if hits2[0].Reference != "Regolamento (it)" {
		t.Fatalf("il secondo media doveva disambiguarsi con la lingua: %q", hits2[0].Reference)
	}
}

// TestIndexMedia_ChunkThatBeginsOnTheSecondPageGetsThatPage è il test che
// conta più di tutti (vedi il piano): un manuale scansionato di tre
// pagine, con una sezione ("Fase di Upkeep") che comincia a pagina 1 e
// continua a pagina 2 SENZA un titolo nuovo. Il paragrafo di pagina 1 è
// tenuto apposta sotto MaxChunkChars ma abbastanza lungo, e SENZA nessuna
// punteggiatura di fine frase (solo virgole) negli ultimi 100 caratteri:
// questo impedisce a splitToSize di portare una coda di sovrapposizione
// da pagina 1 dentro il chunk successivo (vedi tailFrom in chunk.go), che
// altrimenti farebbe iniziare quel chunk ancora dentro pagina 1. Così il
// chunk con il testo di pagina 2 comincia ESATTAMENTE all'offset di
// pagina 2, non prima.
func TestIndexMedia_ChunkThatBeginsOnTheSecondPageGetsThatPage(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{} // gate: il provider di testo è "configurato"

	// ~960 caratteri, tutti separati da virgole: nessun '.', '!', '?' o ':'
	// seguito da spazio negli ultimi 100 caratteri (né altrove).
	clause := "ogni giocatore paga una moneta per ciascun edificio che possiede, in ordine di turno, senza fermarsi mai, "
	page1Body := strings.Repeat(clause, 10)
	page1Body = strings.TrimSpace(page1Body[:960])
	page1 := "## Fase di Upkeep\n\n" + page1Body

	page2 := "Se un giocatore non può pagare, scarta l'edificio invece di pagarlo. Poi si passa alla fase successiva."
	page3 := "## Fine partita\n\nLa partita termina quando la pila di pesca si esaurisce."

	server.Vision = &pageTranscriber{byPage: map[int]string{1: page1, 2: page2, 3: page3}}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	gameID, mediaID := seedGameWithFile(t, server, manuals.NewScannedPDFPages(3), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"scarta"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	found := false
	for _, h := range hits {
		if strings.Contains(h.Text, "scarta l'edificio") {
			found = true
			if h.ReferenceDetail != "pagina 2" {
				t.Fatalf("il chunk che comincia a pagina 2 doveva portare \"pagina 2\", ha %q (testo: %q)",
					h.ReferenceDetail, h.Text)
			}
		}
	}
	if !found {
		t.Fatalf("nessun chunk contiene il testo di pagina 2: %+v", hits)
	}

	// La pagina 1 deve restare pagina 1: non è che tutto sia franato su
	// pagina 2 per un offset scambiato.
	// L'indice FTS5 copre solo il testo del chunk, non il titolo (vedi il
	// trigger in 0015_game_sources.sql): si cerca una parola del CORPO di
	// pagina 1 ("moneta"), non "Upkeep" che vive solo nell'heading.
	hitsPage1, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"moneta"})
	if err != nil {
		t.Fatalf("search moneta: %v", err)
	}
	foundPage1 := false
	for _, h := range hitsPage1 {
		if strings.Contains(h.Text, "paga una moneta") {
			foundPage1 = true
			if h.ReferenceDetail != "pagina 1" {
				t.Fatalf("il chunk che comincia a pagina 1 doveva portare \"pagina 1\", ha %q", h.ReferenceDetail)
			}
		}
	}
	if !foundPage1 {
		t.Fatalf("nessun chunk contiene il testo di pagina 1: %+v", hitsPage1)
	}

	// Pagina 3, con il proprio titolo, resta pagina 3.
	hitsPage3, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"esaurisce"})
	if err != nil {
		t.Fatalf("search fine partita: %v", err)
	}
	if len(hitsPage3) == 0 || hitsPage3[0].ReferenceDetail != "pagina 3" {
		t.Fatalf("il chunk di pagina 3 doveva portare \"pagina 3\": %+v", hitsPage3)
	}
}

// TestIndexMedia_TextLayerPDFCallsSegmentation copre il percorso testo di
// un PDF (layer testo usabile): deve chiamare Segment sul testo estratto,
// non Transcribe.
func TestIndexMedia_TextLayerPDFCallsSegmentation(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	seg := &fakeSegmenter{out: "## Regole\n\nQuesto testo ha un vero layer testo nel PDF di prova."}
	vis := &pageTranscriber{}
	server.Segmenter = seg
	server.Vision = vis
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	gameID, mediaID := seedGameWithFile(t, server, manuals.NewTextPDF(), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if seg.calls != 1 {
		t.Fatalf("un PDF con layer testo deve chiamare Segment esattamente una volta, chiamato %d volte", seg.calls)
	}
	if vis.calls.Load() != 0 {
		t.Fatalf("un PDF con layer testo non deve chiamare Transcribe, chiamato %d volte", vis.calls.Load())
	}

	// "Regole" vive solo nel titolo che il segmentatore ha aggiunto: l'FTS5
	// indicizza solo il testo del chunk (vedi il trigger in
	// 0015_game_sources.sql), quindi si cerca una parola del corpo.
	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"prova"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 || hits[0].ReferenceDetail != "pagina 1" {
		t.Fatalf("atteso un chunk con reference_detail \"pagina 1\", ottenuto %+v", hits)
	}
}

// newTwoPageTextPDF costruisce un PDF di due pagine con un VERO layer
// testo (stesso schema di manuals.NewTextPDF: un font standard non
// embeddato, un operatore Tj per pagina), dove il testo di ciascuna pagina
// è esattamente page1/page2 — incluso un "\n\n" letterale se lo si vuole
// dentro il corpo, perché un byte di newline dentro una stringa PDF fra
// parentesi arriva verbatim nel testo estratto (verificato empiricamente:
// senza, ExtractText concatena il contenuto di Tj separati senza nessun
// separatore, nemmeno uno spazio). Non tocca testpdf.go (di cui questo
// task non ha la proprietà in questo giro di fix): usa solo
// manuals.BuildTestPDF, già esportato.
func newTwoPageTextPDF(page1, page2 string) []byte {
	contentFor := func(text string) string {
		return fmt.Sprintf("BT /F1 12 Tf 20 240 Td (%s) Tj ET", text)
	}
	streamObj := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	return manuals.BuildTestPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",                     // 1
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",       // 2
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " + // 3
			"/Resources << /Font << /F1 7 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " + // 4
			"/Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R >>",
		streamObj(contentFor(page1)),                             // 5
		streamObj(contentFor(page2)),                             // 6
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", // 7
	})
}

// TestIndexMedia_TextLayerPDFChunkThatBeginsOnSecondPageGetsThatPage è il
// finding 1 del primo giro di review: il percorso PDF-con-layer-testo
// (pdfTextChunks/pageStartsInSegmented) condivide con quello vision solo
// pageForOffset, non il calcolo degli starts — e non aveva NESSUN test con
// più di una pagina, quindi il ramo i>0 di pageStartsInSegmented (la vera
// ricerca dell'ancora) non veniva mai eseguito da nessun test. Qui uso un
// segmentatore finto a IDENTITÀ (fakeSegmenter senza .out: restituisce il
// testo invariato) così il testo "segmentato" è byte-per-byte lo stesso
// testo unito che pageStartsInSegmented deve ritrovare per pagina — il
// titolo "## Fase di Upkeep" è già nel PDF vero (disegnato via Tj), non
// aggiunto da un finto Segment.
//
// Stessa fixture (a parte il canale: qui è un vero layer testo via
// ExtractText, non vision) del test gemello sul percorso vision: un
// paragrafo di pagina 1 lungo (~960 caratteri, solo virgole, nessuna fine
// di frase negli ultimi 100 caratteri) forza splitToSize a NON portare una
// coda di sovrapposizione nel chunk di pagina 2, che quindi comincia
// esattamente all'offset di pagina 2.
func TestIndexMedia_TextLayerPDFChunkThatBeginsOnSecondPageGetsThatPage(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{} // identità: nessun titolo aggiunto, il testo passa invariato
	server.Vision = &pageTranscriber{}  // se mai chiamato, il test lo scopre sotto

	clause := "ogni giocatore paga una moneta per ciascun edificio che possiede, in ordine di turno, senza fermarsi mai, "
	page1Body := strings.Repeat(clause, 10)
	page1Body = strings.TrimSpace(page1Body[:960])
	page1 := "## Fase di Upkeep\n\n" + page1Body
	page2 := "Se un giocatore non puo pagare, scarta l'edificio invece di pagarlo. Poi si passa alla fase successiva."

	raw := newTwoPageTextPDF(page1, page2)
	if !manuals.HasTextLayer(raw) {
		t.Fatal("il fixture deve avere un vero layer testo, altrimenti il test esercita vision, non il percorso testo")
	}

	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, raw, "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if v, ok := server.Vision.(*pageTranscriber); ok && v.calls.Load() != 0 {
		t.Fatalf("un PDF con layer testo usabile non deve chiamare Transcribe, chiamato %d volte", v.calls.Load())
	}

	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"scarta"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	found := false
	for _, h := range hits {
		if strings.Contains(h.Text, "scarta l'edificio") {
			found = true
			if h.ReferenceDetail != "pagina 2" {
				t.Fatalf("il chunk che comincia a pagina 2 doveva portare \"pagina 2\", ha %q (testo: %q)",
					h.ReferenceDetail, h.Text)
			}
		}
	}
	if !found {
		t.Fatalf("nessun chunk contiene il testo di pagina 2: %+v", hits)
	}

	hitsPage1, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"moneta"})
	if err != nil {
		t.Fatalf("search moneta: %v", err)
	}
	if len(hitsPage1) == 0 || hitsPage1[0].ReferenceDetail != "pagina 1" {
		t.Fatalf("il chunk che comincia a pagina 1 doveva portare \"pagina 1\": %+v", hitsPage1)
	}
}

// TestIndexMedia_TextLayerPDFAnchorFailureFallsBackToThePreviousPage è la
// seconda parte del finding 1: pageStartsInSegmented dichiara, quando
// l'ancora di una pagina non si ritrova, un fallback silenzioso — "quella
// pagina eredita l'offset della precedente". Un fallback non provato è una
// supposizione. Qui si forza il fallimento facendo restituire al
// segmentatore finto un testo TOTALMENTE estraneo (che non contiene
// nessuna delle due pagine originali): l'ancora di pagina 2 (i primi 40
// caratteri del suo testo VERO) non può comparire in un testo che non
// condivide una sola parola con l'originale, quindi pageStartsInSegmented
// deve ripiegare su starts[1] = starts[0].
//
// L'asserzione verifica che il ripiego sia DAVVERO quello dichiarato: ogni
// chunk del testo sostituito (compreso quello che, per contenuto,
// "sembrerebbe" appartenere a una pagina successiva) risulta attribuito
// alla PAGINA PRECEDENTE (pagina 1), non a pagina 2. Prima della
// correzione a pageForOffset fatta in questo stesso giro di fix, un
// confronto ingenuo su starts uguali avrebbe vinto per l'indice più alto
// (pagina 2, quella il cui ancoraggio è fallito) — l'esatto contrario di
// quanto promesso: vedi il commento su pageForOffset per la prova
// rosso/verde di QUESTA correzione, fatta rompendo di nuovo la funzione.
func TestIndexMedia_TextLayerPDFAnchorFailureFallsBackToThePreviousPage(t *testing.T) {
	replaced := "Testo completamente estraneo restituito dal segmentatore finto, che non condivide " +
		"nessuna parola con le due pagine originali del PDF e quindi non puo essere ritrovato tramite " +
		"l'ancora dei primi quaranta caratteri di nessuna delle due, forzando il ripiego dichiarato."
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{out: replaced}

	page1 := "## Sezione\n\n" + "Questo e il testo vero della prima pagina, che non compare nel sostituito."
	page2 := "Questo e il testo vero della seconda pagina, anche lui assente dal sostituito."
	raw := newTwoPageTextPDF(page1, page2)
	if !manuals.HasTextLayer(raw) {
		t.Fatal("il fixture deve avere un vero layer testo")
	}

	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, raw, "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"estraneo"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("nessun chunk trovato per il testo sostituito")
	}
	for _, h := range hits {
		if h.ReferenceDetail != "pagina 1" {
			t.Fatalf("con l'ancora di pagina 2 introvabile, il fallback dichiarato è \"eredita l'offset della "+
				"precedente\": atteso \"pagina 1\", ottenuto %q (testo: %q)", h.ReferenceDetail, h.Text)
		}
	}
}

// TestManualTarget_RejectsMediaFromAnotherGame pinna il controllo che
// manualTarget faceva già prima di questo task: un media di un altro
// gioco è "non trovato" (404), non un parametro di rotta malformato.
func TestManualTarget_RejectsMediaFromAnotherGame(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	_, mediaID := seedGameWithFile(t, server, []byte("# T\n\nx"), "a.md", "")
	otherGameID, _ := seedGameWithFile(t, server, []byte("# T\n\nx"), "b.md", "")

	req := httptest.NewRequest(http.MethodDelete, indexPath(otherGameID, mediaID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("un media di un altro gioco è un \"non trovato\": atteso 404, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

// expectedTranscribeConcurrency ripete il valore di transcribeConcurrency,
// la costante del pool in manuals_handlers.go, che non è esportata. È
// ripetuta e non letta di là di proposito: la soglia è ciò che questi due
// test verificano, quindi cambiarla nel codice di produzione deve rompere
// il test e costringere a una decisione, non adattarsi in silenzio.
const expectedTranscribeConcurrency = 2

// barrierTranscriber blocca ogni pagina finché non ne sono arrivate
// `barrier` CONTEMPORANEAMENTE, poi le libera tutte insieme. È questa
// forma, e non uno sleep, che rende deterministico il test sulla
// concorrenza: con una trascrizione sequenziale la prima pagina aspetta
// una compagna che non arriverà mai, quindi il select ha anche un timeout
// che fa fallire il test in modo leggibile invece di appenderlo.
//
// peak registra il massimo di chiamate contemporanee osservate: è
// l'osservabile su cui il test asserisce in ENTRAMBE le direzioni, perché
// un pool troppo largo è un guasto quanto uno che non parallelizza.
type barrierTranscriber struct {
	barrier int
	reached chan struct{}
	once    sync.Once

	mu       sync.Mutex
	inFlight int
	peak     int
}

func newBarrierTranscriber(barrier int) *barrierTranscriber {
	return &barrierTranscriber{barrier: barrier, reached: make(chan struct{})}
}

func (b *barrierTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	b.mu.Lock()
	b.inFlight++
	if b.inFlight > b.peak {
		b.peak = b.inFlight
	}
	full := b.inFlight >= b.barrier
	b.mu.Unlock()

	if full {
		b.once.Do(func() { close(b.reached) })
	}
	select {
	case <-b.reached:
	case <-time.After(2 * time.Second):
	}

	b.mu.Lock()
	b.inFlight--
	b.mu.Unlock()

	return fmt.Sprintf(
		"## Sezione %d\n\nTesto della pagina %d del regolamento, con la parola unica segnalibro%d per ritrovarlo.",
		page, page, page), nil
}

func (b *barrierTranscriber) observedPeak() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.peak
}

// TestIndexMedia_ScannedPDFTranscribesPagesInParallel è la ragione di
// questo intervento: un manuale scansionato di N pagine costava N chiamate
// al modello IN SEQUENZA, cioè minuti d'attesa dentro una singola request
// HTTP. Il pool ne tiene in volo transcribeConcurrency alla volta.
//
// Il PDF ha il doppio delle pagine della concorrenza attesa, così il pool
// deve RIUSARE gli slot invece di limitarsi a far partire tutto in un
// colpo: un'implementazione senza limite (una goroutine per pagina) fa
// salire il picco a 10 e questo test la boccia, che è metà del suo scopo —
// dieci richieste vision insieme prendono 429 da qualunque provider a
// tariffa gratuita.
func TestIndexMedia_ScannedPDFTranscribesPagesInParallel(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{} // gate: il provider di testo è configurato
	vis := newBarrierTranscriber(expectedTranscribeConcurrency)
	server.Vision = vis
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		manuals.NewScannedPDFPages(2*expectedTranscribeConcurrency), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if peak := vis.observedPeak(); peak != expectedTranscribeConcurrency {
		t.Fatalf("chiamate contemporanee: atteso un picco di %d, osservato %d", expectedTranscribeConcurrency, peak)
	}
}

// reverseOrderTranscriber fa finire le pagine nell'ordine ESATTAMENTE
// opposto a quello del documento: la pagina 1 è la più lenta, l'ultima la
// più rapida. Con il pool tutte partono insieme, quindi l'ordine di arrivo
// dei risultati è quello inverso — che è la condizione in cui
// un'implementazione che accoda i risultati nell'ordine in cui arrivano
// (invece di scriverli al loro indice) sbaglia, e in cui una che li mette
// al loro posto non può sbagliare.
type reverseOrderTranscriber struct{ pages int }

func (r *reverseOrderTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	time.Sleep(time.Duration(r.pages-page+1) * 30 * time.Millisecond)
	return fmt.Sprintf(
		"## Sezione %d\n\nTesto della pagina %d del regolamento, con la parola unica segnalibro%d per ritrovarlo.",
		page, page, page), nil
}

// TestIndexMedia_ScannedPDFKeepsPageOrderWhenTranscriptionsFinishOutOfOrder
// è il rischio che la parallelizzazione introduce: con le pagine in volo
// insieme, l'ordine in cui il modello risponde non è più l'ordine del
// documento. Il seq dei chunk deve restare quello del documento comunque,
// perché è ciò su cui si appoggiano due cose che non farebbero rumore
// sbagliando: attachNeighbours, che allega "il chunk vicino" come seq ± 1,
// e Summary, che elenca i titoli di sezione "in ordine di seq" per
// l'indice iniettato nel prompt e per le domande suggerite.
func TestIndexMedia_ScannedPDFKeepsPageOrderWhenTranscriptionsFinishOutOfOrder(t *testing.T) {
	const pages = 5
	server, conn := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	server.Vision = &reverseOrderTranscriber{pages: pages}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, manuals.NewScannedPDFPages(pages), "manuale.pdf", "")
	_ = gameID

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	rows, err := conn.QueryContext(context.Background(),
		`SELECT reference_detail FROM game_source_chunk WHERE game_media_id = ? ORDER BY seq`, mediaID)
	if err != nil {
		t.Fatalf("query chunks: %v", err)
	}
	defer rows.Close()

	var got []int
	for rows.Next() {
		var detail string
		if err := rows.Scan(&detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var page int
		if _, err := fmt.Sscanf(detail, "pagina %d", &page); err != nil {
			t.Fatalf("reference_detail inatteso %q: %v", detail, err)
		}
		got = append(got, page)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(got) != pages {
		t.Fatalf("atteso un chunk per pagina (%d), ottenuti %d: %v", pages, len(got), got)
	}
	for i, page := range got {
		if page != i+1 {
			t.Fatalf("i chunk in ordine di seq devono seguire l'ordine del documento: atteso pagina %d in posizione %d, ottenuto %v", i+1, i, got)
		}
	}
}

// countingErroringTranscriber è erroringTranscriber che conta i tentativi:
// serve al test dell'uscita anticipata, dove ciò che conta non è il
// messaggio (già coperto altrove) ma QUANTE pagine sono state tentate.
type countingErroringTranscriber struct {
	err   error
	calls atomic.Int64
}

func (c *countingErroringTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	c.calls.Add(1)
	return "", c.err
}

// TestIndexMedia_ScannedPDFWithoutVisionModelStopsEarly protegge una
// proprietà che la parallelizzazione poteva far perdere in silenzio. Un
// modello vision non configurato fallisce identicamente su ogni pagina:
// prima del pool si uscìva alla prima, con un `return` dentro il ciclo,
// e sostituendo quel ciclo con delle goroutine quel `return` non ferma
// più niente da sé. La prima pagina che lo scopre deve annullare il
// contesto, così le pagine ancora in coda non partono nemmeno.
//
// La soglia è "meno di una pagina per pagina del documento", non un
// numero esatto: le pagine già in volo quando arriva il cancel() sono
// legittimamente tentate, e quante siano dipende dallo scheduler.
func TestIndexMedia_ScannedPDFWithoutVisionModelStopsEarly(t *testing.T) {
	const pages = 4 * expectedTranscribeConcurrency
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	vis := &countingErroringTranscriber{err: ai.ErrNotConfigured}
	server.Vision = vis
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server, manuals.NewScannedPDFPages(pages), "manuale.pdf", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if calls := vis.calls.Load(); calls >= pages {
		t.Fatalf("un modello vision non configurato va scoperto una volta, non %d: tentate %d pagine su %d", pages, calls, pages)
	}
}

func TestIndexMedia_GeneratesSuggestedQuestions(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	sug := &fakeSuggester{}
	server.Suggester = sug
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if sug.calls.Load() != 1 {
		t.Fatalf("attesa 1 chiamata a SuggestQuestions, fatte %d", sug.calls.Load())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande salvate, ottenute %d: %v", len(got), got)
	}
	if got[0].Text != "Generata 1?" {
		t.Fatalf("prima domanda inattesa: %q", got[0].Text)
	}
}

// TestIndexMedia_SuggestionFailureStillIndexes: la generazione è
// best-effort. Trasformare un'indicizzazione riuscita in un errore per una
// domanda suggerita sarebbe fuori scala rispetto al valore della feature.
func TestIndexMedia_SuggestionFailureStillIndexes(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	server.Suggester = &fakeSuggester{err: ai.ErrSuggestionsRejected}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("una generazione fallita non deve far fallire l'indicizzazione: %d %s",
			rec.Code, rec.Body.String())
	}
	// I chunk devono esserci comunque.
	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"mazzo"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("i chunk devono essere stati indicizzati anche senza domande suggerite")
	}
}

// TestIndexMedia_SkipsSuggestionWhenAllThreeAreEdited: se non c'è niente da
// riscrivere non c'è motivo di pagare la chiamata.
func TestIndexMedia_SkipsSuggestionWhenAllThreeAreEdited(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	sug := &fakeSuggester{}
	server.Suggester = sug
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	// Tutte tre scritte a mano prima dell'indicizzazione.
	if err := server.Manuals.SaveEditedQuestions(context.Background(), gameID,
		[]string{"Mia 1?", "Mia 2?", "Mia 3?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if sug.calls.Load() != 0 {
		t.Fatalf("con tutte tre edited non c'è niente da generare: fatte %d chiamate", sug.calls.Load())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if got[0].Text != "Mia 1?" {
		t.Fatalf("le domande scritte a mano devono essere intatte: %v", got)
	}
}
