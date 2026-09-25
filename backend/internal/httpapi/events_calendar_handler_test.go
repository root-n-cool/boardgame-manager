package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/httpapi"
)

func getCalendar(t *testing.T, router http.Handler, eventID int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d/calendar.ics", eventID), nil)
	req.Host = "giochi.example.org"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestEventCalendar_ServesAnICSWithTheEventDetails(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	lat, lon := 45.07, 7.68
	desc := "Serata aperta a tutti, porta un amico; si gioca fino a tardi"
	ev, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title:       "Serata, strategici",
		Description: &desc,
		EventDate:   "2099-03-14",
		StartTime:   "20:30",
		EndTime:     ptrTo("23:30"),
		Venue:       &events.Venue{Name: "Circolo Arci", Address: "Via Roma 1, Torino", Lat: &lat, Lon: &lon},
		Games:       []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	rec := getCalendar(t, router, ev.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Fatalf("expected text/calendar, got %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, ".ics") {
		t.Fatalf("expected an .ics attachment name, got %q", cd)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\r\n") {
		t.Fatal("expected CRLF line endings")
	}
	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"BEGIN:VEVENT",
		"UID:event-" + fmt.Sprint(ev.ID) + "@giochi.example.org",
		"DTSTART:20990314T203000",
		"DTEND:20990314T233000",
		"SUMMARY:Serata\\, strategici",
		"LOCATION:Circolo Arci\\, Via Roma 1\\, Torino",
		"GEO:45.07;7.68",
		"URL:http://giochi.example.org/events/" + fmt.Sprint(ev.ID),
		"END:VEVENT",
		"END:VCALENDAR",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "porta un amico\\; si gioca") {
		t.Errorf("expected the escaped description in:\n%s", body)
	}
}

func TestEventCalendar_EndsPastMidnightOnTheNextDay(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	ev, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Nottambuli", EventDate: "2099-12-31", StartTime: "22:00", EndTime: ptrTo("01:00"),
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	body := getCalendar(t, router, ev.ID).Body.String()
	if !strings.Contains(body, "DTEND:21000101T010000") {
		t.Fatalf("expected the end on the next day, got:\n%s", body)
	}
	if strings.Contains(body, "LOCATION:") || strings.Contains(body, "GEO:") {
		t.Fatalf("expected no location for an event without venue:\n%s", body)
	}
}

func TestEventCalendar_UnknownEventIs404(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	if rec := getCalendar(t, router, 999); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestEventCalendar_FoldsLongLinesWithoutSplittingCharacters(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	title := strings.Repeat("Più città è ", 12)
	ev, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: title, EventDate: "2099-03-14", StartTime: "20:30",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	body := getCalendar(t, router, ev.ID).Body.String()
	for _, line := range strings.Split(body, "\r\n") {
		if len(line) > 75 {
			t.Errorf("line longer than 75 octets: %q", line)
		}
		if !utf8.ValidString(line) {
			t.Errorf("fold split a character: %q", line)
		}
	}
	unfolded := strings.ReplaceAll(body, "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+strings.TrimSpace(title)) {
		t.Fatalf("expected the whole title once unfolded:\n%s", unfolded)
	}
}

func TestEventCalendar_WithoutAnEndRunsUntilMidnight(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	ev, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Aperta", EventDate: "2099-03-14", StartTime: "20:30",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	body := getCalendar(t, router, ev.ID).Body.String()
	if !strings.Contains(body, "DTEND:20990315T000000") {
		t.Fatalf("expected the end at midnight, got:\n%s", body)
	}
}
