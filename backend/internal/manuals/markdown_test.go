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

func TestChunkSections_OffsetPointsExactlyAtTheChunkTextEvenWithRepeatedSentences(t *testing.T) {
	// La stessa frase ripetuta identica è comune in un regolamento ("Ogni
	// giocatore pesca una carta." può comparire più volte). Se l'Offset
	// venisse recuperato cercando il testo del chunk nel documento con
	// strings.Index, la ripetizione rende il corpo periodico: qualunque
	// posizione allineata al periodo contiene un testo byte-identico al
	// chunk, quindi il solo controllo "md[Offset:Offset+len(Text)] ==
	// Text" non basta a scoprire un offset sbagliato — è vero per
	// costruzione anche nel punto sbagliato. La prova che lo scopre è che
	// con strings.Index(s.Body[cursor:], text) il cursore avanza di
	// idx+1 (pochi byte) invece che della lunghezza del chunk: gli offset
	// restano quindi ammassati vicino all'inizio invece di avanzare di
	// circa (len(chunk) - ChunkOverlapChars) a ogni passo, come deve fare
	// un offset tracciato per costruzione.
	//
	// Verificato: con strings.Index(s.Body[cursor:], part) al posto del
	// tracciamento per costruzione, questo test torna rosso (2 chunk su 3
	// avanzano di ~55 byte invece degli ~889 attesi — vedi il report per
	// l'output completo). Con l'implementazione corretta è verde.
	sentence := "Ogni giocatore pesca una carta e la mostra agli altri. "
	md := "## Pesca\n" + strings.Repeat(sentence, 40)

	sections := manuals.ParseSections(md)
	chunks := manuals.ChunkSections(sections)
	if len(chunks) < 3 {
		t.Fatalf("fixture sbagliata: attesi almeno 3 chunk, ottenuti %d", len(chunks))
	}

	prevOffset := -1
	prevLen := 0
	for i, c := range chunks {
		if c.Offset < 0 || c.Offset+len(c.Text) > len(md) {
			t.Fatalf("chunk %d: Offset %d fuori dai limiti del documento (lungo %d)", i, c.Offset, len(md))
		}
		if got := md[c.Offset : c.Offset+len(c.Text)]; got != c.Text {
			t.Fatalf("chunk %d: md[Offset:Offset+len(Text)] = %q, atteso testo del chunk %q", i, got, c.Text)
		}
		if i > 0 {
			// La sovrapposizione (ChunkOverlapChars) è l'unico motivo per
			// cui un chunk può cominciare prima della fine "netta" del
			// precedente: un margine di sicurezza di 300 byte assorbe la
			// variazione dovuta al riallineamento sul confine di frase,
			// molto meno dei ~55 byte di avanzamento che produce la
			// versione rotta.
			minAdvance := prevLen - manuals.ChunkOverlapChars - 300
			advance := c.Offset - prevOffset
			if advance < minAdvance {
				t.Fatalf("chunk %d: Offset avanza solo di %d byte dal precedente (atteso almeno %d): "+
					"un offset ricercato a posteriori su testo ripetuto resta ammassato vicino all'inizio "+
					"invece di avanzare col chunk", i, advance, minAdvance)
			}
		}
		prevOffset = c.Offset
		prevLen = len(c.Text)
	}
}

func TestParseSections_IgnoresHeadingLookalikesInsideFencedCodeBlocks(t *testing.T) {
	// Un "#" dentro un blocco di codice recintato non è un titolo: è
	// contenuto del blocco. Senza tener conto del fence, ParseSections
	// spezzerebbe la sezione nel punto sbagliato, in mezzo a un blocco che
	// l'autore intendeva come testo letterale.
	md := "## Titolo vero\n" +
		"Testo introduttivo.\n\n" +
		"```\n# non è un titolo\naltro testo nel blocco\n```\n\n" +
		"## Altro titolo vero\nCorpo.\n"

	got := manuals.ParseSections(md)
	var headings []string
	for _, s := range got {
		if s.Heading != "" {
			headings = append(headings, s.Heading)
		}
	}
	if len(headings) != 2 || headings[0] != "Titolo vero" || headings[1] != "Altro titolo vero" {
		t.Fatalf("titoli attesi [Titolo vero, Altro titolo vero], ottenuti %v", headings)
	}
	// La riga dentro il fence resta testo della prima sezione, non un
	// titolo proprio, e non sparisce.
	if !strings.Contains(got[0].Body, "# non è un titolo") {
		t.Fatalf("il contenuto del blocco recintato è andato perso: %+v", got[0])
	}
}

func TestParseSections_KeepsAHashThatIsPartOfTheTitle(t *testing.T) {
	// La chiusura ATX ("## Titolo ##") va tolta, ma un "#" che fa parte del
	// testo del titolo stesso (come in "C#") no: la differenza è lo spazio
	// che precede la sequenza di chiusura, che nella sintassi ATX è
	// obbligatorio.
	md := "## C#\nTesto del linguaggio.\n\n## Un altro titolo ##\nAltro corpo.\n"
	got := manuals.ParseSections(md)
	if len(got) != 2 {
		t.Fatalf("attese 2 sezioni, ottenute %d: %+v", len(got), got)
	}
	if got[0].Heading != "C#" {
		t.Fatalf("il titolo con # incorporato è stato mangiato: %q", got[0].Heading)
	}
	if got[1].Heading != "Un altro titolo" {
		t.Fatalf("la chiusura ATX doveva sparire: %q", got[1].Heading)
	}
}
