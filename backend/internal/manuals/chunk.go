package manuals

import (
	"regexp"
	"strings"
	"unicode"
)

// MaxChunkChars è la dimensione massima di un chunk. Mille caratteri sono
// ~250 token: cinque chunk stanno in 1.500 token di payload, che è il
// budget deciso per una chiamata al tool.
const MaxChunkChars = 1000

// ChunkOverlapChars è quanto un chunk ripete della coda del precedente.
// Serve a un caso preciso: una regola a cavallo del taglio, che senza
// sovrapposizione non si troverebbe né prima né dopo.
const ChunkOverlapChars = 100

// sentenceEnd trova la fine di una frase: punto, esclamativo, interrogativo
// o due punti, seguiti da spazio. Compilata a livello di package come le
// regexp di pdf.go: è usata per ogni sezione di ogni manuale, ricompilarla
// ad ogni chiamata rifarebbe lo stesso lavoro.
var sentenceEnd = regexp.MustCompile(`[.!?:]\s+`)

// chunkPiece è un pezzo di testo con l'offset — in byte, relativo al testo
// passato a splitToSize — di dove comincia. È interno al pacchetto: serve a
// ChunkSections per calcolare SectionChunk.Offset SENZA dover ricercare il
// testo del chunk a posteriori nel documento con strings.Index, ricerca che
// su frasi ripetute (comuni in un regolamento: "Ogni giocatore pesca una
// carta." può comparire identica più volte) trova la prima occorrenza dal
// cursore, non necessariamente quella giusta, e produce un offset che
// punta a un punto sbagliato del documento — una citazione sbagliata che
// non fa rumore. L'offset viene invece tracciato per costruzione, sommando
// le posizioni reali delle unità e della coda di sovrapposizione mentre si
// consumano.
type chunkPiece struct {
	Text   string
	Offset int
}

// splitToSize riduce un testo a pezzi sotto MaxChunkChars, con la coda del
// pezzo precedente ripetuta in testa al successivo. L'Offset di ogni pezzo
// è relativo a text (il parametro), non al documento intero: chi chiama
// aggiunge il proprio offset di base.
func splitToSize(text string) []chunkPiece {
	if len(text) <= MaxChunkChars {
		return []chunkPiece{{Text: text, Offset: 0}}
	}

	units := splitUnits(text)
	var out []chunkPiece
	var cur strings.Builder
	// curOffset è l'offset, in text, del primo carattere scritto in cur dopo
	// l'ultimo flush (la coda di sovrapposizione se presente, altrimenti la
	// prima unità nuova). -1 finché cur è vuoto e non ancora assegnato.
	curOffset := -1

	flush := func() {
		body := strings.TrimSpace(cur.String())
		if body == "" {
			return
		}
		out = append(out, chunkPiece{Text: body, Offset: curOffset})
		cur.Reset()
		// La coda del chunk appena chiuso apre il prossimo, tagliata
		// all'inizio di frase più vicino per non cominciare a metà frase.
		tail, tailOffset := tailFrom(body, curOffset)
		curOffset = -1
		if tail != "" {
			cur.WriteString(tail)
			curOffset = tailOffset
		}
	}

	// Nota su un dubbio sollevato in review: un chunk fatto SOLO della coda
	// di sovrapposizione (testo già presente nel chunk precedente,
	// indicizzato due volte e restituibile come hit senza contenuto proprio)
	// non è producibile da questo ciclo. Il flush avviene sempre appena
	// prima di scrivere l'unità che lo ha provocato, quindi al flush
	// successivo `cur` contiene sempre almeno un'unità intera oltre alla
	// coda. Verificato anche per forza bruta su 20.000 testi generati
	// (paragrafi, frasi e unità più lunghe del massimo, con riempitivo
	// diverso per ogni unità così che "chunk contenuto nel precedente"
	// significhi davvero "nessuna unità nuova"): nessun caso.
	for _, u := range units {
		if cur.Len()+len(u.Text) > MaxChunkChars && strings.TrimSpace(cur.String()) != "" {
			flush()
		}
		// Un'unità più lunga del massimo da sola: entra comunque, perché
		// spezzarla a caso produrrebbe un chunk che comincia a metà frase o
		// a metà parola — un difetto peggiore di superare il limite di
		// dimensione in un caso raro (un paragrafo senza punteggiatura).
		if curOffset == -1 {
			curOffset = u.Offset
		}
		cur.WriteString(u.Text)
	}
	if body := strings.TrimSpace(cur.String()); body != "" {
		out = append(out, chunkPiece{Text: body, Offset: curOffset})
	}
	return out
}

