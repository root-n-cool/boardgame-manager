package manuals_test

import (
	"fmt"
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

func TestParseSections_KeepsThePreambleAndEveryHeading(t *testing.T) {
	md := "Un gioco per 2-5 giocatori.\n\n" +
		"## Preparazione\nSi distribuiscono cinque carte.\n\n" +
		"## Fase di Upkeep\nOgni giocatore paga una moneta.\n\n" +
		"### Eccezione\nChi non può pagare demolisce.\n"

	got := manuals.ParseSections(md)
	if len(got) != 4 {
		t.Fatalf("attese 4 sezioni (preambolo + 3 titoli), ottenute %d: %+v", len(got), got)
	}
	// Il preambolo è una sezione senza titolo, non un errore: molti
	// regolamenti aprono con un paragrafo introduttivo.
	if got[0].Heading != "" || !strings.Contains(got[0].Body, "2-5 giocatori") {
		t.Fatalf("il preambolo è andato perso: %+v", got[0])
	}
	if got[1].Heading != "Preparazione" || got[3].Heading != "Eccezione" {
		t.Fatalf("titoli sbagliati: %q, %q", got[1].Heading, got[3].Heading)
	}
	// L'offset deve puntare nel markdown originale: è ciò che permette di
	// risalire alla pagina del PDF in cui la sezione comincia.
	if md[got[1].Offset:got[1].Offset+3] != "Si " {
		t.Fatalf("offset della sezione 1 sbagliato: punta a %q", md[got[1].Offset:got[1].Offset+10])
	}
}

func TestChunkSections_CarriesTheHeadingOfItsOwnSection(t *testing.T) {
	// Due sezioni, la prima abbastanza lunga da spezzarsi: ogni chunk deve
	// portare il titolo della PROPRIA sezione, non il primo del documento.
	long := strings.Repeat("Ogni giocatore paga una moneta per edificio. ", 40)
	sections := []manuals.Section{
		{Heading: "Fase di Upkeep", Body: long, Offset: 0},
		{Heading: "Fine partita", Body: "La partita termina subito.", Offset: len(long) + 100},
	}

	chunks := manuals.ChunkSections(sections)
	if len(chunks) < 3 {
		t.Fatalf("attesi più chunk dalla prima sezione più uno dalla seconda, ottenuti %d", len(chunks))
	}
	last := chunks[len(chunks)-1]
	if last.Heading != "Fine partita" {
		t.Fatalf("l'ultimo chunk deve portare il titolo della sua sezione, porta %q", last.Heading)
	}
	if chunks[0].Heading != "Fase di Upkeep" {
		t.Fatalf("il primo chunk porta %q", chunks[0].Heading)
	}
	// Seq è progressivo su tutto il documento: due sezioni non ripartono da 0,
	// altrimenti "il chunk vicino" (seq ± 1) attraverserebbe le sezioni a caso.
	for i, c := range chunks {
		if c.Seq != i {
			t.Fatalf("seq non progressivo sul documento: chunk %d ha seq %d", i, c.Seq)
		}
	}
}

func TestChunkSections_OffsetAdvancesWithEachChunkInsideASection(t *testing.T) {
	// Una sezione lunga abbastanza da produrre più chunk: l'Offset di ogni
	// chunk deve avanzare col chunk (puntare a dove comincia IL SUO testo
	// nel documento originale), non restare fermo all'inizio della sezione.
	// Costruiamo il corpo con frasi numerate distinguibili, così possiamo
	// verificare che l'Offset di ogni chunk coincida davvero con la
	// posizione, nel body originale, del testo che quel chunk contiene.
	var body strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&body, "Regola numero %d riguarda la produzione di risorse degli edifici. ", i)
	}
	sectionOffset := 1000
	sections := []manuals.Section{
		{Heading: "Produzione", Body: body.String(), Offset: sectionOffset},
	}

	chunks := manuals.ChunkSections(sections)
	if len(chunks) < 3 {
		t.Fatalf("fixture sbagliata: attesi almeno 3 chunk per esercitare l'avanzamento, ottenuti %d", len(chunks))
	}

	prevOffset := -1
	for i, c := range chunks {
		if c.Offset <= prevOffset {
			t.Fatalf("chunk %d: Offset %d non avanza rispetto al precedente %d", i, c.Offset, prevOffset)
		}
		// L'offset deve essere relativo al documento originale (Offset di
		// sezione + posizione nel body), non alla sezione da sola: deve
		// quindi cadere oltre l'inizio della sezione...
		if c.Offset < sectionOffset {
			t.Fatalf("chunk %d: Offset %d cade prima dell'inizio della sezione (%d)", i, c.Offset, sectionOffset)
		}
		// ...e il testo del documento a partire da quell'offset deve
		// combaciare con l'inizio del testo del chunk: è la prova che
		// l'offset punta davvero a dove comincia QUEL chunk, e non è
		// rimasto fermo a sectionOffset per tutti i chunk della sezione.
		localOffset := c.Offset - sectionOffset
		text := strings.TrimSpace(c.Text)
		prefixLen := 20
		if len(text) < prefixLen {
			prefixLen = len(text)
		}
		wantPrefix := text[:prefixLen]
		gotPrefix := body.String()[localOffset : localOffset+prefixLen]
		if gotPrefix != wantPrefix {
			t.Fatalf("chunk %d: il documento a Offset-sectionOffset=%d comincia con %q, il chunk con %q",
				i, localOffset, gotPrefix, wantPrefix)
		}
		prevOffset = c.Offset
	}
}
