package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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
type fakeTranscriber struct {
	calls int
	err   error
}

func (f *fakeTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	f.calls++
	if f.err != nil {
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
// passerebbe indisturbato.
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
	if rec.Code == http.StatusOK {
		t.Fatalf("un media di un altro gioco non deve essere accettato, ottenuto 200: %s", rec.Body.String())
	}
}
