package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// fakeTranscriber finge un modello multimodale: restituisce un testo che
// contiene il numero di pagina, così i test verificano l'accoppiamento.
// errOnPage, se diverso da zero, limita err a quella sola pagina: le altre
// riescono normalmente. È così che si esercita l'isolamento dei guasti
// (una pagina che fallisce non deve far sparire le altre) senza un secondo
// tipo finto.
type fakeTranscriber struct {
	calls     int
	err       error
	errOnPage int
}

func (f *fakeTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	f.calls++
	if f.err != nil && (f.errOnPage == 0 || f.errOnPage == page) {
		return "", f.err
	}
	return "Trascrizione della pagina " + fmt.Sprint(page), nil
}

// loginAsAdmin esegue il bootstrap del primo admin e restituisce il
// cookie di sessione: gli handler di questo file sono tutti protetti.
func loginAsAdmin(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	return bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
}

// seedGameWithManual crea un gioco con lingua "it" e un media "file" il
// cui contenuto su disco è pdfBytes. Passa direttamente per gli store
// (non per l'HTTP): un test come TestManualPages_RequireAuth deve poter
// preparare il fixture senza nessuna sessione admin.
func seedGameWithManual(t *testing.T, server *httpapi.Server, conn *sql.DB, pdfBytes []byte) (gameID, mediaID int64) {
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
	filename, err := server.Storage.Save(storage.ManualCategory, bytes.NewReader(pdfBytes))
	if err != nil {
		t.Fatalf("save manual: %v", err)
	}
	title := "Regolamento"
	media, err := server.Games.CreateMedia(ctx, games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeFile, URLOrPath: filename, Title: &title,
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}
	return game.ID, media.ID
}

// seedGameWithScannedManual è il caso più usato in questo file: un manuale
// scansionato (nessun layer testo), la forma del manuale reale del club.
func seedGameWithScannedManual(t *testing.T, server *httpapi.Server, conn *sql.DB) (gameID, mediaID int64) {
	t.Helper()
	return seedGameWithManual(t, server, conn, manuals.NewScannedPDF())
}

// scannedPDFWithMisleadingTextMarkers costruisce un PDF con le stesse due
// pagine scansionate di manuals.NewScannedPDF (nessun operatore di testo
// nel loro vero contenuto), più un font e un operatore Tj "civetta", mai
// referenziati da nessuna pagina. È esattamente la combinazione, isolata,
// che fa dire manuals.HasTextLayer=true senza che ci sia un vero layer
// testo: il caso limite della correzione 2 del brief. ExtractText, su un
// file così, non restituisce errore ma solo pagine vuote — ed è quello
// che extractManualHandler deve riconoscere per ripiegare su vision.
func scannedPDFWithMisleadingTextMarkers() []byte {
	jpg1 := manuals.NewTestJPEG(24, 32)
	jpg2 := manuals.NewTestJPEG(20, 28)
	content := "q 200 0 0 260 0 0 cm /Im0 Do Q" // disegna solo l'immagine: nessun Tj vero
	page := func(imgRef, contentRef string) string {
		return fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] "+
				"/Resources << /XObject << /Im0 %s >> >> /Contents %s >>", imgRef, contentRef)
	}
	streamObj := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	return manuals.BuildTestPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",                      // 1
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",        // 2
		page("5 0 R", "7 0 R"),                                   // 3
		page("6 0 R", "8 0 R"),                                   // 4
		manuals.ImageObject(jpg1, 24, 32),                        // 5
		manuals.ImageObject(jpg2, 20, 28),                        // 6
		streamObj(content),                                       // 7
		streamObj(content),                                       // 8
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", // 9: mai referenziato da nessuna pagina
		streamObj("(testo civetta) Tj"),                          // 10: mai referenziato da nessun /Contents
	})
}

