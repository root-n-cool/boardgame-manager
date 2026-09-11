package manuals_test

import (
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

// Il testo che ExtractText tira fuori da un PDF impaginato non ha a capo
// per frase: un'intera pagina può essere una riga sola (sul manuale di
// Ticket to Ride la riga più lunga era di 8.666 caratteri). Il modello che
// segmenta ci mette allora il titolo IN TESTA a quella riga invece che su
// una riga propria, e il controllo di fedeltà — che scarta l'intera riga
// di titolo — conta come "titolo" migliaia di caratteri di testo vero e
// dichiara inaffidabile una lettura perfettamente fedele. ReflowSentences
// dà al modello righe corte su cui appoggiare i titoli.
func TestReflowSentences_BreaksAfterSentenceEnd(t *testing.T) {
	in := "Mettete la mappa al centro del tavolo. Ogni giocatore prende 45 vagoni. Siete pronti."
	want := "Mettete la mappa al centro del tavolo.\nOgni giocatore prende 45 vagoni.\nSiete pronti."
	if got := manuals.ReflowSentences(in); got != want {
		t.Fatalf("atteso:\n%q\nottenuto:\n%q", want, got)
	}
}

func TestReflowSentences_BreaksAfterQuestionExclamationAndColon(t *testing.T) {
	in := "Chi vince? Il giocatore con più punti! I punti si contano così: Ogni linea vale."
	want := "Chi vince?\nIl giocatore con più punti!\nI punti si contano così:\nOgni linea vale."
	if got := manuals.ReflowSentences(in); got != want {
		t.Fatalf("atteso:\n%q\nottenuto:\n%q", want, got)
	}
}

// Una numerazione ("1. Pescare carte") non è una fine di frase: spezzare
// lì staccherebbe il numero dalla voce che introduce.
func TestReflowSentences_DoesNotBreakAfterANumberedListMarker(t *testing.T) {
	in := "Un giocatore deve fare una di queste azioni: 1. Pescare carte Carrozza. 2. Controllare una Linea."
	got := manuals.ReflowSentences(in)
	if strings.Contains(got, "1.\n") || strings.Contains(got, "2.\n") {
		t.Fatalf("la numerazione non deve andare a capo da sola: %q", got)
	}
}

// Le abbreviazioni comuni di un regolamento ("pag. 7", "es. quando",
// "n. 3") finiscono con un punto senza chiudere la frase.
func TestReflowSentences_DoesNotBreakAfterCommonAbbreviations(t *testing.T) {
	cases := []string{
		"Vedi pag. 7 per i dettagli.",
		"Per es. Quando peschi una Locomotiva.",
		"La carta n. 3 vale doppio.",
	}
	for _, in := range cases {
		got := manuals.ReflowSentences(in)
		if strings.Count(got, "\n") != 0 {
			t.Fatalf("%q non deve essere spezzato: %q", in, got)
		}
	}
}

// Un punto dentro un'iniziale puntata o un dominio non chiude una frase.
func TestReflowSentences_DoesNotBreakInsideNamesAndDomains(t *testing.T) {
	in := "Creato da Alan R. Moon. Visitate www.daysofwonder.com per altre mappe."
	want := "Creato da Alan R. Moon.\nVisitate www.daysofwonder.com per altre mappe."
	if got := manuals.ReflowSentences(in); got != want {
		t.Fatalf("atteso:\n%q\nottenuto:\n%q", want, got)
	}
}

// Il reflow tocca SOLO gli spazi che seguono una fine di frase: nessun
// carattere aggiunto, nessuno tolto. È quel che permette di lasciare
// intatti il controllo di fedeltà (che normalizza gli spazi bianchi) e le
// ancore di pagina, e va verificato come proprietà, non solo sugli esempi.
func TestReflowSentences_PreservesEveryNonSpaceCharacter(t *testing.T) {
	in := "Prima frase. Seconda frase? Terza: con due punti! Fine.\n\nAltro paragrafo."
	if got, want := strings.Join(strings.Fields(manuals.ReflowSentences(in)), " "),
		strings.Join(strings.Fields(in), " "); got != want {
		t.Fatalf("il testo normalizzato è cambiato:\natteso:  %q\nottenuto: %q", want, got)
	}
}

// Le righe già spezzate (un .txt, o un PDF che gli a capo ce li ha) non
// devono raddoppiare gli a capo né perdere le righe vuote fra paragrafi:
// chunk.go taglia sui paragrafi e quella riga vuota è informazione.
func TestReflowSentences_KeepsExistingLineStructure(t *testing.T) {
	in := "## Preparazione\n\nMettete la mappa.\nOgni giocatore prende 45 vagoni.\n\nSiete pronti."
	if got := manuals.ReflowSentences(in); got != in {
		t.Fatalf("un testo già spezzato non deve cambiare:\natteso:\n%q\nottenuto:\n%q", in, got)
	}
}

// Il caso vero, in piccolo: una pagina su una riga sola diventa più righe,
// e nessuna di esse resta lunga quanto la pagina intera.
func TestReflowSentences_TurnsAOneLinePageIntoManyLines(t *testing.T) {
	page := "Ticket to Ride è un'avventura ferroviaria. " +
		"I giocatori competono per unire le città. " +
		"Vince chi fa più punti. " +
		"Il gioco dura 30-60 minuti."
	got := manuals.ReflowSentences(page)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("attese 4 righe, ottenute %d: %q", len(lines), got)
	}
	for _, l := range lines {
		if len(l) >= len(page) {
			t.Fatalf("una riga è lunga quanto la pagina intera: %q", l)
		}
	}
}
