package events_test

import (
	"context"
	"database/sql"
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
	returned, err := store.ReturnLoan(context.Background(), loan.ID, &notes, nil)
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

	returned, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil)
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
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); err != nil {
		t.Fatalf("first return: %v", err)
	}

	_, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil)
	if !errors.Is(err, events.ErrLoanAlreadyReturned) {
		t.Fatalf("err = %v, want ErrLoanAlreadyReturned", err)
	}
}

func TestReturnLoanRefusesAnUnknownID(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.ReturnLoan(context.Background(), 999, nil, nil)
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

	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: il prestito doveva sparire in cascata", err)
	}
}

func TestUpdateEventRefusesToDropACopyOnLoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	// Una copia sola, e fuori in prestito: togliere il gioco dall'evento
	// vuol dire cancellare proprio quella, e non c'è nessuna copia libera
	// con cui soddisfare la richiesta.
	mustLend(t, store, event.ID, firstCopy(t, store, event.ID).ID, "Anna")

	_, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{},
	})
	if !errors.Is(err, events.ErrCopyOnLoan) {
		t.Fatalf("err = %v, want ErrCopyOnLoan", err)
	}

	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("copie = %d, want 1: la transazione doveva annullarsi", len(after))
	}
}

// TestUpdateEventDropsAnOnlyCopyEvenWithClosedHistory copre il caso in cui
// l'unica copia eliminabile porta con sé dello storico chiuso: il guard
// blocca solo i prestiti aperti, e qui non ce ne sono. La cancellazione
// procede — non c'è nessun'altra copia libera a cui appoggiarsi — e con
// essa sparisce anche la riga di game_loans, per ON DELETE CASCADE: è il
// prezzo di non avere un'alternativa migliore che cancellare l'evento
// intero, e questo test lo verifica invece di fermarsi al conteggio delle
// copie rimaste.
func TestUpdateEventDropsAnOnlyCopyEvenWithClosedHistory(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	loan := mustLend(t, store, event.ID, firstCopy(t, store, event.ID).ID, "Anna")
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); err != nil {
		t.Fatalf("return loan: %v", err)
	}

	// Il guard riguarda i prestiti aperti: uno chiuso non blocca niente.
	if _, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{},
	}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("copie = %d, want 0", len(after))
	}

	// La riga di game_loans deve essere sparita in cascata con la copia:
	// una seconda restituzione dà ErrNotFound (riga assente), non
	// ErrLoanAlreadyReturned (riga ancora lì, già chiusa).
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: la riga di game_loans doveva sparire con la copia", err)
	}
}

// TestUpdateEventPrefersDroppingACopyWithNoLoanHistory copre il caso che
// il branch conteneva senza accorgersene: fra due copie libere, una con
// storico chiuso e una senza, deve cadere quella senza — anche quando non
// è la copia con l'indice più alto, che è il criterio di scelta di
// default. Perdere zero righe di game_loans quando è possibile non è
// negoziabile.
func TestUpdateEventPrefersDroppingACopyWithNoLoanHistory(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
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
	// La copia #2, quella con l'indice più alto, è l'unica con storico: se
	// dropCopies seguisse solo "dalla più alta in giù" la eliminerebbe lei
	// per prima, perdendo la riga. La #1 non ha mai avuto un prestito.
	loan := mustLend(t, store, event.ID, copies[1].ID, "Anna")
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); err != nil {
		t.Fatalf("return loan: %v", err)
	}

	if _, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(after) != 1 || after[0].ID != copies[1].ID {
		t.Fatalf("copia sopravvissuta = %+v, want la #2 (quella con lo storico)", after)
	}

	// La riga del prestito chiuso è ancora lì: ErrLoanAlreadyReturned
	// (non ErrNotFound) prova che la copia #2 non è stata toccata.
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); !errors.Is(err, events.ErrLoanAlreadyReturned) {
		t.Fatalf("err = %v, want ErrLoanAlreadyReturned: lo storico doveva restare intatto", err)
	}
}

func TestUpdateEventShrinkingSpareTheCopyOnLoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
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
	mustLend(t, store, event.ID, copies[1].ID, "Anna")

	// Scendere a una copia si può: si sacrifica quella libera, non quella
	// che qualcuno ha in mano, così il registro dei prestiti resta vero.
	if _, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("copie = %d, want 1", len(after))
	}
	if after[0].ID != copies[1].ID {
		t.Fatalf("copia sopravvissuta = %d, want %d: doveva restare quella in prestito", after[0].ID, copies[1].ID)
	}
}

