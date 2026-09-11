package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

// Le tre forme di risposta del banco prestiti, come struct: un typo in un
// nome di campo diventa un test rosso invece di un nil silenzioso.
type deskBookingBody struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type openLoanBody struct {
	ID            int64   `json:"id"`
	BorrowerName  string  `json:"borrowerName"`
	BorrowerPhone string  `json:"borrowerPhone"`
	LentAt        string  `json:"lentAt"`
	Notes         *string `json:"notes"`
}

type materialBody struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type materialIssueBody struct {
	Name     string `json:"name"`
	Expected int    `json:"expected"`
	Returned *int   `json:"returned"`
}

type deskCopyBody struct {
	EventGameID    int64             `json:"eventGameId"`
	GameID         int64             `json:"gameId"`
	Name           string            `json:"name"`
	CopyIndex      int               `json:"copyIndex"`
	Copies         int               `json:"copies"`
	Bookable       bool              `json:"bookable"`
	Seats          int               `json:"seats"`
	ActiveBookings []deskBookingBody `json:"activeBookings"`
	OpenLoan       *openLoanBody     `json:"openLoan"`
	Materials      []materialBody    `json:"materials"`
	Incomplete     bool              `json:"incomplete"`
}

type returnedLoanBody struct {
	ID             int64               `json:"id"`
	EventGameID    int64               `json:"eventGameId"`
	GameName       string              `json:"gameName"`
	CopyIndex      int                 `json:"copyIndex"`
	BorrowerName   string              `json:"borrowerName"`
	LentAt         string              `json:"lentAt"`
	ReturnedAt     string              `json:"returnedAt"`
	Notes          *string             `json:"notes"`
	MaterialIssues []materialIssueBody `json:"materialIssues"`
}

type loanDeskBody struct {
	Copies   []deskCopyBody     `json:"copies"`
	Returned []returnedLoanBody `json:"returned"`
}

type loanBody struct {
	ID            int64   `json:"id"`
	EventGameID   int64   `json:"eventGameId"`
	BookingID     *int64  `json:"bookingId"`
	BorrowerName  string  `json:"borrowerName"`
	BorrowerPhone string  `json:"borrowerPhone"`
	Notes         *string `json:"notes"`
	LentAt        string  `json:"lentAt"`
	ReturnedAt    *string `json:"returnedAt"`
}

// loanFixture monta un evento con `copies` copie di un gioco e restituisce
// il router, il cookie admin, l'id dell'evento e le sue copie.
func loanFixture(t *testing.T, copies int) (http.Handler, *http.Cookie, int64, []events.EventGame) {
	t.Helper()
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: copies}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	return router, cookie, event.ID, eventGames
}

// doLoanRequest manda una richiesta con o senza cookie e restituisce il
// recorder, così ogni test guarda sia il codice sia il corpo.
func doLoanRequest(router http.Handler, method, path string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func readDesk(t *testing.T, router http.Handler, cookie *http.Cookie, eventID int64) loanDeskBody {
	t.Helper()
	rec := doLoanRequest(router, http.MethodGet, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET loans: %d %s", rec.Code, rec.Body.String())
	}
	var desk loanDeskBody
	if err := json.NewDecoder(rec.Body).Decode(&desk); err != nil {
		t.Fatalf("decode desk: %v", err)
	}
	return desk
}

// lend apre un prestito dall'endpoint e restituisce il prestito creato.
func lend(t *testing.T, router http.Handler, cookie *http.Cookie, eventID, eventGameID int64, name, phone, notes string) loanBody {
	t.Helper()
	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":%q,"borrowerPhone":%q,"notes":%q}`,
		eventGameID, name, phone, notes)
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST loans: %d %s", rec.Code, rec.Body.String())
	}
	var loan loanBody
	if err := json.NewDecoder(rec.Body).Decode(&loan); err != nil {
		t.Fatalf("decode loan: %v", err)
	}
	return loan
}

func TestLoanDesk_ListsEveryCopyAsAvailable(t *testing.T) {
	router, cookie, eventID, _ := loanFixture(t, 2)

	desk := readDesk(t, router, cookie, eventID)

	if len(desk.Copies) != 2 {
		t.Fatalf("copies = %d, want 2", len(desk.Copies))
	}
	first := desk.Copies[0]
	if first.OpenLoan != nil {
		t.Error("openLoan valorizzato su una copia mai prestata")
	}
	if !first.Bookable {
		t.Error("bookable = false su una copia creata senza dire niente")
	}
	if first.Copies != 2 {
		t.Errorf("copies = %d, want 2: serve alla UI per numerare le copie", first.Copies)
	}
	if first.Name != "Carcassonne" {
		t.Errorf("name = %q, want Carcassonne", first.Name)
	}
	if first.ActiveBookings == nil {
		t.Error("activeBookings = null, want []: una lista vuota non deve arrivare come null")
	}
	if desk.Returned == nil {
		t.Error("returned = null, want []")
	}
	if len(desk.Returned) != 0 {
		t.Errorf("returned = %d righe, want 0", len(desk.Returned))
	}
}

