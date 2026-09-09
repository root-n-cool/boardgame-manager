package manuals

import (
	"regexp"
	"strings"
)

// atxHeading riconosce un titolo ATX a inizio riga: da uno a sei
// cancelletti, uno spazio, il testo del titolo, e opzionalmente la stessa
// sintassi di chiusura ("## Titolo ##"). Non serve un parser markdown
// completo: ci interessano solo i titoli, per tagliare il documento in
// sezioni.
var atxHeading = regexp.MustCompile(`(?m)^(#{1,6})[ \t]+(.+?)[ \t]*$`)

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
// una sezione con Heading vuoto, non viene scartato.
func ParseSections(md string) []Section {
	matches := atxHeading.FindAllStringSubmatchIndex(md, -1)

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

// stripHeadingClose toglie la sintassi di chiusura di un ATX heading
// ("Titolo ##" -> "Titolo"): il gruppo 2 della regex include già la
// chiusura perché "(.+?)" è non goloso ma "[ \t]*$" ferma solo gli spazi
// finali, non i cancelletti di chiusura.
func stripHeadingClose(s string) string {
	s = strings.TrimRight(s, "#")
	return strings.TrimSpace(s)
}

// ChunkSections applica splitToSize al corpo di ogni sezione, portando il
// titolo della sezione su ogni chunk che ne deriva. Seq è progressivo su
// tutto il documento (non riparte a ogni sezione): è ciò che permette di
// allegare "il chunk vicino" (seq ± 1) senza attraversare sezioni a caso.
func ChunkSections(sections []Section) []SectionChunk {
	var out []SectionChunk
	seq := 0
	for _, s := range sections {
		body := strings.TrimSpace(s.Body)
		if body == "" {
			continue
		}
		// cursor avanza man mano che i chunk si susseguono nella sezione:
		// ogni chunk (dopo il primo) comincia con la coda di sovrapposizione
		// del precedente, quindi il suo vero inizio cade dentro la stringa
		// del chunk precedente. Cercare da un cursore che avanza (invece che
		// da s.Body[0] ogni volta) evita di trovare, per un chunk qualunque,
		// un'occorrenza anteriore coincidente dello stesso testo altrove
		// nella sezione.
		cursor := 0
		for _, part := range splitToSize(body) {
			offset := s.Offset + cursor
			if idx := strings.Index(s.Body[cursor:], part); idx >= 0 {
				offset = s.Offset + cursor + idx
				cursor += idx + 1
			}
			out = append(out, SectionChunk{
				Heading: s.Heading,
				Seq:     seq,
				Offset:  offset,
				Text:    part,
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
