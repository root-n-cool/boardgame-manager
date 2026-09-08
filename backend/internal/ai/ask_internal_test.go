package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Test di package interno: postRaw non è esportato, ma è qui che si
// decide davvero quale timeout si applica a una chiamata. Vive a parte
// da ask_test.go (che è ai_test, sulla superficie pubblica) apposta per
// poter chiamare postRaw direttamente.
func TestPostRaw_TimeoutArgumentIsNotShortenedByTheSharedClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// NewHTTPClient imposta HTTPClient.Timeout a 60s (requestTimeout, per
	// Translate). Se postRaw lasciasse vincere quel campo sul suo stesso
	// argomento timeout, questi 50ms non farebbero mai scadere la
	// richiesta e la chiamata tornerebbe solo dopo i 200ms del server.
	client := NewHTTPClient(srv.URL, "sk-test", "m")

	start := time.Now()
	_, err := client.postRaw(context.Background(), []byte(`{}`), 50*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("atteso un errore di timeout, nessun errore ricevuto")
	}
	if elapsed >= 150*time.Millisecond {
		t.Fatalf("la richiesta ha impiegato %v: il timeout passato a postRaw non è stato applicato", elapsed)
	}
}
