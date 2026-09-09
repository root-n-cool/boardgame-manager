package manuals

import (
	"regexp"
	"strings"
	"unicode"
)

// atxHeading riconosce un titolo ATX a inizio riga: da uno a sei
// cancelletti, uno spazio, il testo del titolo, e opzionalmente la stessa
// sintassi di chiusura ("## Titolo ##"). Non serve un parser markdown
// completo: ci interessano solo i titoli, per tagliare il documento in
// sezioni.
var atxHeading = regexp.MustCompile(`(?m)^(#{1,6})[ \t]+(.+?)[ \t]*$`)

// atxClose toglie la sequenza di chiusura di un ATX heading ("Titolo ##"),
// riconosciuta SOLO quando preceduta da uno spazio: è quanto dice la
// sintassi ATX, ed è ciò che distingue la chiusura dal cancelletto che fa
// parte del titolo stesso, come in "C#" — senza il vincolo dello spazio,
// "## C#" perderebbe il cancelletto del titolo insieme a quello di
// chiusura.
var atxClose = regexp.MustCompile(`[ \t]+#+[ \t]*$`)

// fenceMarker riconosce l'apertura o la chiusura di un blocco di codice
// recintato (``` o ~~~), a inizio riga con eventuale indentazione. Un "#"
// dentro un blocco recintato non è un titolo: è contenuto del blocco (per
// esempio un commento in uno pseudocodice, o un "#" di Markdown mostrato
// come esempio), e va lasciato nel corpo della sezione senza tagliarla.
var fenceMarker = regexp.MustCompile(`(?m)^[ \t]*(?:` + "```" + `|~~~)`)

// Section è un pezzo di markdown compreso fra un titolo (incluso) e il
// successivo. Heading è vuoto per il testo che precede il primo titolo del
// documento: un preambolo, non un errore — molti regolamenti aprono con un
// paragrafo introduttivo prima del primo "##".
//
// Offset è l'offset in byte di Body nel markdown d'origine: è ciò che nel
// Task 5 permette di risalire alla pagina del PDF in cui la sezione
// comincia (reference_detail). Senza, un chunk sarebbe citabile solo come
// "da qualche parte nel manuale".
type Section struct {
	Heading string
	Body    string
	Offset  int
}

// ParseSections spezza md sui titoli ATX (da # a ######), gestendo la
// forma chiusa ("## Titolo ##"). Il testo prima del primo titolo diventa
// una sezione con Heading vuoto, non viene scartato. Un "#" dentro un
// blocco di codice recintato (``` o ~~~) non è un titolo e non spezza la
// sezione.
func ParseSections(md string) []Section {
	fences := fencedRanges(md)

	all := atxHeading.FindAllStringSubmatchIndex(md, -1)
	matches := make([][]int, 0, len(all))
	for _, m := range all {
		if insideAny(fences, m[0]) {
			continue
		}
		matches = append(matches, m)
	}

	var sections []Section
	// Il preambolo va dall'inizio del documento al primo titolo (o alla
	// fine, se non ce n'è nessuno).
	firstStart := len(md)
	if len(matches) > 0 {
		firstStart = matches[0][0]
	}
	if preamble := md[:firstStart]; strings.TrimSpace(preamble) != "" {
		sections = append(sections, Section{Heading: "", Body: preamble, Offset: 0})
	}

	for i, m := range matches {
		// m[4], m[5] sono l'inizio/fine del gruppo 2 (il testo del titolo).
		heading := stripHeadingClose(md[m[4]:m[5]])
		bodyStart := m[1] // fine dell'intero match (titolo + newline finale escluso)
		// bodyStart punta subito dopo il titolo; se segue un newline lo
		// includiamo nel corpo, non nell'offset del corpo stesso: il corpo
		// comincia sulla riga successiva al titolo.
		if bodyStart < len(md) && md[bodyStart] == '\n' {
			bodyStart++
		} else if bodyStart+1 < len(md) && md[bodyStart] == '\r' && md[bodyStart+1] == '\n' {
			bodyStart += 2
		}
		bodyEnd := len(md)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		sections = append(sections, Section{
			Heading: heading,
			Body:    md[bodyStart:bodyEnd],
			Offset:  bodyStart,
		})
	}

	return sections
}

