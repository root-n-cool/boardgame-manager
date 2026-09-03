package geocode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/geocode"
)

func TestSearch_ParsesPlacesAndSendsNominatimParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("q") != "via roma 1 torino" {
			t.Errorf("expected the query to be forwarded, got %q", q.Get("q"))
		}
		if q.Get("format") != "jsonv2" {
			t.Errorf("expected format=jsonv2, got %q", q.Get("format"))
		}
		if q.Get("limit") == "" {
			t.Error("expected a limit, Nominatim answers 10 results by default")
		}
		if q.Get("addressdetails") != "1" {
			t.Error("expected addressdetails=1, the display name alone is too verbose to print")
		}
		// La usage policy di Nominatim chiede uno User-Agent che identifichi
		// l'applicazione: senza, l'endpoint pubblico blocca le richieste.
		if !strings.Contains(r.Header.Get("User-Agent"), "boardgames-manager") {
			t.Errorf("expected an identifying User-Agent, got %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"place_id": 123,
				"lat": "45.0703",
				"lon": "7.6869",
				"name": "Circolo Arci",
				"display_name": "Circolo Arci, Via Roma, 1, Centro, Circoscrizione 1, Torino, Piemonte, 10123, Italia",
				"address": {
					"house_number": "1",
					"road": "Via Roma",
					"city": "Torino",
					"postcode": "10123",
					"country": "Italia"
				}
			}
		]`))
	}))
	defer server.Close()

	client := &geocode.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	places, err := client.Search(context.Background(), "via roma 1 torino")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(places) != 1 {
		t.Fatalf("expected 1 place, got %d", len(places))
	}
	got := places[0]
	if got.Name != "Circolo Arci" {
		t.Errorf("expected the short name, got %q", got.Name)
	}
	// Il display_name di Nominatim infila circoscrizione, provincia e nazione:
	// quello che si legge su una locandina è via, civico, CAP e città.
	if got.Address != "Via Roma 1, 10123 Torino" {
		t.Errorf("expected a readable address, got %q", got.Address)
	}
	if got.Lat != 45.0703 || got.Lon != 7.6869 {
		t.Errorf("expected the coordinates parsed as numbers, got %v %v", got.Lat, got.Lon)
	}
}

// Un luogo fuori dai casi normali (una frazione senza CAP, un posto
// all'estero) non ha i campi per comporre l'indirizzo: lì il display_name
// completo è meglio di una riga mutilata.
func TestSearch_FallsBackToTheDisplayNameWhenTheAddressIsUnusable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"lat": "45.07",
				"lon": "7.68",
				"name": "Rifugio",
				"display_name": "Rifugio, Val Chisone, Piemonte, Italia",
				"address": {"county": "Torino", "country": "Italia"}
			}
		]`))
	}))
	defer server.Close()

	client := &geocode.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	places, err := client.Search(context.Background(), "rifugio val chisone")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if places[0].Address != "Rifugio, Val Chisone, Piemonte, Italia" {
		t.Errorf("expected the display name, got %q", places[0].Address)
	}
}

func TestSearch_ReturnsErrorOnNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bandwidth limit exceeded", http.StatusForbidden)
	}))
	defer server.Close()

	client := &geocode.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.Search(context.Background(), "via roma"); err == nil {
		t.Fatal("expected an error when Nominatim refuses the request")
	}
}

func TestSearch_ReturnsNoPlacesForABlankQuery(t *testing.T) {
	// Nominatim risponde 400 a una query vuota: la richiesta non va nemmeno
	// fatta, perché è la tastiera dell'admin a produrla mentre cancella.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("expected no request to Nominatim for a blank query")
	}))
	defer server.Close()

	client := &geocode.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	places, err := client.Search(context.Background(), "   ")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(places) != 0 {
		t.Fatalf("expected no places, got %d", len(places))
	}
}
