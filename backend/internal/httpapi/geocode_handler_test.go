package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/geocode"
	"boardgames-manager/internal/httpapi"
)

type fakeGeocodeClient struct {
	places []geocode.Place
	err    error
	query  string
	calls  int
}

func (f *fakeGeocodeClient) Search(ctx context.Context, query string) ([]geocode.Place, error) {
	f.calls++
	f.query = query
	return f.places, f.err
}

type placeResponse struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Lat     *float64 `json:"lat"`
	Lon     *float64 `json:"lon"`
}

func searchPlaces(t *testing.T, router http.Handler, query string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/geocode/search?q="+query, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSearchPlaces_ReturnsThePlacesFromNominatim(t *testing.T) {
	server := newTestServer(t)
	fake := &fakeGeocodeClient{places: []geocode.Place{
		{Name: "Circolo Arci", Address: "Circolo Arci, Via Roma 1, Torino", Lat: 45.07, Lon: 7.68},
	}}
	server.Geocode = fake
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "password123")

	rec := searchPlaces(t, router, "via+roma+torino", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body []placeResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected 1 place, got %d", len(body))
	}
	if body[0].Name != "Circolo Arci" || body[0].Address != "Circolo Arci, Via Roma 1, Torino" {
		t.Fatalf("unexpected place: %+v", body[0])
	}
	if body[0].Lat == nil || *body[0].Lat != 45.07 || body[0].Lon == nil || *body[0].Lon != 7.68 {
		t.Fatalf("expected the coordinates, got %+v", body[0])
	}
	if fake.query != "via roma torino" {
		t.Fatalf("expected the query forwarded, got %q", fake.query)
	}
}

func TestSearchPlaces_RequiresAnAdminSession(t *testing.T) {
	server := newTestServer(t)
	fake := &fakeGeocodeClient{}
	server.Geocode = fake
	router := httpapi.NewRouter(server)

	rec := searchPlaces(t, router, "via+roma", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.calls != 0 {
		t.Fatal("expected no call to Nominatim without a session")
	}
}

func TestSearchPlaces_RejectsAQueryTooShortToMeanAnything(t *testing.T) {
	server := newTestServer(t)
	fake := &fakeGeocodeClient{}
	server.Geocode = fake
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "password123")

	rec := searchPlaces(t, router, "vi", cookie)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.calls != 0 {
		t.Fatal("expected no call to Nominatim for a two-letter query")
	}
}

func TestSearchPlaces_ReportsNominatimBeingUnreachable(t *testing.T) {
	server := newTestServer(t)
	server.Geocode = &fakeGeocodeClient{err: context.DeadlineExceeded}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "password123")

	rec := searchPlaces(t, router, "via+roma", cookie)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}