func TestLoanDesk_RequiresASession(t *testing.T) {
	router, _, eventID, eventGames := loanFixture(t, 1)

	for _, tc := range []struct{ name, method, path, body string }{
		{"lettura", http.MethodGet, fmt.Sprintf("/api/events/%d/loans", eventID), ""},
		{"consegna", http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID),
			fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"Anna","borrowerPhone":"3331234567"}`, eventGames[0].ID)},
		{"restituzione", http.MethodPost, "/api/loans/1/return", `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doLoanRequest(router, tc.method, tc.path, nil, tc.body)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestLoanDesk_UnknownEventIs404(t *testing.T) {
	router, cookie, _, _ := loanFixture(t, 1)

	rec := doLoanRequest(router, http.MethodGet, "/api/events/9999/loans", cookie, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateLoan_ThenTheCopyIsOut(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)

	created := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "scatola ok")

	if created.BorrowerName != "Anna" {
		t.Errorf("borrowerName = %q, want Anna", created.BorrowerName)
	}
	if created.ReturnedAt != nil {
		t.Error("returnedAt valorizzato su un prestito appena aperto")
	}
	if created.LentAt == "" {
		t.Error("lentAt vuoto")
	}

	desk := readDesk(t, router, cookie, eventID)
	open := desk.Copies[0].OpenLoan
	if open == nil {
		t.Fatal("openLoan nil dopo la consegna")
	}
	if open.BorrowerName != "Anna" {
		t.Errorf("openLoan.borrowerName = %q, want Anna", open.BorrowerName)
	}
	if open.Notes == nil || *open.Notes != "scatola ok" {
		t.Errorf("openLoan.notes = %v, want \"scatola ok\"", open.Notes)
	}
}

func TestCreateLoan_WithoutBorrowerIs400(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"","borrowerPhone":""}`, eventGames[0].ID)

	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateLoan_OnACopyAlreadyOutIs409(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"Bruno","borrowerPhone":"3339999999"}`, eventGames[0].ID)
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateLoan_OnACopyOfAnotherEventIs404(t *testing.T) {
	router, cookie, _, eventGames := loanFixture(t, 1)
	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"Anna","borrowerPhone":"3331234567"}`, eventGames[0].ID)

	// L'evento 9999 non esiste: quella copia non gli appartiene.
	rec := doLoanRequest(router, http.MethodPost, "/api/events/9999/loans", cookie, body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestReturnLoan_MovesItToTheLog(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie,
		`{"notes":"manca una tessera"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	desk := readDesk(t, router, cookie, eventID)
	if desk.Copies[0].OpenLoan != nil {
		t.Error("la copia è ancora fuori dopo la restituzione")
	}
	if len(desk.Returned) != 1 {
		t.Fatalf("returned = %d righe, want 1", len(desk.Returned))
	}
	row := desk.Returned[0]
	if row.Notes == nil || *row.Notes != "manca una tessera" {
		t.Errorf("notes = %v", row.Notes)
	}
	if row.ReturnedAt == "" {
		t.Error("returnedAt vuoto nel log")
	}
	if row.GameName != "Carcassonne" {
		t.Errorf("gameName = %q, want Carcassonne", row.GameName)
	}
}