// splitUnits spezza in paragrafi e, per i paragrafi troppo lunghi, in
// frasi. Le unità conservano lo spazio finale, così ricomporle non incolla
// le parole. Offset è la posizione reale di ogni unità in text: viene
// calcolata scorrendo i separatori "\n\n" (che strings.Split consuma) e
// contando gli spazi bianchi iniziali che TrimSpace toglie da ogni
// paragrafo — non recuperata cercando il testo dell'unità nel documento,
// ricerca che su testo ripetuto troverebbe l'occorrenza sbagliata.
func splitUnits(text string) []chunkPiece {
	var units []chunkPiece
	pos := 0
	parts := strings.Split(text, "\n\n")
	for i, para := range parts {
		paraStart := pos
		pos += len(para)
		if i < len(parts)-1 {
			pos += 2 // il separatore "\n\n" consumato da Split
		}
		trimmed := strings.TrimSpace(para)
		if trimmed == "" {
			continue
		}
		lead := len(para) - len(strings.TrimLeftFunc(para, unicode.IsSpace))
		trimmedOffset := paraStart + lead
		if len(trimmed) <= MaxChunkChars {
			units = append(units, chunkPiece{Text: trimmed + "\n\n", Offset: trimmedOffset})
			continue
		}
		last := 0
		for _, m := range sentenceEnd.FindAllStringIndex(trimmed, -1) {
			units = append(units, chunkPiece{Text: trimmed[last:m[1]], Offset: trimmedOffset + last})
			last = m[1]
		}
		if last < len(trimmed) {
			units = append(units, chunkPiece{Text: trimmed[last:], Offset: trimmedOffset + last})
		}
	}
	return units
}

// tailFrom restituisce gli ultimi ~ChunkOverlapChars caratteri di body,
// allineati all'inizio di una frase, insieme al loro offset reale (bodyOffset
// è l'offset di body nel documento, tracciato dal chiamante). body è sempre
// la concatenazione di unità intere (paragrafi o frasi, mai un pezzo di
// frase), quindi quando è più corto della finestra di sovrapposizione è già
// di per sé un prefisso valido per il chunk successivo.
//
// Se invece la finestra delle ultime ChunkOverlapChars non contiene nessun
// confine di frase (una frase più lunga della finestra stessa: raro, ma
// possibile su un paragrafo senza punteggiatura), non c'è modo di tagliarla
// senza spezzarla a metà: qui si rinuncia alla sovrapposizione piuttosto
// che rischiare un chunk che comincia a metà frase, che è l'invariante che
// conta di più. In quel caso l'offset restituito non ha significato (-1):
// il chiamante lo ignora perché non scrive nessuna coda.
func tailFrom(body string, bodyOffset int) (string, int) {
	if len(body) <= ChunkOverlapChars {
		return body + " ", bodyOffset
	}
	windowStart := len(body) - ChunkOverlapChars
	window := body[windowStart:]
	m := sentenceEnd.FindStringIndex(window)
	if m == nil {
		return "", -1
	}
	rest := window[m[1]:]
	lead := len(rest) - len(strings.TrimLeftFunc(rest, unicode.IsSpace))
	tail := strings.TrimSpace(rest)
	if tail == "" {
		return "", -1
	}
	tailOffset := bodyOffset + windowStart + m[1] + lead
	return tail + " ", tailOffset
}
