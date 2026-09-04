package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/mailer"
)

// mailSendTimeout è il tetto su un singolo invio. Vive nella goroutine,
// non nella richiesta HTTP: il partecipante ha già avuto la sua risposta.
const mailSendTimeout = 20 * time.Second

// mailSender restituisce il sender per questa richiesta: quello iniettato
// se c'è (i test), altrimenti uno costruito al volo dalle impostazioni
// salvate. Costruirlo per richiesta è ciò che permette all'admin di
// cambiare provider senza riavviare il container, come per il traduttore.
func (s *Server) mailSender(ctx context.Context) mailer.Sender {
	if s.Mail != nil {
		return s.Mail
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		// Senza impostazioni non si manda niente; un sender vuoto
		// restituisce ErrNotConfigured, che è l'esito giusto.
		log.Printf("mail: could not load settings: %v", err)
		return mailer.NewSMTPSender(mailer.Config{})
	}
	return mailer.NewSMTPSender(smtpConfigFrom(cfg))
}

// mailEnabled dice se una mail partirà davvero. Serve alle risposte che
// portano "mailQueued": promettere una mail che non partirà è peggio
// che non prometterla.
func (s *Server) mailEnabled(ctx context.Context) bool {
	if s.Mail != nil {
		return true
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return false
	}
	return smtpConfigFrom(cfg).Configured()
}

// sendMailAsync spedisce senza far aspettare la risposta HTTP. Il sender
// arriva risolto dal chiamante di proposito: costruirlo dentro la
// goroutine vorrebbe dire leggere le impostazioni con un context che la
// richiesta ha già chiuso.
//
// Nessun errore risale: una prenotazione, un annullamento e un invito
// riescono o falliscono per i loro motivi, mai per la posta.
// ErrNotConfigured non finisce nemmeno nei log — è l'app senza SMTP, che
// è una configurazione valida.
func (s *Server) sendMailAsync(sender mailer.Sender, m mailer.Message) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mailSendTimeout)
		defer cancel()

		err := sender.Send(ctx, m)
		if err == nil || errors.Is(err, mailer.ErrNotConfigured) {
			return
		}
		log.Printf("mail to %s (%q) failed: %v", m.To, m.Subject, err)
	}()
}

// publicBaseURL è la radice degli indirizzi che finiscono nelle mail,
// senza slash finale. Vince l'indirizzo pubblico configurato: chi riceve
// il link deve raggiungere l'app dal dominio dell'associazione, non da
// quello con cui il browser dell'admin sta navigando.
//
// Il ripiego sulla richiesta esiste perché una mail non ha un browser su
// cui contare, e vale per l'installazione locale che non ha configurato
// niente. Si fida di X-Forwarded-Proto perché il deploy previsto sta
// dietro il proprio reverse proxy; l'host invece resta quello della
// richiesta, e un'installazione esposta dovrebbe configurare l'indirizzo
// pubblico e non dipendere da questo ramo.
func (s *Server) publicBaseURL(r *http.Request) string {
	if s.Settings != nil {
		if cfg, err := s.Settings.Get(r.Context()); err == nil && cfg.PublicBaseURL != "" {
			return strings.TrimRight(cfg.PublicBaseURL, "/")
		}
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
	}
	return scheme + "://" + r.Host
}

// Gli indirizzi che finiscono nelle mail. Stanno insieme perché devono
// restare allineati alle rotte del frontend: cambiare un path in
// frontend/src/router/index.ts vuol dire cambiare qui.
//
// Né i token d'invito (hex) né i codici di prenotazione (lettere e cifre
// da un alfabeto senza caratteri ambigui) contengono caratteri da
// codificare, quindi la concatenazione basta e resta leggibile in una mail.

func inviteURL(base, token string) string {
	return base + "/invito/" + token
}

func bookingManageURL(base, code string) string {
	return base + "/prenotazione/" + code
}

func bookingScoreURL(base, code string) string {
	return base + "/prenotazione/" + code + "/punteggio"
}

func eventPublicURL(base string, eventID int64) string {
	return fmt.Sprintf("%s/events/%d", base, eventID)
}

