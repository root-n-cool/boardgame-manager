package manuals

import (
	"regexp"
	"strings"
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