// mustMaterials scrive le voci del catalogo con SQL diretto: internal/events
// non importa internal/games, e nemmeno i suoi test devono farlo per una
// tabella di due colonne.
func mustMaterials(t *testing.T, conn *sql.DB, gameID int64, rows ...[2]any) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(rows))
	for i, r := range rows {
		res, err := conn.Exec(
			`INSERT INTO game_material (game_id, name, quantity, position) VALUES (?, ?, ?, ?)`,
			gameID, r[0], r[1], i)
		if err != nil {
			t.Fatalf("insert material %v: %v", r[0], err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestReturnLoanWithoutChecksWritesNoIssue(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	mustMaterials(t, conn, gameID, [2]any{"tessere", 72})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); err != nil {
		t.Fatalf("return: %v", err)
	}
	issues, err := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	if len(issues[loan.ID]) != 0 {
		t.Fatalf("senza checklist non si scrive niente, ho %+v", issues[loan.ID])
	}
}

func TestReturnLoanWithEverythingCheckedWritesNoIssue(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"tessere", 72}, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	checks := []events.MaterialCheck{
		{MaterialID: ids[0], Complete: true},
		{MaterialID: ids[1], Complete: true},
	}
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, checks); err != nil {
		t.Fatalf("return: %v", err)
	}
	issues, _ := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if len(issues[loan.ID]) != 0 {
		t.Fatalf("un controllo pulito non lascia righe, ho %+v", issues[loan.ID])
	}
}

func TestReturnLoanRecordsShortagesAndUncheckedRows(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID,
		[2]any{"tessere", 72}, [2]any{"carte", 40}, [2]any{"dadi", 5})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	returned := 35
	checks := []events.MaterialCheck{
		{MaterialID: ids[0], Complete: true},                       // tutte tornate
		{MaterialID: ids[1], Complete: false, Returned: &returned}, // 35 su 40
		// ids[2] non compare: non verificata
	}
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, checks); err != nil {
		t.Fatalf("return: %v", err)
	}

	issues, err := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	got := issues[loan.ID]
	if len(got) != 2 {
		t.Fatalf("volevo 2 righe di esito, ho %+v", got)
	}
	if got[0].Name != "carte" || got[0].Expected != 40 || got[0].Returned == nil || *got[0].Returned != 35 {
		t.Errorf("riga incompleta inattesa: %+v", got[0])
	}
	if got[1].Name != "dadi" || got[1].Returned != nil {
		t.Errorf("riga non verificata inattesa: %+v", got[1])
	}
}

func TestReturnLoanTreatsExtraPiecesAsComplete(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	// 41 su 40: capita di ritrovare il pezzo di un'altra scatola. Non manca
	// niente, quindi non è un esito da registrare.
	extra := 41
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil,
		[]events.MaterialCheck{{MaterialID: ids[0], Returned: &extra}}); err != nil {
		t.Fatalf("return: %v", err)
	}
	issues, _ := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if len(issues[loan.ID]) != 0 {
		t.Fatalf("non doveva restare niente, ho %+v", issues[loan.ID])
	}
}

func TestReturnLoanRejectsNegativeQuantity(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	negative := -1
	_, err := store.ReturnLoan(context.Background(), loan.ID, nil,
		[]events.MaterialCheck{{MaterialID: ids[0], Returned: &negative}})
	if !errors.Is(err, events.ErrMaterialCheckInvalid) {
		t.Fatalf("volevo ErrMaterialCheckInvalid, ho %v", err)
	}
	// E il prestito deve essere ancora aperto: o si chiude con il suo esito,
	// o non si chiude.
	open, err := store.ListLoansForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(open) != 1 || open[0].ReturnedAt != nil {
		t.Fatalf("il prestito non doveva chiudersi: %+v", open)
	}
}

func TestReturnLoanTwiceStillFails(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	mustMaterials(t, conn, gameID, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); err != nil {
		t.Fatalf("prima return: %v", err)
	}
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); !errors.Is(err, events.ErrLoanAlreadyReturned) {
		t.Fatalf("volevo ErrLoanAlreadyReturned, ho %v", err)
	}
}

// Gli id del catalogo cambiano solo quando una voce viene rinominata o
// tolta (ReplaceMaterials li conserva), ma se accade mentre una modale di
// riconsegna è aperta le spunte arrivano con id che non esistono più. Il
// prestito si chiude comunque e ogni voce risulta non verificata: è la
// direzione prudente dell'errore, l'opposto — dare per presente ciò che
// nessuno ha guardato — non deve essere possibile.
func TestReturnLoanWithStaleMaterialIDsRecordsEverythingUnverified(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"tessere", 72}, [2]any{"carte", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	returned := 35
	checks := []events.MaterialCheck{
		{MaterialID: ids[0] + 1000, Complete: true},
		{MaterialID: ids[1] + 1000, Returned: &returned},
	}
	closed, err := store.ReturnLoan(context.Background(), loan.ID, nil, checks)
	if err != nil {
		t.Fatalf("return: %v", err)
	}
	if closed.ReturnedAt == nil {
		t.Fatal("il prestito doveva chiudersi lo stesso")
	}

	issues, err := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	got := issues[loan.ID]
	if len(got) != 2 {
		t.Fatalf("volevo una riga per ogni voce del catalogo, ho %+v", got)
	}
	for _, iss := range got {
		if iss.Returned != nil {
			t.Errorf("%q doveva risultare non verificata, ho %d", iss.Name, *iss.Returned)
		}
	}
}
