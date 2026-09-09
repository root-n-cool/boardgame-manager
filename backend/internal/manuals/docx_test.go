package manuals_test

import (
	"errors"
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

// hasExactLine dice se md contiene, come riga intera, line. Un semplice
// strings.Contains non basterebbe a distinguere "# Titolo" da "## Titolo":
// la seconda contiene la prima come sottostringa (a partire dal secondo
// cancelletto), quindi un test sul livello del titolo che usasse Contains
// passerebbe anche con un livello sbagliato.
func hasExactLine(md, line string) bool {
	for _, l := range strings.Split(md, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// TestDocxToMarkdown_JoinsTheRunsOfAParagraph è il test che conta di più:
// un paragrafo il cui testo Word ha spezzato su tre run. Un'implementazione
// che ne leggesse solo uno perderebbe metà della frase.
func TestDocxToMarkdown_JoinsTheRunsOfAParagraph(t *testing.T) {
	raw := manuals.NewDocxWithRuns([]manuals.DocxParagraph{
		{Style: "Heading1", Text: "Fase di Upkeep"},
	}, map[int][]string{
		1: {"Ogni giocatore ", "paga una moneta ", "per ciascun edificio."},
	})

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if !strings.Contains(md, "# Fase di Upkeep") {
		t.Fatalf("lo stile Heading1 non è diventato un titolo markdown:\n%s", md)
	}
	if !strings.Contains(md, "Ogni giocatore paga una moneta per ciascun edificio.") {
		t.Fatalf("i run non sono stati uniti:\n%s", md)
	}
}

// TestDocxToMarkdown_MapsHeadingLevelsToHashes verifica Heading1/2/3 →
// #/##/###, con un confronto di riga esatta: vedi hasExactLine sul perché
// Contains non discriminerebbe fra livelli adiacenti.
func TestDocxToMarkdown_MapsHeadingLevelsToHashes(t *testing.T) {
	raw := manuals.NewDocx([]manuals.DocxParagraph{
		{Style: "Heading1", Text: "Titolo uno"},
		{Style: "Heading2", Text: "Titolo due"},
		{Style: "Heading3", Text: "Titolo tre"},
	})

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	for _, line := range []string{"# Titolo uno", "## Titolo due", "### Titolo tre"} {
		if !hasExactLine(md, line) {
			t.Fatalf("attesa la riga esatta %q, non trovata in:\n%s", line, md)
		}
	}
}

// TestDocxToMarkdown_RecognizesHeadingStyleNameVariants copre le varianti
// del nome dello stile che produttori diversi da Word scrivono: "heading 1"
// con spazio, "Titolo1" e "TITOLO2" dei documenti italiani.
func TestDocxToMarkdown_RecognizesHeadingStyleNameVariants(t *testing.T) {
	raw := manuals.NewDocx([]manuals.DocxParagraph{
		{Style: "heading 1", Text: "Variante spaziata"},
		{Style: "Titolo1", Text: "Variante italiana"},
		{Style: "TITOLO2", Text: "Variante italiana maiuscola"},
	})

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	for _, line := range []string{
		"# Variante spaziata",
		"# Variante italiana",
		"## Variante italiana maiuscola",
	} {
		if !hasExactLine(md, line) {
			t.Fatalf("attesa la riga esatta %q, non trovata in:\n%s", line, md)
		}
	}
}

// TestDocxToMarkdown_OutlineLvlFallsBackToHeadingWhenStyleIsCustom copre lo
// stile personalizzato (non "Heading"/"Titolo") che Word tratta comunque
// come titolo tramite <w:outlineLvl>.
func TestDocxToMarkdown_OutlineLvlFallsBackToHeadingWhenStyleIsCustom(t *testing.T) {
	body := `<w:p><w:pPr><w:pStyle w:val="RegolaEvidenziata"/><w:outlineLvl w:val="1"/></w:pPr>` +
		`<w:r><w:t>Titolo da outline</w:t></w:r></w:p>`
	raw := manuals.NewDocxRawBody(body)

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	// outlineLvl è 0-based: il valore 1 corrisponde al secondo livello, "##".
	if !hasExactLine(md, "## Titolo da outline") {
		t.Fatalf("outlineLvl non è stato usato come titolo di livello 2:\n%s", md)
	}
}

// TestDocxToMarkdown_ListItemBecomesDashLine copre <w:numPr>: un paragrafo
// di un elenco (puntato o numerato, la distinzione vive in
// word/numbering.xml che non apriamo) diventa una riga "- ...".
func TestDocxToMarkdown_ListItemBecomesDashLine(t *testing.T) {
	body := `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>` +
		`<w:r><w:t>Voce elenco</w:t></w:r></w:p>`
	raw := manuals.NewDocxRawBody(body)

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if !hasExactLine(md, "- Voce elenco") {
		t.Fatalf("il paragrafo con numPr non è diventato una riga di elenco:\n%s", md)
	}
}

// TestDocxToMarkdown_ExcludesTableText verifica la scelta di design di
// escludere le tabelle: il testo delle celle non deve comparire, perché
// senza struttura a griglia sembrerebbe prosa scorretta invece di dati
// tabellari mancanti.
func TestDocxToMarkdown_ExcludesTableText(t *testing.T) {
	body := `<w:p><w:r><w:t>Prima della tabella</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Cella A</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`<w:p><w:r><w:t>Dopo la tabella</w:t></w:r></w:p>`
	raw := manuals.NewDocxRawBody(body)

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if !strings.Contains(md, "Prima della tabella") || !strings.Contains(md, "Dopo la tabella") {
		t.Fatalf("i paragrafi fuori dalla tabella sono spariti:\n%s", md)
	}
	if strings.Contains(md, "Cella A") {
		t.Fatalf("il testo della tabella non doveva comparire:\n%s", md)
	}
}

// TestDocxToMarkdown_EscapesLeadingHashSoParseSectionsDoesNotMisreadIt copre
// il caso per cui questo task esiste tanto quanto per i run: un paragrafo
// Normal che comincia per coincidenza con "#" non deve, una volta riletto
// da ParseSections (il consumatore reale di questo markdown), diventare un
// titolo — spezzerebbe la sezione a metà frase.
func TestDocxToMarkdown_EscapesLeadingHashSoParseSectionsDoesNotMisreadIt(t *testing.T) {
	raw := manuals.NewDocx([]manuals.DocxParagraph{
		{Style: "Heading1", Text: "Preparazione"},
		{Text: "# nota importante, non un titolo"},
	})

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if !hasExactLine(md, `\# nota importante, non un titolo`) {
		t.Fatalf("il paragrafo con # iniziale non è stato protetto:\n%s", md)
	}

	sections := manuals.ParseSections(md)
	if len(sections) != 1 {
		t.Fatalf("ParseSections ha visto %d sezioni, attesa 1 sola (il # non protetto sarebbe letto come titolo): %+v",
			len(sections), sections)
	}
}

// TestDocxToMarkdown_OnlyEscapesLeadingHash conferma che "-", "*", "+", ">"
// e un elenco numerato ("1.") NON vengono protetti: l'unico consumatore di
// questo markdown è ParseSections, che rilegge solo i titoli ATX. Un
// regolamento scritto senza usare le liste di Word comincia legittimamente
// paragrafi così, e proteggerli sarebbe rumore spurio dentro il testo
// indicizzato e nelle citazioni mostrate all'utente.
func TestDocxToMarkdown_OnlyEscapesLeadingHash(t *testing.T) {
	raw := manuals.NewDocx([]manuals.DocxParagraph{
		{Text: "1. Preparazione del tavolo"},
		{Text: "- variante per due giocatori"},
		{Text: "* punto elenco"},
		{Text: "> citazione dal regolamento originale"},
	})

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	for _, line := range []string{
		"1. Preparazione del tavolo",
		"- variante per due giocatori",
		"* punto elenco",
		"> citazione dal regolamento originale",
	} {
		if !hasExactLine(md, line) {
			t.Fatalf("il paragrafo %q è uscito alterato da un escaping spurio:\n%s", line, md)
		}
	}
}

// TestDocxToMarkdown_NoTextReturnsError copre tre forme diverse in cui un
// .docx reale arriva senza contenuto testuale utile: zero paragrafi, un
// paragrafo di soli spazi, e un documento che contiene testo SOLO dentro
// una tabella (esclusa per scelta di design). La prima da sola non basta a
// discriminare l'implementazione: un bug che, per esempio, dimenticasse di
// TrimSpace il testo di un paragrafo prima di considerarlo vuoto passerebbe
// comunque con zero paragrafi, ma non con un paragrafo di soli spazi o con
// un documento che ha SOLO una tabella.
func TestDocxToMarkdown_NoTextReturnsError(t *testing.T) {
	cases := map[string][]byte{
		"nessun paragrafo": manuals.NewDocx(nil),
		"solo spazi":       manuals.NewDocx([]manuals.DocxParagraph{{Text: "   \n\t  "}}),
		"solo una tabella": manuals.NewDocxRawBody(
			`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Cella A</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`),
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := manuals.DocxToMarkdown(raw)
			if err == nil {
				t.Fatalf("%s: nessun errore per un docx senza testo utile", name)
			}
			if !errors.Is(err, manuals.ErrDocxNoText) {
				t.Fatalf("%s: errore inatteso per un docx senza testo: %v", name, err)
			}
		})
	}
}