// fencedRanges restituisce gli intervalli [inizio, fine) di md occupati da
// blocchi di codice recintati, individuando le righe che aprono/chiudono un
// fence (``` o ~~~) e accoppiandole a due a due, nell'ordine in cui
// compaiono. Un fence lasciato aperto (mai chiuso) copre fino alla fine del
// documento: un errore nel manuale non deve far leggere titoli dentro un
// blocco che l'autore intendeva come codice.
func fencedRanges(md string) [][2]int {
	openings := fenceMarker.FindAllStringIndex(md, -1)
	if len(openings) == 0 {
		return nil
	}

	var ranges [][2]int
	open := false
	var start int
	for _, m := range openings {
		lineEnd := m[0]
		if idx := strings.IndexByte(md[m[0]:], '\n'); idx >= 0 {
			lineEnd = m[0] + idx + 1
		} else {
			lineEnd = len(md)
		}
		if !open {
			open = true
			start = m[0]
		} else {
			open = false
			ranges = append(ranges, [2]int{start, lineEnd})
		}
	}
	if open {
		ranges = append(ranges, [2]int{start, len(md)})
	}
	return ranges
}

// insideAny dice se pos cade dentro uno degli intervalli [inizio, fine).
func insideAny(ranges [][2]int, pos int) bool {
	for _, r := range ranges {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}

// stripHeadingClose toglie la sintassi di chiusura di un ATX heading
// ("Titolo ##" -> "Titolo"): il gruppo 2 della regex include già la
// chiusura perché "(.+?)" è non goloso ma "[ \t]*$" ferma solo gli spazi
// finali, non i cancelletti di chiusura. Solo la sequenza preceduta da uno
// spazio è chiusura; un "#" attaccato al testo (come in "C#") resta.
func stripHeadingClose(s string) string {
	s = atxClose.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// ChunkSections applica splitToSize al corpo di ogni sezione, portando il
// titolo della sezione su ogni chunk che ne deriva. Seq è progressivo su
// tutto il documento (non riparte a ogni sezione): è ciò che permette di
// allegare "il chunk vicino" (seq ± 1) senza attraversare sezioni a caso.
//
// L'Offset di ogni chunk viene da splitToSize, che lo traccia PER
// COSTRUZIONE (sommando le posizioni reali delle unità e della coda di
// sovrapposizione mentre le consuma) e non ricercandolo a posteriori nel
// testo: su un corpo con frasi ripetute — comune in un regolamento — una
// ricerca del testo del chunk troverebbe la prima occorrenza, non
// necessariamente quella giusta, e produrrebbe una citazione sbagliata
// senza errore visibile.
func ChunkSections(sections []Section) []SectionChunk {
	var out []SectionChunk
	seq := 0
	for _, s := range sections {
		// leadTrim è quanto TrimSpace toglie dall'inizio di s.Body: serve a
		// riportare gli offset di splitToSize (relativi al corpo già
		// trimmato) all'offset nel documento originale.
		leadTrim := len(s.Body) - len(strings.TrimLeftFunc(s.Body, unicode.IsSpace))
		body := strings.TrimSpace(s.Body)
		if body == "" {
			continue
		}
		for _, piece := range splitToSize(body) {
			out = append(out, SectionChunk{
				Heading: s.Heading,
				Seq:     seq,
				Offset:  s.Offset + leadTrim + piece.Offset,
				Text:    piece.Text,
			})
			seq++
		}
	}
	return out
}

// SectionChunk è un pezzo cercabile di una sezione di markdown. Offset è
// l'offset in byte, nel markdown d'origine, dove comincia il chunk: serve
// nel Task 5 a decidere in quale pagina del PDF il chunk comincia.
type SectionChunk struct {
	Heading string
	Seq     int
	Offset  int
	Text    string
}
