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

// TestRetryDelay verifica la politica di attesa fra due tentativi di
// trascrizione senza dormire davvero: retryDelay è una funzione pura
// proprio per questo — la scala dei tempi veri (secondi) renderebbe il
// test della sola aritmetica lento senza aggiungere niente.
func TestRetryDelay(t *testing.T) {
	cases := []struct {
		name    string
		attempt int
		err     *StatusError
		want    time.Duration
	}{
		{
			name:    "primo tentativo fallito: attesa di base",
			attempt: 0,
			err:     &StatusError{Status: http.StatusTooManyRequests},
			want:    transcribeBackoff[0],
		},
		{
			name:    "secondo tentativo fallito: attesa più lunga",
			attempt: 1,
			err:     &StatusError{Status: http.StatusServiceUnavailable},
			want:    transcribeBackoff[1],
		},
		{
			// Un provider che dice quanto aspettare ne sa più di noi: il
			// suo valore vince sull'attesa di base, in entrambe le
			// direzioni.
			name:    "Retry-After più lungo dell'attesa di base vince",
			attempt: 0,
			err:     &StatusError{Status: http.StatusTooManyRequests, RetryAfter: 7 * time.Second},
			want:    7 * time.Second,
		},
		{
			name:    "Retry-After più corto dell'attesa di base vince ugualmente",
			attempt: 1,
			err:     &StatusError{Status: http.StatusTooManyRequests, RetryAfter: 200 * time.Millisecond},
			want:    200 * time.Millisecond,
		},
		{
			// Il tetto esiste perché Retry-After è un numero che arriva
			// dalla rete: un provider confuso (o ostile) che chiede
			// un'ora terrebbe occupata la request HTTP dell'admin fino al
			// timeout, con l'indicizzazione ferma e nessuna spiegazione.
			name:    "Retry-After assurdo è limitato dal tetto",
			attempt: 0,
			err:     &StatusError{Status: http.StatusTooManyRequests, RetryAfter: time.Hour},
			want:    maxRetryAfter,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryDelay(tc.attempt, tc.err); got != tc.want {
				t.Fatalf("retryDelay(%d, %+v) = %v, atteso %v", tc.attempt, tc.err, got, tc.want)
			}
		})
	}
}

// TestStatusErrorTemporary fissa quali status vale la pena riprovare. La
// distinzione è il cuore del retry: riprovare un 401 (chiave sbagliata) o
// un 400 (richiesta malformata) brucia tempo e token per ottenere
// esattamente lo stesso esito, e su un manuale di trenta pagine lo fa
// trenta volte.
func TestStatusErrorTemporary(t *testing.T) {
	temporary := []int{http.StatusTooManyRequests, 500, 502, 503, 504}
	permanent := []int{400, 401, 403, 404, 422}
	for _, status := range temporary {
		if !(&StatusError{Status: status}).Temporary() {
			t.Errorf("lo status %d è transitorio e va riprovato", status)
		}
	}
	for _, status := range permanent {
		if (&StatusError{Status: status}).Temporary() {
			t.Errorf("lo status %d è definitivo: riprovarlo spreca token per lo stesso esito", status)
		}
	}
}