// bookingMailDataFor raccoglie quello che le due mail di prenotazione
// devono dire. Ripercorre la stessa strada di toBookingDetailResponse
// perché le due superfici devono raccontare la stessa prenotazione: se
// la pagina dice "Catan #2", la mail non può dire "Catan".
func (s *Server) bookingMailDataFor(ctx context.Context, b events.Booking) (bookingMailData, error) {
	event, err := s.Events.GetEvent(ctx, b.EventID)
	if err != nil {
		return bookingMailData{}, err
	}
	eventGame, err := s.Events.GetEventGame(ctx, b.EventGameID)
	if err != nil {
		return bookingMailData{}, err
	}
	game, err := s.Games.GetGame(ctx, eventGame.GameID)
	if err != nil {
		return bookingMailData{}, err
	}
	copies, err := s.Events.CountEventGameCopies(ctx, b.EventID, eventGame.GameID)
	if err != nil {
		return bookingMailData{}, err
	}

	// Il numero della copia serve solo quando l'evento porta più copie di
	// questo gioco, esattamente come in ManageBookingView.
	label := game.Name
	if copies > 1 {
		label = fmt.Sprintf("%s #%d", game.Name, eventGame.CopyIndex)
	}

	return bookingMailData{
		ParticipantName:  b.ParticipantName,
		ParticipantEmail: b.ParticipantEmail,
		BookingCode:      b.BookingCode,
		GameLabel:        label,
		EventTitle:       event.Title,
		EventDate:        event.EventDate,
		StartTime:        event.StartTime,
		EventID:          b.EventID,
		SharedTable:      eventGame.Seats > 1,
	}, nil
}

// sendBookingConfirmation manda la conferma, o non fa niente se non c'è
// posta. Raccoglie i dati in modo sincrono — servono il context della
// richiesta e il database — e spedisce in modo asincrono.
//
// Un errore nel raccogliere i dati non risale: la prenotazione è già
// fatta, e non mandare una mail è meglio che rispondere con un errore
// per qualcosa che è andato bene.
//
// Deriva subito un context senza cancellazione (context.WithoutCancel) e
// riusa r con quello: un partecipante che perde il segnale subito dopo che
// la prenotazione è stata scritta non deve perdere anche la mail col
// codice, che è esattamente quello che gli servirebbe al posto della
// risposta HTTP che non è arrivata. Solo la cancellazione è tolta: una
// scadenza a monte (se mai aggiunta) resterebbe valida. mailEnabled,
// mailSender, publicBaseURL (via r) e bookingMailDataFor leggono tutte da
// qui, così l'intera raccolta — non solo l'invio — sopravvive alla
// disconnessione del client.
func (s *Server) sendBookingConfirmation(r *http.Request, b events.Booking) {
	r = r.WithContext(context.WithoutCancel(r.Context()))
	if !s.mailEnabled(r.Context()) {
		return
	}
	data, err := s.bookingMailDataFor(r.Context(), b)
	if err != nil {
		log.Printf("mail: could not gather booking %d data: %v", b.ID, err)
		return
	}
	base := s.publicBaseURL(r)
	s.sendMailAsync(s.mailSender(r.Context()), bookingConfirmationMail(
		data,
		bookingManageURL(base, b.BookingCode),
		bookingScoreURL(base, b.BookingCode),
	))
}

// sendBookingCancelled avvisa il partecipante. byAdmin cambia il testo,
// non il destinatario: la mail va sempre a chi aveva prenotato, ed è
// nel caso dell'admin che serve davvero — è l'unico modo in cui il
// partecipante scopre di non avere più il posto.
//
// Come sendBookingConfirmation, raccoglie con un context senza
// cancellazione: non è la richiesta che deve arrivare a chi disdice, è
// la mail a chi aveva prenotato.
func (s *Server) sendBookingCancelled(r *http.Request, b events.Booking, byAdmin bool) {
	r = r.WithContext(context.WithoutCancel(r.Context()))
	if !s.mailEnabled(r.Context()) {
		return
	}
	data, err := s.bookingMailDataFor(r.Context(), b)
	if err != nil {
		log.Printf("mail: could not gather cancelled booking %d data: %v", b.ID, err)
		return
	}
	s.sendMailAsync(s.mailSender(r.Context()), bookingCancelledMail(
		data,
		eventPublicURL(s.publicBaseURL(r), data.EventID),
		byAdmin,
	))
}
