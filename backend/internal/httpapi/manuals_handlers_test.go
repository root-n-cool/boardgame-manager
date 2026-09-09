package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	calls  int
}

func (p *pageTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	p.calls++
	return p.byPage[page], nil
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
	if vis.calls != 0 {
		t.Fatalf("un .md non deve chiamare Transcribe, chiamato %d volte", vis.calls)
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
	if vis.calls != 0 {
		t.Fatalf("un PDF con layer testo non deve chiamare Transcribe, chiamato %d volte", vis.calls)
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
	if v, ok := server.Vision.(*pageTranscriber); ok && v.calls != 0 {
		t.Fatalf("un PDF con layer testo usabile non deve chiamare Transcribe, chiamato %d volte", v.calls)
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
