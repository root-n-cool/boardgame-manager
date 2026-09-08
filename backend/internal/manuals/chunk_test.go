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
	// splitToSize non spezza mai un'unità atomica (un paragrafo, o una
	// frase quando il paragrafo è troppo lungo) fra due chunk: un'unità
	// intera finisce sempre in un chunk solo. Una regola fatta da UNA sola
	// frase non potrebbe quindi mai finire a cavallo di un taglio, con o
	// senza sovrapposizione — e un test con quella forma passerebbe anche
	// se tailFrom non facesse nulla, verificando l'atomicità (già coperta
	// da TestChunk_SplitsLongPagesWithoutBreakingSentences) invece della
	// sovrapposizione.
	//
	// Qui la regola è DUE frasi (due unità), e il riempimento è tarato
	// perché il taglio cada esattamente fra le due: la prima frase resta
	// l'ultima unità del chunk prima del taglio, la seconda apre quello
	// dopo. Solo la coda ripetuta da tailFrom rimette la prima frase in
	// testa al chunk successivo, riunendo la regola intera in un chunk
	// solo — senza quella coda, ciascun chunk ne conterrebbe solo metà.
	// Verificato disattivando temporaneamente la chiamata a tailFrom in
	// flush(): con la sovrapposizione disattivata questo test fallisce
	// (nessun chunk contiene la regola intera); con la sovrapposizione
	// riattivata passa. Vedi il fix report del Task 4 per l'evidenza.
	filler := strings.Repeat("Testo di riempimento del regolamento. ", 24)
	rule1 := "Il giocatore attivo pesca due carte dal mazzo principale."
	rule2 := "Se il mazzo è vuoto rimescola gli scarti e continua a pescare."
	rule := rule1 + " " + rule2
	chunks := manuals.Chunk([]manuals.Page{{Number: 8, Text: filler + rule1 + " " + rule2 + " " + filler}})

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

func TestChunk_SeqRestartsPerPageAcrossMultiChunkPages(t *testing.T) {
	// Seq è l'ordinale dentro la pagina, non un contatore che avanza per
	// tutto il manuale: due pagine che producono ciascuna più chunk devono
	// avere entrambe una sequenza 0,1,2,... propria. I test esistenti
	// coprivano solo una pagina multi-chunk da sola, o due pagine a un
	// chunk solo ciascuna: nessuno dei due esercita un Seq che
	// erroneamente continuasse a salire da una pagina all'altra invece di
	// azzerarsi. PageNumber e Seq sono ciò che diventa la citazione "pag.
	// N" che qualcuno legge al tavolo, quindi qui si controllano entrambi
	// su ogni chunk di entrambe le pagine, non solo il conteggio totale.
	sentence := "Ogni giocatore paga una moneta per ciascun edificio posseduto e ne verifica la produzione. "
	long := strings.Repeat(sentence, 40)
	chunks := manuals.Chunk([]manuals.Page{
		{Number: 10, Text: long},
		{Number: 11, Text: long},
	})

	var page10, page11 []manuals.TextChunk
	for _, c := range chunks {
		switch c.PageNumber {
		case 10:
			page10 = append(page10, c)
		case 11:
			page11 = append(page11, c)
		default:
			t.Fatalf("chunk con PageNumber inatteso, né 10 né 11: %d", c.PageNumber)
		}
	}
	if len(page10) < 2 || len(page11) < 2 {
		t.Fatalf("attese entrambe le pagine multi-chunk, ottenuti %d chunk (pag. 10) e %d chunk (pag. 11)",
			len(page10), len(page11))
	}
	for i, c := range page10 {
		if c.Seq != i {
			t.Fatalf("pag. 10: seq non progressivo, chunk %d ha seq %d", i, c.Seq)
		}
	}
	for i, c := range page11 {
		if c.Seq != i {
			t.Fatalf("pag. 11: seq non riparte da 0, chunk %d ha seq %d", i, c.Seq)
		}
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
	// Ognuno dei tre casi di rifiuto qui sotto isola UNA delle tre
	// proprietà del commento di DetectHeading, cioè continua a fallire (e
	// quindi a proteggere davvero, non solo ad apparire verde) anche se le
	// altre due proprietà venissero disattivate. Verificato disattivando a
	// turno ciascun ramo di rifiuto in DetectHeading (chunk.go) e
	// controllando che SOLO il caso mirato a quel ramo tornasse a
	// restituire il titolo invece di "": i due casi negativi originali di
	// questo test (prima frase compiuta, prima riga troppo lunga) non
	// erano isolati — la frase compiuta aveva anche più di 8 parole, la
	// riga lunga cominciava per minuscola — quindi passavano ancora con il
	// ramo bersaglio disattivato, protetti per caso da un altro ramo.
	cases := []struct{ in, want string }{
		{"Fase di Upkeep\nOgni giocatore paga una moneta per ogni edificio.", "Fase di Upkeep"},
		{"CONTEGGIO DEI PUNTI\nOgni edificio vale i punti stampati.", "CONTEGGIO DEI PUNTI"},
		// Isola SOLO il rifiuto per troppe parole: comincia in maiuscolo e
		// non finisce con un punto, quindi se il conteggio delle parole
		// non venisse controllato non ci sarebbe nessun altro motivo per
		// rifiutarla.
		{"Regole speciali per la partita con più di quattro giocatori esperti\naltro testo", ""},
		// Isola SOLO il rifiuto per punto finale: poche parole, comincia
		// in maiuscolo, quindi se il punto finale non venisse controllato
		// non ci sarebbe nessun altro motivo per rifiutarla.
		{"Il gioco finisce qui.", ""},
		// Isola SOLO il rifiuto per iniziale minuscola: poche parole, non
		// finisce con un punto — l'unica proprietà delle tre che la
		// squalifica è l'iniziale minuscola. Prima di questo test
		// unicode.IsUpper era codice reale ma mai esercitato da un caso
		// che dipendesse solo da lui.
		{"regole speciali\nOgni giocatore pesca due carte.", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := manuals.DetectHeading(c.in); got != c.want {
			t.Fatalf("DetectHeading(%.30q) = %q, atteso %q", c.in, got, c.want)
		}
	}
}
