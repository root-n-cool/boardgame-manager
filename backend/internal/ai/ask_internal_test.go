package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
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

// TestRetryable fissa cosa merita un altro tentativo. Il caso che ha
// motivato questa funzione è il terzo: sul manuale reale del club due
// pagine su quattro sono morte con "context deadline exceeded" — il
// provider non aveva risposto affatto — e il retry di allora, che
// guardava solo lo StatusError, non le ha riprovate nemmeno una volta.
// Erano due pagine perse in silenzio dall'indice.
func TestRetryable(t *testing.T) {
	// Un contesto già annullato serve al caso in cui a essere finito è il
	// NOSTRO contesto, non il tentativo.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{
			name: "rate limit: il provider passerà",
			ctx:  context.Background(),
			err:  &StatusError{Status: http.StatusTooManyRequests},
			want: true,
		},
		{
			name: "chiave sbagliata: darà lo stesso esito ogni volta",
			ctx:  context.Background(),
			err:  &StatusError{Status: http.StatusUnauthorized},
			want: false,
		},
		{
			// La forma esatta in cui l'errore arriva da http.Client.Do
			// quando scade il contesto del singolo tentativo.
			name: "tentativo scaduto: il provider non ha risposto, non ha risposto no",
			ctx:  context.Background(),
			err:  &url.Error{Op: "Post", URL: "https://example.invalid", Err: context.DeadlineExceeded},
			want: true,
		},
		{
			// Distinzione che conta: se è il contesto del CHIAMANTE a
			// essere finito, riprovare vorrebbe dire insistere su una
			// richiesta che non interessa più a nessuno — l'admin ha
			// chiuso la pagina, o un'altra pagina ha già scoperto che il
			// modello vision non è configurato e ha annullato tutto.
			name: "contesto del chiamante annullato: non c'è niente da riprovare",
			ctx:  cancelled,
			err:  &url.Error{Op: "Post", URL: "https://example.invalid", Err: context.DeadlineExceeded},
			want: false,
		},
		{
			// Un host sbagliato o un servizio spento non guariscono
			// riprovando: è lo stesso argomento del 4xx, e su trenta
			// pagine costerebbe trenta attese prima dell'errore che
			// l'admin deve vedere.
			name: "connessione rifiutata: è configurazione, non un guasto passeggero",
			ctx:  context.Background(),
			err:  &url.Error{Op: "Post", URL: "https://example.invalid", Err: errors.New("connect: connection refused")},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryable(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("retryable(%v) = %v, atteso %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestPostChatRetrying_RetriesATimedOutAttempt esercita il retry sul
// timeout da capo a fondo, non solo il predicato. Passa un timeout
// minuscolo invece di transcribeTimeout — postChatRetrying lo prende
// come argomento proprio per questo — così il test dimostra il
// comportamento vero in millisecondi invece che in minuti.
func TestPostChatRetrying_RetriesATimedOutAttempt(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			// Più lungo del timeout passato sotto: il primo tentativo
			// muore senza che il provider abbia risposto.
			time.Sleep(300 * time.Millisecond)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"## Preparazione\n\nMescola il mazzo."}}]}`)
	}))
	defer srv.Close()

	c := &HTTPClient{BaseURL: srv.URL, APIKey: "sk-test", Model: "m", HTTPClient: &http.Client{}}
	out, err := c.postChatRetrying(context.Background(), []byte(`{}`), 50*time.Millisecond)
	if err != nil {
		t.Fatalf("un tentativo scaduto va riprovato, non restituito: %v", err)
	}
	if !strings.Contains(out, "Mescola") {
		t.Fatalf("risposta inattesa: %q", out)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attesi 2 tentativi (lo scaduto più quello riuscito), fatti %d", got)
	}
}