func TestReturnLoan_TwiceIs409(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")
	path := fmt.Sprintf("/api/loans/%d/return", loan.ID)
	if rec := doLoanRequest(router, http.MethodPost, path, cookie, `{}`); rec.Code != http.StatusOK {
		t.Fatalf("first return: %d %s", rec.Code, rec.Body.String())
	}

	rec := doLoanRequest(router, http.MethodPost, path, cookie, `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestReturnLoan_UnknownIDIs404(t *testing.T) {
	router, cookie, _, _ := loanFixture(t, 1)

	rec := doLoanRequest(router, http.MethodPost, "/api/loans/9999/return", cookie, `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestLoanDesk_ShowsActiveBookingsOfACopy(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	// La prenotazione entra dall'endpoint pubblico, così nome e telefono
	// sono quelli veri.
	booking := fmt.Sprintf(
		`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","participantPhone":"3331234567"}`,
		eventGames[0].ID)
	if rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), nil, booking); rec.Code != http.StatusCreated {
		t.Fatalf("booking: %d %s", rec.Code, rec.Body.String())
	}

	desk := readDesk(t, router, cookie, eventID)

	if len(desk.Copies[0].ActiveBookings) != 1 {
		t.Fatalf("activeBookings = %d righe, want 1", len(desk.Copies[0].ActiveBookings))
	}
	row := desk.Copies[0].ActiveBookings[0]
	if row.Name != "Anna" || row.Phone != "3331234567" {
		t.Errorf("activeBookings[0] = %+v, want Anna / 3331234567", row)
	}
}

func TestCreateLoan_FromABookingLinksIt(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	booking := fmt.Sprintf(
		`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","participantPhone":"3331234567"}`,
		eventGames[0].ID)
	if rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), nil, booking); rec.Code != http.StatusCreated {
		t.Fatalf("booking: %d %s", rec.Code, rec.Body.String())
	}
	desk := readDesk(t, router, cookie, eventID)
	bookingID := desk.Copies[0].ActiveBookings[0].ID

	body := fmt.Sprintf(
		`{"eventGameId":%d,"bookingId":%d,"borrowerName":"Anna","borrowerPhone":"3331234567"}`,
		eventGames[0].ID, bookingID)
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var loan loanBody
	if err := json.NewDecoder(rec.Body).Decode(&loan); err != nil {
		t.Fatalf("decode loan: %v", err)
	}
	if loan.BookingID == nil || *loan.BookingID != bookingID {
		t.Fatalf("bookingId = %v, want %d", loan.BookingID, bookingID)
	}
}

// loanFixtureWithMaterials è loanFixture più le voci del catalogo sul
// gioco della serata, e restituisce i loro id nell'ordine in cui sono
// state scritte. Non riusa loanFixture perché ha bisogno del *Server per
// scrivere i materiali, che loanFixture non restituisce.
func loanFixtureWithMaterials(t *testing.T, rows ...games.MaterialInput) (http.Handler, *http.Cookie, int64, []events.EventGame, []int64) {
	t.Helper()
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	saved, err := server.Games.ReplaceMaterials(context.Background(), gameID, rows)
	if err != nil {
		t.Fatalf("replace materials: %v", err)
	}
	ids := make([]int64, 0, len(saved))
	for _, m := range saved {
		ids = append(ids, m.ID)
	}

	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	return router, cookie, event.ID, eventGames, ids
}

func TestLoanDesk_CarriesTheGameMaterials(t *testing.T) {
	router, cookie, eventID, _, ids := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
		games.MaterialInput{Name: "meeple", Quantity: 40},
	)

	desk := readDesk(t, router, cookie, eventID)
	if len(desk.Copies) != 1 {
		t.Fatalf("copies = %d, want 1", len(desk.Copies))
	}
	got := desk.Copies[0].Materials
	if len(got) != 2 {
		t.Fatalf("materials = %d righe, want 2: %+v", len(got), got)
	}
	if got[0].ID != ids[0] || got[0].Name != "tessere" || got[0].Quantity != 72 {
		t.Errorf("prima voce = %+v", got[0])
	}
	if got[1].Name != "meeple" || got[1].Quantity != 40 {
		t.Errorf("seconda voce = %+v", got[1])
	}
}

func TestReturnLoan_WithChecklistRecordsWhatIsMissing(t *testing.T) {
	router, cookie, eventID, eventGames, ids := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
		games.MaterialInput{Name: "carte", Quantity: 40},
		games.MaterialInput{Name: "dadi", Quantity: 5},
	)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	// tessere spuntate, carte contate 35 su 40, dadi mai toccati.
	body := fmt.Sprintf(
		`{"materials":[{"materialId":%d,"complete":true},{"materialId":%d,"complete":false,"returned":35}]}`,
		ids[0], ids[1])
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	desk := readDesk(t, router, cookie, eventID)
	if len(desk.Returned) != 1 {
		t.Fatalf("returned = %d righe, want 1", len(desk.Returned))
	}
	issues := desk.Returned[0].MaterialIssues
	if len(issues) != 2 {
		t.Fatalf("materialIssues = %d righe, want 2 (carte e dadi): %+v", len(issues), issues)
	}
	if issues[0].Name != "carte" || issues[0].Expected != 40 ||
		issues[0].Returned == nil || *issues[0].Returned != 35 {
		t.Errorf("riga incompleta = %+v", issues[0])
	}
	if issues[1].Name != "dadi" || issues[1].Returned != nil {
		t.Errorf("riga non verificata = %+v", issues[1])
	}
	for _, iss := range issues {
		if iss.Name == "tessere" {
			t.Error("una voce spuntata non deve lasciare un esito")
		}
	}
}

