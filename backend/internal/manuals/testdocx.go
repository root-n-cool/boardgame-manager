package manuals

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// Questo file esiste solo per i test: costruisce un .docx minimo ma valido
// (zip con le parti OOXML che contano) usato sia dai test di questo
// package (docx_test.go) sia da quelli di internal/httpapi (che devono
// esercitare l'ingestione su un .docx vero). Non è un file _test.go per lo
// stesso motivo di testpdf.go: gli helper di test di un package non sono
// importabili dai test di un altro package, e tenerne una sola
// implementazione qui evita che le due suite finiscano con due copie della
// stessa costruzione byte-a-byte del docx, destinate a divergere
// silenziosamente nel tempo.

// DocxParagraph è un paragrafo così come lo vede DocxToMarkdown: uno stile
// ("Heading1".."Heading6", "" per un paragrafo normale) e il testo. Serve
// solo al caso semplice — un run solo per paragrafo; il caso che conta
// davvero (un paragrafo spezzato su più run) passa da NewDocxWithRuns.
type DocxParagraph struct {
	Style string
	Text  string
}

// NewDocx costruisce un .docx valido con un paragrafo per elemento di
// paragraphs, ciascuno scritto come un run solo. Per il caso che DEVE
// discriminare l'implementazione — un paragrafo i cui run Word ha spezzato
// in più pezzi — usa NewDocxWithRuns.
func NewDocx(paragraphs []DocxParagraph) []byte {
	return NewDocxWithRuns(paragraphs, nil)
}

// NewDocxWithRuns è NewDocx con la possibilità di dettagliare, per indice
// di paragrafo (0-based), la lista di run in cui il testo è spezzato:
// runsByIndex[i] sostituisce interamente il run singolo che verrebbe
// costruito da paragraphs[i].Text. Un indice presente solo in runsByIndex
// (fuori dai limiti di paragraphs) produce un paragrafo aggiuntivo senza
// stile (un paragrafo normale, il caso comune per il testo spezzato su più
// run). Il numero totale di paragrafi è il massimo fra len(paragraphs) e
// l'indice più alto in runsByIndex, più uno.
func NewDocxWithRuns(paragraphs []DocxParagraph, runsByIndex map[int][]string) []byte {
	total := len(paragraphs)
	for i := range runsByIndex {
		if i+1 > total {
			total = i + 1
		}
	}

	var body strings.Builder
	for i := 0; i < total; i++ {
		style := ""
		if i < len(paragraphs) {
			style = paragraphs[i].Style
		}
		runs, ok := runsByIndex[i]
		if !ok {
			if i < len(paragraphs) {
				runs = []string{paragraphs[i].Text}
			} else {
				runs = nil
			}
		}
		writeParagraphXML(&body, style, runs)
	}

	return NewDocxRawBody(body.String())
}

// NewDocxRawBody costruisce un .docx il cui <w:body> è esattamente bodyXML,
// senza passare dai paragrafi di DocxParagraph: serve ai test che devono
// costruire strutture che DocxParagraph non modella (tabelle, caselle di
// testo, w:numPr, w:outlineLvl) senza duplicare altrove la costruzione
// dello zip.
func NewDocxRawBody(bodyXML string) []byte {
	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` + bodyXML + `</w:body></w:document>`
	return buildDocxZip(documentXML)
}

// writeParagraphXML scrive un singolo <w:p>, con <w:pPr><w:pStyle .../></w:pPr>
// quando style non è vuoto, e un <w:r><w:t>...</w:t></w:r> per ogni run —
// esattamente la forma "un paragrafo, più run" che l'implementazione reale
// deve saper unire.
func writeParagraphXML(w *strings.Builder, style string, runs []string) {
	w.WriteString("<w:p>")
	if style != "" {
		fmt.Fprintf(w, `<w:pPr><w:pStyle w:val="%s"/></w:pPr>`, xmlEscape(style))
	}
	for _, r := range runs {
		fmt.Fprintf(w, `<w:r><w:t xml:space="preserve">%s</w:t></w:r>`, xmlEscape(r))
	}
	w.WriteString("</w:p>")
}

// xmlEscape sfugge i cinque caratteri che l'XML riserva, così un run o uno
// stile con "&", "<", ">" o virgolette nel testo di test non produce uno
// zip con dentro XML malformato.
func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

// buildDocxZip impacchetta documentXML nelle tre parti minime che rendono
// il file un .docx apribile: il manifesto dei content type, la relazione
// radice che punta a word/document.xml, e il documento stesso. Non include
// word/_rels/document.xml.rels: nessun fixture di test referenzia
// immagini o hyperlink dal corpo, quindi non serve.
func buildDocxZip(documentXML string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	writeEntry(zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`+
		`</Types>`)

	writeEntry(zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>`+
		`</Relationships>`)

	writeEntry(zw, "word/document.xml", documentXML)

	if err := zw.Close(); err != nil {
		// Può fallire solo per un bug in questo fixture (mai per input
		// esterno): stesso ragionamento di NewTestJPEG in testpdf.go, un
		// panic a tempo di test è preferibile a portarsi dietro un *testing.T.
		panic("manuals: build fixture docx: " + err.Error())
	}
	return buf.Bytes()
}

// writeEntry scrive un file nello zip in costruzione, andando in panic
// sull'errore per lo stesso motivo di buildDocxZip: qui può fallire solo
// per un bug del fixture.
func writeEntry(zw *zip.Writer, name, content string) {
	f, err := zw.Create(name)
	if err != nil {
		panic("manuals: create fixture zip entry " + name + ": " + err.Error())
	}
	if _, err := f.Write([]byte(content)); err != nil {
		panic("manuals: write fixture zip entry " + name + ": " + err.Error())
	}
}

// NewInvalidZip restituisce byte che non sono uno zip valido: serve al test
// che verifica l'errore su un archivio corrotto, distinto da uno zip valido
// a cui manca semplicemente word/document.xml (NewEmptyZip).
func NewInvalidZip() []byte {
	return []byte("questo non è uno zip, sono solo byte a caso 12345")
}

// NewEmptyZip restituisce uno zip valido ma senza word/document.xml: il
// caso di un archivio ben formato che però non è un .docx (per esempio un
// altro formato OOXML, o un .docx corrotto a cui manca la parte
// principale).
func NewEmptyZip() []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeEntry(zw, "readme.txt", "non è un docx")
	if err := zw.Close(); err != nil {
		panic("manuals: build fixture empty zip: " + err.Error())
	}
	return buf.Bytes()
}