// TestDocxToMarkdown_DocumentXMLTooLargeReturnsError copre un
// word/document.xml che, una volta decompresso, supera la soglia: un
// document.xml è XML ripetitivo che comprime moltissimo, quindi un
// archivio piccolo può nasconderne uno enorme (uno zip bomb).
//
// L'asserzione usa errors.Is su ErrDocumentXMLTooLarge, non un controllo
// generico "un errore qualsiasi" o un Contains sul messaggio: un
// io.LimitReader che tronca semplicemente il flusso, SENZA un controllo
// esplicito sulla lunghezza letta, produce comunque un errore (di parsing
// XML, perché il documento troncato non chiude i suoi tag) che un
// controllo debole non distinguerebbe dal comportamento corretto —
// verificato rompendo apposta il controllo di lunghezza in isolamento.
func TestDocxToMarkdown_DocumentXMLTooLargeReturnsError(t *testing.T) {
	// Il corpo da solo, prima dell'involucro XML, è già oltre la soglia:
	// il totale lo sarà per forza.
	oversized := strings.Repeat("A", 6*1024*1024)
	raw := manuals.NewDocxRawBody(oversized)

	_, err := manuals.DocxToMarkdown(raw)
	if !errors.Is(err, manuals.ErrDocumentXMLTooLarge) {
		t.Fatalf("atteso manuals.ErrDocumentXMLTooLarge, ottenuto: %v", err)
	}
}

// TestDocxToMarkdown_InvalidZipReturnsError copre uno zip corrotto: non è
// nemmeno un archivio valido.
func TestDocxToMarkdown_InvalidZipReturnsError(t *testing.T) {
	_, err := manuals.DocxToMarkdown(manuals.NewInvalidZip())
	if err == nil {
		t.Fatal("byte che non sono uno zip non hanno dato errore")
	}
}

// TestDocxToMarkdown_MissingDocumentXMLReturnsError copre uno zip valido ma
// senza word/document.xml: un caso diverso da uno zip corrotto, e
// l'errore deve dirlo (altrimenti l'admin non può distinguere "file
// illeggibile" da "non è un docx").
func TestDocxToMarkdown_MissingDocumentXMLReturnsError(t *testing.T) {
	_, err := manuals.DocxToMarkdown(manuals.NewEmptyZip())
	if err == nil {
		t.Fatal("uno zip senza word/document.xml non ha dato errore")
	}
	if !strings.Contains(err.Error(), "document.xml") {
		t.Fatalf("l'errore non menziona document.xml, non si distingue da un zip generico corrotto: %v", err)
	}
}