func TestReturnLoan_WithoutMaterialsFieldStillWorks(t *testing.T) {
	router, cookie, eventID, eventGames, _ := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
	)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	// Il corpo di ieri: nessun campo materials. È il contratto che tiene in
	// piedi i giochi senza materiali e i client non aggiornati.
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	desk := readDesk(t, router, cookie, eventID)
	if len(desk.Returned) != 1 {
		t.Fatalf("returned = %d righe, want 1", len(desk.Returned))
	}
	if len(desk.Returned[0].MaterialIssues) != 0 {
		t.Fatalf("senza checklist non si scrive niente: %+v", desk.Returned[0].MaterialIssues)
	}
}

func TestReturnLoan_RejectsANegativeReturnedQuantity(t *testing.T) {
	router, cookie, eventID, eventGames, ids := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
	)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	body := fmt.Sprintf(`{"materials":[{"materialId":%d,"complete":false,"returned":-3}]}`, ids[0])
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	// O si chiude con il suo esito, o non si chiude: la copia deve essere
	// ancora fuori.
	desk := readDesk(t, router, cookie, eventID)
	if desk.Copies[0].OpenLoan == nil {
		t.Error("il prestito si è chiuso nonostante l'errore")
	}
}

// Il banco deve dire quale scatola è già incompleta PRIMA di consegnarla:
// chi la dà in mano lo sa, e chi la riporta non si prende una colpa non
// sua. Due giochi nella stessa serata perché il campo esce da una mappa
// per gioco: con uno solo non si vedrebbe se la mancanza finisce sulla
// riga giusta.
func TestLoanDesk_MarksOnlyTheIncompleteGame(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	shortGameID := createTestGameForEvent(t, server.Games, "Carcassonne")
	cleanGameID := createTestGameForEvent(t, server.Games, "Azul")
	saved, err := server.Games.ReplaceMaterials(context.Background(), shortGameID,
		[]games.MaterialInput{{Name: "carte", Quantity: 40}})
	if err != nil {
		t.Fatalf("materials: %v", err)
	}
	if _, err := server.Games.ReplaceMaterials(context.Background(), cleanGameID,
		[]games.MaterialInput{{Name: "tessere", Quantity: 100}}); err != nil {
		t.Fatalf("materials: %v", err)
	}

	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{
			{GameID: shortGameID, Copies: 1},
			{GameID: cleanGameID, Copies: 1},
		},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}

	copyOf := func(desk loanDeskBody, gameID int64) deskCopyBody {
		t.Helper()
		for _, c := range desk.Copies {
			if c.GameID == gameID {
				return c
			}
		}
		t.Fatalf("il gioco %d non è al banco: %+v", gameID, desk.Copies)
		return deskCopyBody{}
	}

	// Prima di qualunque riconsegna nessuno dei due è marchiato.
	before := readDesk(t, router, cookie, event.ID)
	if copyOf(before, shortGameID).Incomplete || copyOf(before, cleanGameID).Incomplete {
		t.Fatalf("senza riconsegne nessun gioco è incompleto: %+v", before.Copies)
	}

	var shortCopyID int64
	for _, eg := range eventGames {
		if eg.GameID == shortGameID {
			shortCopyID = eg.ID
		}
	}
	loan := lend(t, router, cookie, event.ID, shortCopyID, "Anna", "3331234567", "")
	body := fmt.Sprintf(`{"materials":[{"materialId":%d,"complete":false,"returned":35}]}`, saved[0].ID)
	if rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, body); rec.Code != http.StatusOK {
		t.Fatalf("return: %d %s", rec.Code, rec.Body.String())
	}

	after := readDesk(t, router, cookie, event.ID)
	if !copyOf(after, shortGameID).Incomplete {
		t.Errorf("il gioco con la mancanza deve risultare incompleto: %+v", copyOf(after, shortGameID))
	}
	if copyOf(after, cleanGameID).Incomplete {
		t.Errorf("il gioco pulito non deve essere marchiato: %+v", copyOf(after, cleanGameID))
	}
}
