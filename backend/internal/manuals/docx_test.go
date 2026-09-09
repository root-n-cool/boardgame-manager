package manuals_test

import (
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

// TestDocxToMarkdown_NoTextReturnsError copre un .docx senza contenuto
// testuale utile: deve dare un errore riconoscibile, non una stringa vuota
// senza errore — che sparirebbe in silenzio invece di arrivare all'admin.
func TestDocxToMarkdown_NoTextReturnsError(t *testing.T) {
	raw := manuals.NewDocx(nil)

	_, err := manuals.DocxToMarkdown(raw)
	if err == nil {
		t.Fatal("un docx senza paragrafi non ha dato errore")
	}
	if !strings.Contains(err.Error(), "non contiene testo") {
		t.Fatalf("errore inatteso per un docx senza testo: %v", err)
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
