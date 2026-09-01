package bgg_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
