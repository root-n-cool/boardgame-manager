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
}

type returnedLoanBody struct {
	ID           int64   `json:"id"`
	EventGameID  int64   `json:"eventGameId"`
	GameName     string  `json:"gameName"`
	CopyIndex    int     `json:"copyIndex"`
	BorrowerName string  `json:"borrowerName"`
	LentAt       string  `json:"lentAt"`
	ReturnedAt   string  `json:"returnedAt"`
	Notes        *string `json:"notes"`
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