// scannedPDFWithThinHeaderText costruisce un manuale scansionato di due
// pagine, ognuna con un'immagine a piena pagina *e* un vero operatore Tj
// che disegna la stessa intestazione corta ("Manuale Esempio", 15
// caratteri). A differenza di scannedPDFWithMisleadingTextMarkers, qui
// /Font e Tj sono nel contenuto vero della pagina, non in oggetti civetta
// mai referenziati: è la forma realistica del problema di scala descritto
// nel commento su minAvgUsableCharsPerPage — uno scanner che stampa
// un'intestazione o un numero di pagina in un vero (ma quasi vuoto) layer
// testo sopra l'immagine scansionata. Sommando su più pagine il totale
// cresce con il numero di pagine (qui ~30 caratteri su 2 pagine, già sopra
// una vecchia soglia sul totale di 20) restando comunque inutile riga per
// riga: solo una media per pagina lo riconosce.
func scannedPDFWithThinHeaderText() []byte {
	jpg1 := manuals.NewTestJPEG(24, 32)
	jpg2 := manuals.NewTestJPEG(20, 28)
	header := "Manuale Esempio"
	content := fmt.Sprintf("q 200 0 0 260 0 0 cm /Im0 Do Q BT /F1 12 Tf 10 10 Td (%s) Tj ET", header)
	page := func(imgRef, contentRef string) string {
		return fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] "+
				"/Resources << /XObject << /Im0 %s >> /Font << /F1 9 0 R >> >> /Contents %s >>", imgRef, contentRef)
	}
	streamObj := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	return manuals.BuildTestPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",                      // 1
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",        // 2
		page("5 0 R", "7 0 R"),                                   // 3
		page("6 0 R", "8 0 R"),                                   // 4
		manuals.ImageObject(jpg1, 24, 32),                        // 5
		manuals.ImageObject(jpg2, 20, 28),                        // 6
		streamObj(content),                                       // 7
		streamObj(content),                                       // 8
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", // 9
	})
}

func manualPath(gameID, mediaID int64, suffix string) string {
	return fmt.Sprintf("/api/games/%d/languages/it/media/%d/%s", gameID, mediaID, suffix)
}

