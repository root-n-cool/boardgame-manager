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

// TextChunk è un pezzo cercabile di manuale. PageNumber è ciò che rende
// possibile la citazione; Seq è l'ordinale nella pagina, e serve ad
// allegare il chunk vicino quando il match cade su un bordo.
//
// Chiamato TextChunk e non Chunk: Go non permette un tipo e una funzione
// con lo stesso identificatore nello stesso package, e la funzione sotto
// deve chiamarsi Chunk (è così che la chiama, senza qualificatore, lo
// store del Task 5). Il brief di questo task nominava entrambi "Chunk";
// è un conflitto irrisolvibile alla lettera, quindi si rinomina il tipo,
// che altrove nel piano non è mai referenziato per nome — solo la
// funzione lo è (`Chunk(plain)` in store.go).
type TextChunk struct {
	PageNumber int
	Seq        int
	Text       string
}

// sentenceEnd trova la fine di una frase: punto, esclamativo, interrogativo
// o due punti, seguiti da spazio. Compilata a livello di package come le
// regexp di pdf.go: è usata per ogni pagina di ogni manuale, ricompilarla
// ad ogni chiamata rifarebbe lo stesso lavoro.
var sentenceEnd = regexp.MustCompile(`[.!?:]\s+`)

// Chunk spezza le pagine in chunk, tagliando prima sui paragrafi e poi
// sulle frasi, senza mai spezzare una frase. Deterministico: lo stesso
// input dà sempre gli stessi chunk, così manual_chunk si può svuotare e
// ricostruire da manual_page in qualunque momento.
func Chunk(pages []Page) []TextChunk {
	var out []TextChunk
	for _, p := range pages {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}
		for i, body := range splitToSize(text) {
			out = append(out, TextChunk{PageNumber: p.Number, Seq: i, Text: body})
		}
	}
	return out
}

// splitToSize riduce un testo a pezzi sotto MaxChunkChars, con la coda del
// pezzo precedente ripetuta in testa al successivo.
func splitToSize(text string) []string {
	if len(text) <= MaxChunkChars {
		return []string{text}
	}

	units := splitUnits(text)
	var out []string
	var cur strings.Builder

	flush := func() {
		body := strings.TrimSpace(cur.String())
		if body == "" {
			return
		}
		out = append(out, body)
		cur.Reset()
		// La coda del chunk appena chiuso apre il prossimo, tagliata
		// all'inizio di frase più vicino per non cominciare a metà frase.
		cur.WriteString(tailFrom(body))
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
		if cur.Len()+len(u) > MaxChunkChars && strings.TrimSpace(cur.String()) != "" {
			flush()
		}
		// Un'unità più lunga del massimo da sola: entra comunque, perché
		// spezzarla a caso produrrebbe un chunk che comincia a metà frase o
		// a metà parola — un difetto peggiore di superare il limite di
		// dimensione in un caso raro (un paragrafo senza punteggiatura).
		cur.WriteString(u)
	}
	if body := strings.TrimSpace(cur.String()); body != "" {
		out = append(out, body)
	}
	return out
}

// splitUnits spezza in paragrafi e, per i paragrafi troppo lunghi, in
// frasi. Le unità conservano lo spazio finale, così ricomporle non incolla
// le parole.
func splitUnits(text string) []string {
	var units []string
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if len(para) <= MaxChunkChars {
			units = append(units, para+"\n\n")
			continue
		}
		last := 0
		for _, m := range sentenceEnd.FindAllStringIndex(para, -1) {
			units = append(units, para[last:m[1]])
			last = m[1]
		}
		if last < len(para) {
			units = append(units, para[last:])
		}
	}
	return units
}

// tailFrom restituisce gli ultimi ~ChunkOverlapChars caratteri di body,
// allineati all'inizio di una frase. body è sempre la concatenazione di
// unità intere (paragrafi o frasi, mai un pezzo di frase), quindi quando è
// più corto della finestra di sovrapposizione è già di per sé un prefisso
// valido per il chunk successivo.
//
// Se invece la finestra delle ultime ChunkOverlapChars non contiene nessun
// confine di frase (una frase più lunga della finestra stessa: raro, ma
// possibile su un paragrafo senza punteggiatura), non c'è modo di tagliarla
// senza spezzarla a metà: qui si rinuncia alla sovrapposizione piuttosto
// che rischiare un chunk che comincia a metà frase, che è l'invariante che
// conta di più.
func tailFrom(body string) string {
	if len(body) <= ChunkOverlapChars {
		return body + " "
	}
	window := body[len(body)-ChunkOverlapChars:]
	m := sentenceEnd.FindStringIndex(window)
	if m == nil {
		return ""
	}
	window = strings.TrimSpace(window[m[1]:])
	if window == "" {
		return ""
	}
	return window + " "
}

// headingWords è il massimo di parole che può avere un titolo di sezione.
const headingWords = 8

// headingOrdinal è la numerazione che apre un titolo in un regolamento:
// "1. Preparazione", "2) Il turno". Serve lo spazio dopo il separatore, così
// "1.5 punti vittoria" non diventa "5 punti vittoria".
var headingOrdinal = regexp.MustCompile(`^\d{1,2}[.)]\s+`)

// stripHeadingMarkers toglie la sintassi che precede il titolo vero:
// i cancelletti di un heading markdown, gli asterischi o i trattini bassi
// dell'enfasi, e la numerazione di sezione.
//
// Non è un dettaglio cosmetico: il prompt di trascrizione (internal/ai)
// chiede esplicitamente il markdown e di conservare i titoli, quindi da una
// pagina scansionata arriva "## Fase di Upkeep" e non "Fase di Upkeep".
// Senza questa ripulitura il primo rune è '#', il controllo sull'iniziale
// maiuscola fallisce e OGNI pagina di un manuale scansionato resta senza
// titolo — cioè l'indice del manuale, che esiste per risparmiare al modello
// la chiamata esplorativa al tool, sparisce proprio sui manuali lunghi, che
// sono quelli che il tool lo usano davvero.
func stripHeadingMarkers(s string) string {
	// TrimRight oltre a TrimLeft: un heading ATX può essere chiuso
	// ("## Titolo ##") e l'enfasi lo è sempre ("**Titolo**").
	s = strings.Trim(s, "#*_ \t")
	return strings.TrimSpace(headingOrdinal.ReplaceAllString(s, ""))
}

// DetectHeading restituisce il titolo di sezione di una pagina, o stringa
// vuota. Un titolo è la prima riga — spogliata dei marcatori markdown e
// della numerazione — quando è corta, non finisce con un punto e comincia in
// maiuscolo: sono le tre proprietà che distinguono "Fase di Upkeep" da
// "La partita termina quando...". I marcatori si tolgono prima, mai al posto
// dei tre controlli: "## Il gioco finisce qui." resta una frase compiuta e
// va rifiutata come lo era senza cancelletti.
//
// Alimenta l'indice del manuale iniettato nel prompt, che è ciò che evita
// al modello la chiamata esplorativa al tool.
func DetectHeading(text string) string {
	first := strings.TrimSpace(text)
	if first == "" {
		return ""
	}
	if idx := strings.IndexByte(first, '\n'); idx >= 0 {
		first = strings.TrimSpace(first[:idx])
	}
	first = stripHeadingMarkers(first)
	if first == "" {
		return ""
	}
	if len(strings.Fields(first)) > headingWords {
		return ""
	}
	if strings.HasSuffix(first, ".") {
		return ""
	}
	r := []rune(first)[0]
	if !unicode.IsUpper(r) {
		return ""
	}
	return first
}
