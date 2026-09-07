package events_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/events"
)

// mustLend apre un prestito su una copia, per i test che partono da lì.
func mustLend(t *testing.T, store *events.Store, eventID, eventGameID int64, name string) events.Loan {
	t.Helper()
	loan, err := store.LendCopy(context.Background(), eventID, events.LoanInput{
		EventGameID: eventGameID, BorrowerName: name, BorrowerPhone: "3331234567",
	})
	if err != nil {
		t.Fatalf("lend to %q: %v", name, err)
	}
	return loan
}

// firstCopy è la copia su cui girano quasi tutti i test dei prestiti.
func firstCopy(t *testing.T, store *events.Store, eventID int64) events.EventGame {
	t.Helper()
	copies, err := store.ListEventGames(context.Background(), eventID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(copies) == 0 {
		t.Fatal("l'evento non ha copie")
	}
	return copies[0]
}

func TestLendCopyOpensALoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)

	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	if loan.BorrowerName != "Anna" {
		t.Errorf("borrower = %q, want Anna", loan.BorrowerName)
	}
	if loan.ReturnedAt != nil {
		t.Error("un prestito appena aperto non può essere già restituito")
	}
	if loan.LentAt.IsZero() {
		t.Error("lentAt vuoto: la data di inizio la mette il database")
	}
	if loan.BookingID != nil {
		t.Error("bookingID valorizzato su un prestito senza prenotazione")
	}
}

func TestLendCopyTrimsAndRequiresBorrower(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)

	for _, tc := range []struct{ name, borrower, phone string }{
		{"nome vuoto", "", "3331234567"},
		{"nome di spazi", "   ", "3331234567"},
		{"telefono vuoto", "Anna", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
				EventGameID: copy.ID, BorrowerName: tc.borrower, BorrowerPhone: tc.phone,
			})
			if !errors.Is(err, events.ErrBorrowerRequired) {
				t.Fatalf("err = %v, want ErrBorrowerRequired", err)
			}
		})
	}
}

func TestLendCopyRefusesASecondOpenLoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	mustLend(t, store, event.ID, copy.ID, "Anna")

	_, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copy.ID, BorrowerName: "Bruno", BorrowerPhone: "3339999999",
	})
	if !errors.Is(err, events.ErrCopyAlreadyOut) {
		t.Fatalf("err = %v, want ErrCopyAlreadyOut", err)
	}
}

func TestLendCopyRefusesACopyOfAnotherEvent(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	mine := mustCreateEvent(t, store, "Mia", "2030-01-01", "21:00", gameID)
	other := mustCreateEvent(t, store, "Altra", "2030-02-01", "21:00", gameID)
	otherCopy := firstCopy(t, store, other.ID)

	_, err := store.LendCopy(context.Background(), mine.ID, events.LoanInput{
		EventGameID: otherCopy.ID, BorrowerName: "Anna", BorrowerPhone: "3331234567",
	})
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestReturnLoanClosesItAndFreesTheCopy(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	notes := "manca una tessera"
	returned, err := store.ReturnLoan(context.Background(), loan.ID, &notes)
	if err != nil {
		t.Fatalf("return loan: %v", err)
	}
	if returned.ReturnedAt == nil {
		t.Fatal("returnedAt vuoto dopo la restituzione")
	}
	if returned.Notes == nil || *returned.Notes != notes {
		t.Errorf("notes = %v, want %q", returned.Notes, notes)
	}

	// La copia è tornata libera: si può riprestare, e lo storico resta.
	second := mustLend(t, store, event.ID, copy.ID, "Bruno")
	if second.ID == loan.ID {
		t.Fatal("il secondo prestito ha riusato la riga del primo")
	}
	all, err := store.ListLoansForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("prestiti = %d, want 2", len(all))
	}
}

func TestReturnLoanKeepsNotesWhenNoneAreSent(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	atPickup := "scatola già ammaccata"
	loan, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copy.ID, BorrowerName: "Anna", BorrowerPhone: "3331234567",
		Notes: &atPickup,
	})
	if err != nil {
		t.Fatalf("lend: %v", err)
	}

	returned, err := store.ReturnLoan(context.Background(), loan.ID, nil)
	if err != nil {
		t.Fatalf("return loan: %v", err)
	}
	if returned.Notes == nil || *returned.Notes != atPickup {
		t.Fatalf("notes = %v, want %q: un nil non deve cancellare quello che c'era", returned.Notes, atPickup)
	}
}

func TestReturnLoanRefusesTwice(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil); err != nil {
		t.Fatalf("first return: %v", err)
	}

	_, err := store.ReturnLoan(context.Background(), loan.ID, nil)
	if !errors.Is(err, events.ErrLoanAlreadyReturned) {
		t.Fatalf("err = %v, want ErrLoanAlreadyReturned", err)
	}
}

func TestReturnLoanRefusesAnUnknownID(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.ReturnLoan(context.Background(), 999, nil)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLendCopyFromABooking(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Wingspan")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	if err := store.TestInsertBooking(event.ID, copy.ID, events.BookingStatusActive); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	bookings, err := store.ListBookingsForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	bookingID := bookings[0].ID

	loan, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copy.ID, BookingID: &bookingID,
		BorrowerName: "Anna", BorrowerPhone: "3331234567",
	})
	if err != nil {
		t.Fatalf("lend from booking: %v", err)
	}
	if loan.BookingID == nil || *loan.BookingID != bookingID {
		t.Fatalf("bookingID = %v, want %d", loan.BookingID, bookingID)
	}
}

func TestLendCopyRefusesABookingOnAnotherCopy(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Wingspan")
	event, err := store.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if err := store.TestInsertBooking(event.ID, copies[0].ID, events.BookingStatusActive); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	bookings, err := store.ListBookingsForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	bookingID := bookings[0].ID

	// La prenotazione è sulla copia #1: agganciarla al prestito della #2
	// renderebbe il registro una bugia.
	_, err = store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copies[1].ID, BookingID: &bookingID,
		BorrowerName: "Anna", BorrowerPhone: "3331234567",
	})
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListLoansForEventCarriesTheGameLabel(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	mustLend(t, store, event.ID, copy.ID, "Anna")

	loans, err := store.ListLoansForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(loans) != 1 {
		t.Fatalf("prestiti = %d, want 1", len(loans))
	}
	if loans[0].GameName != "Carcassonne" {
		t.Errorf("gameName = %q, want Carcassonne", loans[0].GameName)
	}
	if loans[0].CopyIndex != 1 {
		t.Errorf("copyIndex = %d, want 1", loans[0].CopyIndex)
	}
	if loans[0].GameID != gameID {
		t.Errorf("gameID = %d, want %d", loans[0].GameID, gameID)
	}
}

func TestListLoansForEventIgnoresOtherEvents(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	mine := mustCreateEvent(t, store, "Mia", "2030-01-01", "21:00", gameID)
	other := mustCreateEvent(t, store, "Altra", "2030-02-01", "21:00", gameID)
	mustLend(t, store, other.ID, firstCopy(t, store, other.ID).ID, "Bruno")

	loans, err := store.ListLoansForEvent(context.Background(), mine.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(loans) != 0 {
		t.Fatalf("prestiti = %d, want 0", len(loans))
	}
}

func TestDeleteEventRemovesItsLoans(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	loan := mustLend(t, store, event.ID, firstCopy(t, store, event.ID).ID, "Anna")

	if err := store.DeleteEvent(context.Background(), event.ID); err != nil {
		t.Fatalf("delete event: %v", err)
	}

	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil); !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: il prestito doveva sparire in cascata", err)
	}
}