func TestExtractManual_ReturnsPagesWithoutSaving(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	tr := &fakeTranscriber{}
	server.Vision = tr
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	req := httptest.NewRequest(http.MethodPost, manualPath(gameID, mediaID, "extract"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Source string `json:"source"`
		Pages  []struct {
			PageNumber int    `json:"pageNumber"`
			Text       string `json:"text"`
			Heading    string `json:"heading"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if body.Source != "vision" {
		t.Fatalf("un PDF scansionato va sul percorso vision, source = %q", body.Source)
	}
	if len(body.Pages) != 2 {
		t.Fatalf("attese 2 pagine (quante ne ha lo scan), ottenute %d", len(body.Pages))
	}
	if body.Pages[0].PageNumber != 1 || body.Pages[1].PageNumber != 2 {
		t.Fatalf("numerazione pagine sbagliata: %d, %d", body.Pages[0].PageNumber, body.Pages[1].PageNumber)
	}
	if tr.calls != len(body.Pages) {
		t.Fatalf("una richiesta per pagina: %d pagine, %d chiamate", len(body.Pages), tr.calls)
	}

	// extract NON deve salvare: la conferma dell'admin è un altro giro.
	pages, err := manuals.NewStore(conn).ListPages(context.Background(), mediaID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("extract ha salvato %d pagine: doveva solo proporle", len(pages))
	}
}

func TestExtractManual_WithoutAVisionModelReturnsEmptyPages(t *testing.T) {
	// Nessun modello vision configurato: l'anteprima si apre comunque, con
	// le pagine vuote, e l'admin scrive il testo a mano. La funzione non si
	// blocca perché manca l'AI.
	server, conn := newTestServerWithDB(t)
	server.Vision = &fakeTranscriber{err: ai.ErrNotConfigured}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	req := httptest.NewRequest(http.MethodPost, manualPath(gameID, mediaID, "extract"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("senza modello vision l'anteprima si apre comunque: atteso 200, ottenuto %d (%s)",
			rec.Code, rec.Body.String())
	}
	var body struct {
		Pages []struct {
			PageNumber int    `json:"pageNumber"`
			Text       string `json:"text"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	// Le pagine proposte devono corrispondere davvero alle pagine del PDF
	// (2, quante ne ha lo scan): un'implementazione che, in assenza di
	// vision, restituisse un'unica pagina vuota qualsiasi supererebbe un
	// controllo che si limitasse a "len(pages) != 0" senza che l'admin
	// abbia davvero un posto dove scrivere ciascuna pagina del manuale.
	if len(body.Pages) != 2 {
		t.Fatalf("attese 2 pagine vuote da riempire a mano (una per pagina del PDF), ottenute %d", len(body.Pages))
	}
	if body.Pages[0].PageNumber != 1 || body.Pages[1].PageNumber != 2 {
		t.Fatalf("numerazione pagine sbagliata: %d, %d", body.Pages[0].PageNumber, body.Pages[1].PageNumber)
	}
	for i, p := range body.Pages {
		if p.Text != "" {
			t.Fatalf("pagina %d non doveva avere testo: %q", i, p.Text)
		}
	}
}

// TestExtractManual_TextLayerFalsePositiveFallsBackToVision è la
// correzione 2 del brief: un PDF che manuals.HasTextLayer accetta (c'è un
// /Font e un operatore Tj da qualche parte nel file) ma la cui estrazione
// testo vera non produce niente di usabile deve comunque finire trascritto
// da vision, non tornare con pagine vuote silenziosamente.
func TestExtractManual_TextLayerFalsePositiveFallsBackToVision(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	tr := &fakeTranscriber{}
	server.Vision = tr
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	gameID, mediaID := seedGameWithManual(t, server, conn, scannedPDFWithMisleadingTextMarkers())

	req := httptest.NewRequest(http.MethodPost, manualPath(gameID, mediaID, "extract"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Source string `json:"source"`
		Pages  []struct {
			Text string `json:"text"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if body.Source != "vision" {
		t.Fatalf("un /Font e un Tj civetta senza vero testo devono ripiegare su vision, source = %q", body.Source)
	}
	if len(body.Pages) != 2 {
		t.Fatalf("attese 2 pagine trascritte, ottenute %d", len(body.Pages))
	}
	for i, p := range body.Pages {
		if p.Text == "" {
			t.Fatalf("pagina %d doveva avere il testo trascritto da vision, non vuoto", i)
		}
	}
	if tr.calls != 2 {
		t.Fatalf("attese 2 chiamate a vision (una per pagina), ottenute %d", tr.calls)
	}
}

// TestExtractManual_ThinPerPageTextFallsBackToVision è il caso di scala
// segnalato in review: un totale sommato su tutte le pagine cresce con il
// numero di pagine, quindi un'intestazione corta ma reale, ripetuta su più
// pagine, può superare una soglia sul totale nonostante resti inutile
// pagina per pagina. La media per pagina non ha questo buco: 15 caratteri
// di media restano sotto qualunque soglia ragionevole indipendentemente da
// quante pagine ripetono la stessa intestazione.
func TestExtractManual_ThinPerPageTextFallsBackToVision(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	tr := &fakeTranscriber{}
	server.Vision = tr
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	gameID, mediaID := seedGameWithManual(t, server, conn, scannedPDFWithThinHeaderText())

	req := httptest.NewRequest(http.MethodPost, manualPath(gameID, mediaID, "extract"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Source string `json:"source"`
		Pages  []struct {
			Text string `json:"text"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if body.Source != "vision" {
		t.Fatalf("un'intestazione corta ripetuta su più pagine deve ripiegare su vision (media troppo bassa), source = %q", body.Source)
	}
	if len(body.Pages) != 2 {
		t.Fatalf("attese 2 pagine trascritte, ottenute %d", len(body.Pages))
	}
	if tr.calls != 2 {
		t.Fatalf("attese 2 chiamate a vision (una per pagina), ottenute %d", tr.calls)
	}
}

// TestExtractManual_ShortAllTextPDFReturnsItsTextInsteadOfAnError copre
// l'effetto collaterale segnalato in review: alzare la soglia (o passare a
// una media) per chiudere il buco di scala sopra non deve trasformare un
// PDF di solo testo genuinamente corto — un cartoncino di riferimento di
// una pagina, senza nessuna immagine — in un errore 422. Il percorso
// vision, su un PDF così, non trova nessuna immagine da trascrivere: il
// ripiego deve restituire comunque il testo vero (per quanto debole in
// media) invece di rispondere con un errore.
func TestExtractManual_ShortAllTextPDFReturnsItsTextInsteadOfAnError(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	gameID, mediaID := seedGameWithManual(t, server, conn, manuals.NewTextPDF())

	req := httptest.NewRequest(http.MethodPost, manualPath(gameID, mediaID, "extract"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("un cartoncino di una pagina, per quanto corto, ha un testo vero: non deve tornare un errore. atteso 200, ottenuto %d: %s",
			rec.Code, rec.Body.String())
	}
	var body struct {
		Source string `json:"source"`
		Pages  []struct {
			PageNumber int    `json:"pageNumber"`
			Text       string `json:"text"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if body.Source != "pdf_text" {
		t.Fatalf("un PDF di solo testo senza immagini deve tornare il suo testo, source = %q", body.Source)
	}
	if len(body.Pages) != 1 {
		t.Fatalf("attesa 1 pagina, ottenute %d", len(body.Pages))
	}
	if !strings.Contains(body.Pages[0].Text, "Upkeep") {
		t.Fatalf("il testo estratto doveva essere quello vero del PDF, ottenuto %q", body.Pages[0].Text)
	}
}

// TestExtractManual_OnePageFailingTranscriptionDoesNotLoseTheOthers pinna
// l'isolamento dei guasti nel percorso vision: la pagina 2 fallisce, ma le
// pagine 1 e 3 devono comunque tornare col loro testo, la pagina 2 deve
// comunque essere presente (vuota, source "manual") e la numerazione non
// deve slittare — quei numeri sono la citazione che qualcuno usa per
// aprire il manuale alla pagina giusta.
func TestExtractManual_OnePageFailingTranscriptionDoesNotLoseTheOthers(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	tr := &fakeTranscriber{errOnPage: 2, err: errors.New("provider momentaneamente giù")}
	server.Vision = tr
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	// TRE pagine, con quella che fallisce IN MEZZO: con due pagine la
	// fallita sarebbe l'ultima, e il test non distinguerebbe una
	// numerazione presa da img.Number da una presa dall'indice di append —
	// che coincidono finché nessuna pagina "salta". Qui, se il numero
	// venisse dall'indice, la pagina 3 arriverebbe numerata 3 lo stesso ma
	// la 2 sarebbe l'unica a poter slittare: è il caso in mezzo che rende
	// visibile l'accoppiamento.
	gameID, mediaID := seedGameWithManual(t, server, conn, manuals.NewScannedPDFPages(3))

	req := httptest.NewRequest(http.MethodPost, manualPath(gameID, mediaID, "extract"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Pages []struct {
			PageNumber int    `json:"pageNumber"`
			Text       string `json:"text"`
			Source     string `json:"source"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if len(body.Pages) != 3 {
		t.Fatalf("attese 3 pagine (una fallita non deve far sparire le altre), ottenute %d", len(body.Pages))
	}
	for i, p := range body.Pages {
		if p.PageNumber != i+1 {
			t.Fatalf("la numerazione non deve slittare: pagina in posizione %d numerata %d", i, p.PageNumber)
		}
	}
	for _, i := range []int{0, 2} {
		if body.Pages[i].Text == "" || body.Pages[i].Source != "vision" {
			t.Fatalf("pagina %d doveva riuscire: text=%q source=%q",
				i+1, body.Pages[i].Text, body.Pages[i].Source)
		}
		// fakeTranscriber ignora i byte JPEG ed echeggia il proprio
		// argomento page (vedi fakeTranscriber più sopra): il testo che
		// arriva qui è quindi il numero di pagina passato a Transcribe,
		// non una lettura dell'immagine. Con la pagina fallita IN MEZZO
		// (vedi sopra), verificare che la pagina in posizione i porti il
		// numero i+1 lega il PageNumber della risposta all'argomento
		// realmente passato a Transcribe per quella pagina — e non
		// all'indice della sua posizione nello slice dei successi, che
		// coinciderebbe comunque se il codice ricomponesse per indice le
		// sole trascrizioni riuscite (bug invisibile se il fallimento è
		// l'ultima pagina).
		if !strings.Contains(body.Pages[i].Text, fmt.Sprint(i+1)) {
			t.Fatalf("pagina %d ha ricevuto la trascrizione di un'altra pagina: %q", i+1, body.Pages[i].Text)
		}
	}
	if body.Pages[1].Text != "" {
		t.Fatalf("pagina 2 (fallita) doveva arrivare con testo vuoto, non %q", body.Pages[1].Text)
	}
	if body.Pages[1].Source != "manual" {
		t.Fatalf("pagina 2 (fallita) doveva avere source \"manual\", non %q", body.Pages[1].Source)
	}
}

func TestPutManualPages_SavesAndBuildsTheIndex(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	payload := `{"pages":[
	  {"pageNumber":4,"text":"Fase di Upkeep\nOgni giocatore paga una moneta.","source":"vision"},
	  {"pageNumber":7,"text":"Fine partita\nLa partita termina subito.","source":"manual"}
	]}`
	req := httptest.NewRequest(http.MethodPut, manualPath(gameID, mediaID, "pages"), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	store := manuals.NewStore(conn)
	pages, _ := store.ListPages(context.Background(), mediaID)
	if len(pages) != 2 {
		t.Fatalf("attese 2 pagine salvate, ottenute %d", len(pages))
	}
	if pages[0].Heading != "Fase di Upkeep" {
		t.Fatalf("l'heading doveva essere rilevato al salvataggio, è %q", pages[0].Heading)
	}
	// I chunk devono essere pronti: la ricerca funziona subito dopo il salvataggio.
	res, err := store.Search(context.Background(), gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("dopo il salvataggio la ricerca non trova nulla: i chunk non sono stati costruiti")
	}
}

func TestManualPages_RequireAuth(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	for _, tc := range []struct{ method, suffix string }{
		{http.MethodPost, "extract"},
		{http.MethodGet, "pages"},
		{http.MethodPut, "pages"},
		{http.MethodDelete, "pages"},
	} {
		path := manualPath(gameID, mediaID, tc.suffix)
		req := httptest.NewRequest(tc.method, path, strings.NewReader(`{"pages":[]}`))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s senza sessione: atteso 401, ottenuto %d (%s)", tc.method, path, rec.Code, rec.Body.String())
		}
	}
}

func TestDeleteManualPages(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	store := manuals.NewStore(conn)
	store.ReplacePages(context.Background(), gameID, mediaID, "it", []manuals.StoredPage{
		{PageNumber: 1, Text: "Testo qualsiasi.", Source: "manual"},
	})

	req := httptest.NewRequest(http.MethodDelete, manualPath(gameID, mediaID, "pages"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("atteso 204, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	pages, _ := store.ListPages(context.Background(), mediaID)
	if len(pages) != 0 {
		t.Fatalf("restano %d pagine", len(pages))
	}
}

// TestManualTarget_RejectsMediaFromAnotherGame pinna il controllo
// descritto nel brief: manualTarget deve verificare che il media
// appartenga davvero a quel gioco (e a quella lingua), non solo che
// esista. Senza questo controllo, l'id di un media di un altro gioco
// passerebbe indisturbato. Lo status atteso è esattamente 404 (non un
// generico "diverso da 200"): un media che non appartiene a quel gioco è
// un caso "non trovato", come lo tratta translateLanguageHandler per lo
// stesso genere di lookup — non un parametro di rotta malformato (400).
func TestManualTarget_RejectsMediaFromAnotherGame(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)

	_, mediaID := seedGameWithScannedManual(t, server, conn)
	otherGameID, _ := seedGameWithScannedManual(t, server, conn)

	req := httptest.NewRequest(http.MethodGet, manualPath(otherGameID, mediaID, "pages"), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("un media di un altro gioco è un \"non trovato\": atteso 404, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}
