package manuals_test

import (
	"fmt"
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

func TestChunkSections_KeepsShortSectionsWhole(t *testing.T) {
	sections := []manuals.Section{
		{Heading: "Turno", Body: "Turno del giocatore. Si pescano due carte.", Offset: 3},
	}
	chunks := manuals.ChunkSections(sections)
	if len(chunks) != 1 {
		t.Fatalf("una sezione corta è un chunk solo, ottenuti %d", len(chunks))
	}
	if chunks[0].Heading != "Turno" || chunks[0].Seq != 0 {
		t.Fatalf("heading/seq attesi Turno/0, ottenuti %q/%d", chunks[0].Heading, chunks[0].Seq)
	}
}

func TestChunkSections_SplitsLongSectionsWithoutBreakingSentences(t *testing.T) {
	// Una sezione lunga il triplo del massimo: deve uscire in più chunk,
	// ognuno sotto il limite, e nessuno deve cominciare a metà frase.
	sentence := "Ogni giocatore paga una moneta per ciascun edificio posseduto e ne verifica la produzione. "
	long := strings.Repeat(sentence, 40)
	chunks := manuals.ChunkSections([]manuals.Section{{Heading: "Upkeep", Body: long, Offset: 0}})

	if len(chunks) < 2 {
		t.Fatalf("attesi più chunk da una sezione lunga, ottenuti %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Text) > manuals.MaxChunkChars {
			t.Fatalf("chunk %d supera il limite: %d caratteri", i, len(c.Text))
		}
		if c.Heading != "Upkeep" {
			t.Fatalf("chunk %d ha perso il titolo della sezione: %q", i, c.Heading)
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

func TestChunkSections_OverlapsSoARuleOnTheBoundaryIsFindable(t *testing.T) {
	// splitToSize non spezza mai un'unità atomica (un paragrafo, o una
	// frase quando il paragrafo è troppo lungo) fra due chunk: un'unità
	// intera finisce sempre in un chunk solo. Una regola fatta da UNA sola
	// frase non potrebbe quindi mai finire a cavallo di un taglio, con o
	// senza sovrapposizione — e un test con quella forma passerebbe anche
	// se tailFrom non facesse nulla, verificando l'atomicità invece della
	// sovrapposizione.
	//
	// Qui la regola è DUE frasi (due unità), e il riempimento è tarato
	// perché il taglio cada esattamente fra le due: la prima frase resta
	// l'ultima unità del chunk prima del taglio, la seconda apre quello
	// dopo. Solo la coda ripetuta da tailFrom rimette la prima frase in
	// testa al chunk successivo, riunendo la regola intera in un chunk
	// solo — senza quella coda, ciascun chunk ne conterrebbe solo metà.
	//
	// Riprovato rompendolo per questo task: portando ChunkOverlapChars a 0
	// (o saltando tailFrom in flush()) il test torna rosso, confermando che
	// misura davvero la sovrapposizione e non solo l'atomicità.
	filler := strings.Repeat("Testo di riempimento del regolamento. ", 24)
	rule1 := "Il giocatore attivo pesca due carte dal mazzo principale."
	rule2 := "Se il mazzo è vuoto rimescola gli scarti e continua a pescare."
	rule := rule1 + " " + rule2
	sections := []manuals.Section{
		{Heading: "Pesca", Body: filler + rule1 + " " + rule2 + " " + filler, Offset: 0},
	}
	chunks := manuals.ChunkSections(sections)

	found := false
	for _, c := range chunks {
		if strings.Contains(c.Text, rule) {
			found = true
		}
	}
	if !found {
		t.Fatal("la regola a due frasi, a cavallo del taglio, non compare intera in nessun chunk")
	}
}

func TestChunkSections_IsDeterministic(t *testing.T) {
	sections := []manuals.Section{
		{Heading: "Regole", Body: strings.Repeat("Una frase del manuale. ", 120), Offset: 0},
	}
	a := manuals.ChunkSections(sections)
	b := manuals.ChunkSections(sections)
	if len(a) != len(b) {
		t.Fatalf("due esecuzioni danno %d e %d chunk", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("chunk %d differisce fra due esecuzioni", i)
		}
	}
}

func TestChunkSections_SkipsEmptySections(t *testing.T) {
	chunks := manuals.ChunkSections([]manuals.Section{
		{Heading: "Vuota", Body: "   \n  ", Offset: 0},
		{Heading: "Piena", Body: "Contenuto vero.", Offset: 10},
	})
	if len(chunks) != 1 || chunks[0].Heading != "Piena" {
		t.Fatalf("una sezione vuota non produce chunk; ottenuti %d", len(chunks))
	}
}

func TestChunkSections_KeepsASectionOfExactlyTheMaximumWhole(t *testing.T) {
	// Il confine esatto: splitToSize taglia con `len(text) <= MaxChunkChars`
	// e una sezione lunga esattamente il massimo deve restare intera. Un
	// off-by-one qui (`<` invece di `<=`) la manderebbe sul percorso di
	// split, che con la sovrapposizione produrrebbe testo duplicato.
	//
	// DUE paragrafi e non uno solo: con un paragrafo solo il percorso di
	// split restituirebbe comunque un unico chunk identico all'originale
	// (un'unità intera non si spezza mai), e il test passerebbe anche con
	// l'off-by-one — verificando niente. Con due paragrafi da 499
	// caratteri, ognuno diventa un'unità da 501 (`\n\n` incluso) e il
	// percorso di split ne fa due chunk: la differenza si vede.
	para := func(prefix string, n int) string {
		return prefix + strings.Repeat("x", n-len(prefix))
	}
	text := para("Fase di Upkeep. ", 499) + "\n\n" + para("Fine partita. ", 499)
	if len(text) != manuals.MaxChunkChars {
		t.Fatalf("fixture sbagliata: %d caratteri invece di %d", len(text), manuals.MaxChunkChars)
	}

	chunks := manuals.ChunkSections([]manuals.Section{{Heading: "Sezione", Body: text, Offset: 0}})
	if len(chunks) != 1 {
		t.Fatalf("una sezione lunga esattamente il massimo è un chunk solo, ottenuti %d", len(chunks))
	}
	if chunks[0].Text != text {
		t.Fatalf("il testo è stato alterato: %d caratteri su %d", len(chunks[0].Text), len(text))
	}
}

func TestChunkSections_ASingleSentenceLongerThanTheMaximumIsOneChunk(t *testing.T) {
	// Una sezione fatta di UNA sola unità più lunga del massimo: non c'è
	// nessun posto dove tagliarla senza spezzare una parola, quindi esce
	// intera, in un chunk solo, e senza perdere niente.
	giant := strings.TrimSpace(strings.Repeat("parola ", 400))
	if len(giant) <= manuals.MaxChunkChars {
		t.Fatalf("fixture sbagliata: %d caratteri, non supera il massimo", len(giant))
	}
	chunks := manuals.ChunkSections([]manuals.Section{{Heading: "Sezione", Body: giant, Offset: 0}})
	if len(chunks) != 1 {
		t.Fatalf("attesa una sola unità atomica in un chunk solo, ottenuti %d", len(chunks))
	}
	if strings.TrimSpace(chunks[0].Text) != giant {
		t.Fatal("il testo dell'unità atomica è stato alterato")
	}
}

func TestChunkSections_ALongUnitAfterOthersDoesNotLoseTheTextBeforeIt(t *testing.T) {
	// Il taglio davanti a un'unità più lunga del massimo: quel che veniva
	// prima deve restare in un chunk suo, e l'unità lunga deve arrivare
	// intera in quello dopo. Frasi tutte diverse, così un chunk mancante si
	// vede: con frasi identiche qualunque perdita passerebbe inosservata.
	var before strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&before, "Regola numero %d: ogni giocatore paga una moneta per edificio. ", i)
	}
	giant := strings.Repeat("parolalunghissimasenzapunteggiatura ", 60)
	chunks := manuals.ChunkSections([]manuals.Section{{Heading: "Sezione", Body: before.String() + giant, Offset: 0}})

	if len(chunks) < 2 {
		t.Fatalf("attesi almeno due chunk, ottenuti %d", len(chunks))
	}
	if !strings.Contains(chunks[len(chunks)-1].Text, "parolalunghissimasenzapunteggiatura") {
		t.Fatal("l'unità più lunga del massimo è sparita dai chunk")
	}
	joined := ""
	for i, c := range chunks {
		if strings.TrimSpace(c.Text) == "" {
			t.Fatalf("chunk %d vuoto", i)
		}
		joined += c.Text + " "
	}
	for i := 1; i <= 20; i++ {
		if !strings.Contains(joined, fmt.Sprintf("Regola numero %d:", i)) {
			t.Fatalf("la regola %d è andata persa nel taglio", i)
		}
	}
}
