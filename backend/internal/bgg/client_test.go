package bgg_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"boardgames-manager/internal/bgg"
)

func TestSearch_ParsesResultsAndSendsAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization header with test-token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<items>
	<item type="boardgame" id="13">
		<name type="primary" value="Catan"/>
		<name type="alternate" value="Die Siedler von Catan"/>
		<yearpublished value="1995"/>
	</item>
</items>`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	results, err := client.Search(context.Background(), "test-token", "catan")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "13" || results[0].Name != "Catan" || results[0].Year != 1995 {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestGetThing_ParsesDetailAndPicksPrimaryName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<items>
	<item type="boardgame" id="13">
		<image>https://example.com/catan.jpg</image>
		<name type="primary" value="Catan"/>
		<name type="alternate" value="Die Siedler von Catan"/>
		<description>A game about settling an island.</description>
		<yearpublished value="1995"/>
		<minplayers value="3"/>
		<maxplayers value="4"/>
		<playingtime value="90"/>
	</item>
</items>`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	detail, err := client.GetThing(context.Background(), "test-token", "13")
	if err != nil {
		t.Fatalf("get thing: %v", err)
	}
	if detail.Name != "Catan" {
		t.Fatalf("expected primary name Catan, got %q", detail.Name)
	}
	if detail.Description != "A game about settling an island." {
		t.Fatalf("unexpected description: %q", detail.Description)
	}
	if detail.Year != 1995 || detail.MinPlayers != 3 || detail.MaxPlayers != 4 || detail.PlayingTime != 90 {
		t.Fatalf("unexpected numeric fields: %+v", detail)
	}
	if detail.ImageURL != "https://example.com/catan.jpg" {
		t.Fatalf("unexpected image url: %q", detail.ImageURL)
	}
}

func TestSearch_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Search(context.Background(), "bad-token", "catan")
	if err == nil {
		t.Fatal("expected an error for non-200 status")
	}
}

func TestSearch_FallsBackToAlternateNameNotID(t *testing.T) {
	// BGG's /search lists plenty of items whose only <name> is an alternate
	// (fan expansions, localised editions). Falling back to the id turned
	// those rows into "134277 (2012)" in the picker.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<items>
	<item type="boardgame" id="134277">
		<name type="alternate" value="The 7 Wonders of Catan"/>
		<yearpublished value="2012"/>
	</item>
</items>`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	results, err := client.Search(context.Background(), "test-token", "catan")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if results[0].Name != "The 7 Wonders of Catan" {
		t.Fatalf("expected the alternate name, got %q", results[0].Name)
	}
}

func TestDetails_FetchesManyIDsWithStats(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<items>
	<item type="boardgame" id="13">
		<thumbnail>https://example.com/catan_t.png</thumbnail>
		<name type="primary" value="Catan"/>
		<yearpublished value="1995"/>
		<statistics><ratings><averageweight value="2.2809"/></ratings></statistics>
	</item>
	<item type="boardgame" id="822">
		<thumbnail>https://example.com/carcassonne_t.jpg</thumbnail>
		<name type="primary" value="Carcassonne"/>
		<yearpublished value="2000"/>
		<statistics><ratings><averageweight value="1.8839"/></ratings></statistics>
	</item>
</items>`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	details, err := client.Details(context.Background(), "test-token", []string{"13", "822"})
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if gotQuery.Get("id") != "13,822" {
		t.Fatalf("expected a single request for both ids, got id=%q", gotQuery.Get("id"))
	}
	if gotQuery.Get("stats") != "1" {
		t.Fatalf("averageweight only comes back with stats=1, got stats=%q", gotQuery.Get("stats"))
	}
	if len(details) != 2 {
		t.Fatalf("expected 2 details, got %d", len(details))
	}
	if details["13"].ThumbnailURL != "https://example.com/catan_t.png" {
		t.Fatalf("unexpected thumbnail: %q", details["13"].ThumbnailURL)
	}
	if details["13"].Weight != 2.2809 || details["822"].Weight != 1.8839 {
		t.Fatalf("unexpected weights: %v / %v", details["13"].Weight, details["822"].Weight)
	}
}

func TestDetails_NoIDsMakesNoRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no ids should mean no upstream call")
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	details, err := client.Details(context.Background(), "test-token", nil)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if len(details) != 0 {
		t.Fatalf("expected no details, got %d", len(details))
	}
}

func TestGetThing_ReadsThumbnailAndWeight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<items>
	<item type="boardgame" id="13">
		<image>https://example.com/catan.jpg</image>
		<thumbnail>https://example.com/catan_t.png</thumbnail>
		<name type="primary" value="Catan"/>
		<yearpublished value="1995"/>
		<statistics><ratings><averageweight value="2.2809"/></ratings></statistics>
	</item>
</items>`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	detail, err := client.GetThing(context.Background(), "test-token", "13")
	if err != nil {
		t.Fatalf("get thing: %v", err)
	}
	if detail.Weight != 2.2809 {
		t.Fatalf("unexpected weight: %v", detail.Weight)
	}
	if detail.ThumbnailURL != "https://example.com/catan_t.png" {
		t.Fatalf("unexpected thumbnail: %q", detail.ThumbnailURL)
	}
}
