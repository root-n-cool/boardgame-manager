package manuals_test

import (
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

func TestChunk_KeepsShortPagesWhole(t *testing.T) {
	pages := []manuals.Page{{Number: 3, Text: "Turno del giocatore. Si pescano due carte."}}
	chunks := manuals.Chunk(pages)
	if len(chunks) != 1 {
		t.Fatalf("una pagina corta è un chunk solo, ottenuti %d", len(chunks))
	}
	if chunks[0].PageNumber != 3 || chunks[0].Seq != 0 {
		t.Fatalf("pagina/seq attesi 3/0, ottenuti %d/%d", chunks[0].PageNumber, chunks[0].Seq)
	}
}

func TestChunk_SplitsLongPagesWithoutBreakingSentences(t *testing.T) {
	// Una pagina lunga il triplo del massimo: deve uscire in più chunk,
	// ognuno sotto il limite, e nessuno deve cominciare a metà frase.
	sentence := "Ogni giocatore paga una moneta per ciascun edificio posseduto e ne verifica la produzione. "
	long := strings.Repeat(sentence, 40)
	chunks := manuals.Chunk([]manuals.Page{{Number: 4, Text: long}})

	if len(chunks) < 2 {
		t.Fatalf("attesi più chunk da una pagina lunga, ottenuti %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Text) > manuals.MaxChunkChars {
			t.Fatalf("chunk %d supera il limite: %d caratteri", i, len(c.Text))
		}
		if c.PageNumber != 4 {
			t.Fatalf("chunk %d ha perso il numero di pagina: %d", i, c.PageNumber)
		}
		if c.Seq != i {
			t.Fatalf("seq non progressivo: chunk %d ha seq %d", i, c.Seq)
		}
		first := strings.TrimSpace(c.Text)
		if first == "" {
			t.Fatalf("chunk %d è vuoto", i)
		}
		// Un chunk che comincia con una minuscola è una frase tagliata a
		// metà, cioè il difetto che la sovrapposizione deve evitare.
		if i > 0 && strings.ToLower(first[:1]) == first[:1] && strings.ToUpper(first[:1]) != first[:1] {
			t.Fatalf("chunk %d comincia a metà frase: %q", i, first[:40])
		}
	}
}

func TestChunk_OverlapsSoARuleOnTheBoundaryIsFindable(t *testing.T) {
	// La regola sta a cavallo del taglio: deve comparire intera in almeno
	// un chunk, altrimenti non la trova né il chunk prima né quello dopo.
	filler := strings.Repeat("Testo di riempimento del regolamento. ", 25)
	rule := "Se due giocatori sono in pareggio vince chi ha meno edifici demoliti."
	chunks := manuals.Chunk([]manuals.Page{{Number: 8, Text: filler + rule + " " + filler}})

	found := false
	for _, c := range chunks {
		if strings.Contains(c.Text, rule) {
			found = true
		}
	}
	if !found {
		t.Fatal("la regola a cavallo del taglio non compare intera in nessun chunk")
	}
}

func TestChunk_IsDeterministic(t *testing.T) {
	pages := []manuals.Page{{Number: 1, Text: strings.Repeat("Una frase del manuale. ", 120)}}
	a := manuals.Chunk(pages)
	b := manuals.Chunk(pages)
	if len(a) != len(b) {
		t.Fatalf("due esecuzioni danno %d e %d chunk", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("chunk %d differisce fra due esecuzioni", i)
		}
	}
}

func TestChunk_SkipsEmptyPages(t *testing.T) {
	chunks := manuals.Chunk([]manuals.Page{
		{Number: 1, Text: "   \n  "},
		{Number: 2, Text: "Contenuto vero."},
	})
	if len(chunks) != 1 || chunks[0].PageNumber != 2 {
		t.Fatalf("una pagina vuota non produce chunk; ottenuti %d", len(chunks))
	}
}

func TestDetectHeading(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Fase di Upkeep\nOgni giocatore paga una moneta per ogni edificio.", "Fase di Upkeep"},
		{"CONTEGGIO DEI PUNTI\nOgni edificio vale i punti stampati.", "CONTEGGIO DEI PUNTI"},
		// Una prima riga che è già una frase compiuta non è un titolo.
		{"La partita termina quando la pila di pesca si esaurisce e non è possibile pescare.", ""},
		// Troppo lunga per essere un titolo.
		{strings.Repeat("parola ", 20) + "\naltro testo", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := manuals.DetectHeading(c.in); got != c.want {
			t.Fatalf("DetectHeading(%.30q) = %q, atteso %q", c.in, got, c.want)
		}
	}
}
