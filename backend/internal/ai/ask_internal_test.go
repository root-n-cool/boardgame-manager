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
//
// Il test va nella direzione che discrimina davvero il fix: il client
// condiviso ha un Timeout CORTO (50ms, cioè più corto della singola
// chiamata), il timeout passato a postRaw è LUNGO (2s), e il server
// risponde a metà strada (200ms). Con context.WithDeadline la scadenza
// più vicina vince sempre, quindi una versione con un argomento-timeout
// corto contro un client-Timeout lungo (come nella prima stesura di
// questo test) passa comunque, fix o non fix: non discrimina niente. Qui
// invece, se clientCopy.Timeout = 0 non azzerasse il campo del client
// condiviso, la richiesta scadrebbe a 50ms e la chiamata fallirebbe: il
// successo prova che è l'argomento a governare, non il campo fisso.
func TestPostRaw_TimeoutArgumentIsNotShortenedByTheSharedClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "sk-test", "m")
	client.HTTPClient.Timeout = 50 * time.Millisecond

	_, err := client.postRaw(context.Background(), []byte(`{}`), 2*time.Second)
	if err != nil {
		t.Fatalf("atteso successo (il timeout dell'argomento è largo), ottenuto errore: %v", err)
	}
}
