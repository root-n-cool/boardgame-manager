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

// filesJSON è una risposta ridotta di api.geekdo.com/api/files: due file
// con lingua e uno "(neutral)", che nel JSON reale arriva come null.
const filesJSON = `{"files":[
	{"filepageid":"142767","fileid":"218863","filename":"S-CAR v7.4.pdf","size":"551718","title":"Annotated Rules","numpositive":"330","language":"English","languageid":"2184","href":"\/filepage\/142767\/annotated-rules"},
	{"filepageid":"143911","fileid":"183457","filename":"Carcopedia.pdf","size":"18083546","title":"Carcopedia","numpositive":"13","language":"Italian","languageid":"2193","href":"\/filepage\/143911\/carcopedia"},
	{"filepageid":"9","fileid":"9","filename":"tiles.zip","size":"12","title":"Tiles","numpositive":null,"language":null,"href":"\/filepage\/9\/tiles"}
],"config":{"endpage":1,"numitems":3}}`

func TestFiles_ParsesEntriesAndBuildsAbsolutePageURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(filesJSON))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{FilesBaseURL: server.URL, HTTPClient: server.Client()}
	files, err := client.Files(context.Background(), "822", "")
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	first := files[0]
	if first.Title != "Annotated Rules" || first.Filename != "S-CAR v7.4.pdf" {
		t.Fatalf("unexpected title/filename: %+v", first)
	}
	if first.Language != "English" || first.Positive != 330 || first.SizeBytes != 551718 {
		t.Fatalf("unexpected metadata: %+v", first)
	}
	if first.PageURL != "https://boardgamegeek.com/filepage/142767/annotated-rules" {
		t.Fatalf("expected absolute page URL, got %q", first.PageURL)
	}
	if first.LanguageID != "2184" {
		t.Fatalf("expected language id 2184, got %q", first.LanguageID)
	}
}

func TestFiles_NullLanguageAndVotesBecomeEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(filesJSON))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{FilesBaseURL: server.URL, HTTPClient: server.Client()}
	files, err := client.Files(context.Background(), "822", "")
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	neutral := files[2]
	if neutral.Language != "" {
		t.Fatalf("expected empty language for null, got %q", neutral.Language)
	}
	if neutral.Positive != 0 {
		t.Fatalf("expected 0 votes for null, got %d", neutral.Positive)
	}
	if neutral.LanguageID != "" {
		t.Fatalf("expected empty language id for null, got %q", neutral.LanguageID)
	}
}

func TestFiles_SendsObjectIDLanguageAndHotSort(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{"files":[],"config":{"numitems":0}}`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{FilesBaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.Files(context.Background(), "822", "2193"); err != nil {
		t.Fatalf("files: %v", err)
	}
	if got.Get("objectid") != "822" {
		t.Errorf("expected objectid 822, got %q", got.Get("objectid"))
	}
	if got.Get("objecttype") != "thing" {
		t.Errorf("expected objecttype thing, got %q", got.Get("objecttype"))
	}
	if got.Get("languageid") != "2193" {
		t.Errorf("expected languageid 2193, got %q", got.Get("languageid"))
	}
	if got.Get("sort") != "hot" {
		t.Errorf("expected sort hot (the only ordering BGG honours), got %q", got.Get("sort"))
	}
}

func TestFiles_OmitsLanguageIDWhenEmpty(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{"files":[],"config":{"numitems":0}}`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{FilesBaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.Files(context.Background(), "822", ""); err != nil {
		t.Fatalf("files: %v", err)
	}
	if _, present := got["languageid"]; present {
		t.Errorf("expected no languageid param, got %q", got.Get("languageid"))
	}
}

func TestFiles_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("blocked"))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{FilesBaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.Files(context.Background(), "822", ""); err == nil {
		t.Fatal("expected error on 403")
	}
}
