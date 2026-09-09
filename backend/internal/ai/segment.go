package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SegmentWindowMaxChars è il tetto di caratteri per finestra mandata al
// modello. Un file senza struttura (.txt, o un PDF con layer testo) può
// essere grande quanto tutto il manuale: oltre questa soglia il testo si
// spezza in più finestre (vedi splitIntoWindows), ciascuna segmentata con
// una propria chiamata.
const SegmentWindowMaxChars = 60000

// segmentTimeout è generoso quanto transcribeTimeout: una finestra da
// SegmentWindowMaxChars caratteri è una richiesta lunga quanto una pagina
// scansionata da leggere, e un modello economico non è veloce. Il timeout
// passato qui governa davvero la chiamata (vedi il commento su postRaw in
// ask.go): non è più il campo fisso del client condiviso a decidere.
const segmentTimeout = 120 * time.Second

// maxContentDeviationRatio è la soglia di scarto fra la lunghezza del
// testo (titoli esclusi, spazi normalizzati) prima e dopo la
// segmentazione. Il 15% lascia margine ai titoli aggiunti (che allungano
// il testo) senza tollerare un riassunto mascherato da segmentazione.
const maxContentDeviationRatio = 0.15

// ErrSegmentationRejected dice che la risposta del modello si scosta
// troppo, in lunghezza, dal testo di partenza: probabilmente un riassunto
// invece di una segmentazione. È un errore distinto da un guasto di rete o
// di parsing perché al Task 5 serve dire all'admin "la lettura non è
// affidabile, riprova", non un errore generico — da qui in poi si
// controlla con errors.Is, non con la sola presenza di un errore.
var ErrSegmentationRejected = errors.New("ai segmentation rejected: response length deviates too much from the source text")

// Segmenter è l'astrazione che serve all'ingestione di .txt e PDF con
// layer testo: prende testo piatto, restituisce lo stesso testo con
// titoli markdown ATX inseriti dove comincia una sezione. HTTPClient la
// implementa; i test iniettano un finto.
type Segmenter interface {
	Segment(ctx context.Context, text string) (string, error)
}

// segmentSystemPrompt è il vincolo espresso a parole discusso nello step 1
// del brief: chiede titoli ATX, vieta la riscrittura. Non è una garanzia
// meccanica — per questo la mitigazione vera è il confronto di lunghezza
// in segmentWindow, non questo testo.
const segmentSystemPrompt = "Ricevi un brano di testo grezzo, senza struttura, estratto da un file: è il regolamento di un gioco da tavolo. " +
	"Il tuo compito è inserire titoli in markdown dove comincia una nuova sezione o argomento (per esempio \"Preparazione\", \"Turno di gioco\", \"Fine partita\", \"Punteggio\"). " +
	"Regole assolute:\n" +
	"1. Non riscrivere, riassumere, correggere, tradurre o parafrasare il testo: restituisci ESATTAMENTE lo stesso testo, parola per parola, con la sola aggiunta dei titoli.\n" +
	"2. Ogni titolo va su una riga propria, che comincia con \"## \" (due cancelletti e uno spazio) seguiti dal titolo, subito prima del testo a cui si riferisce.\n" +
	"3. Non aggiungere nient'altro: nessun commento, nessuna nota, nessuna spiegazione che non fosse già nel testo originale.\n" +
	"4. Se il brano non ha sezioni distinte, restituiscilo invariato: non inventare titoli pur di inserirne uno."

// Segment spezza text in finestre da al più SegmentWindowMaxChars
// caratteri e chiede al modello, finestra per finestra, di inserire
// titoli markdown senza toccare il contenuto. L'ultimo titolo di ogni
// finestra passa come contesto alla successiva, perché una sezione può
// proseguire oltre il taglio di finestra e il modello non deve ripeterne
// il titolo o inventarne uno nuovo a metà.
func (c *HTTPClient) Segment(ctx context.Context, text string) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}

	windows := splitIntoWindows(text, SegmentWindowMaxChars)

	var out strings.Builder
	var previousHeading string
	for i, window := range windows {
		segmented, err := c.segmentWindow(ctx, window, previousHeading)
		if err != nil {
			return "", err
		}
		if i > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(segmented)
		if h := lastHeadingLine(segmented); h != "" {
			previousHeading = h
		}
	}
	return out.String(), nil
}

// segmentWindow segmenta una singola finestra e applica la mitigazione:
// se il testo risultante (titoli esclusi, spazi normalizzati) si scosta
// da quello di partenza oltre maxContentDeviationRatio, la risposta è
// rifiutata invece di essere usata — un errore qui è meglio di una regola
// inventata, perché l'admin lo vede (vedi ErrSegmentationRejected).
func (c *HTTPClient) segmentWindow(ctx context.Context, window, previousHeading string) (string, error) {
	system := segmentSystemPrompt
	if previousHeading != "" {
		system += fmt.Sprintf(
			"\n\nQuesto brano prosegue il testo precedente: l'ultima sezione era intitolata %q. "+
				"Se il testo che segue continua ancora quella sezione, non ripetere il titolo; "+
				"inserisci un titolo nuovo solo quando comincia davvero un argomento diverso.",
			previousHeading,
		)
	}

	payload, err := json.Marshal(chatRequest{
		Model:       c.Model,
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: window},
		},
	})
	if err != nil {
		return "", err
	}

	out, err := c.postChat(ctx, payload, segmentTimeout)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)

	if !contentLengthWithinTolerance(window, out) {
		return "", ErrSegmentationRejected
	}
	return out, nil
}

// segmentHeadingLine riconosce una riga di titolo ATX (da # a ######) a
// inizio riga, catturando il testo del titolo. Non serve gestire la forma
// chiusa ("## Titolo ##") o i blocchi di codice recintati come fa
// manuals.ParseSections: qui l'uso è solo interno, per estrarre l'ultimo
// titolo di una finestra e stimare cosa è titolo nel confronto di
// lunghezza — non per l'estrazione delle sezioni vere, che è compito del
// Task 5 su ParseSections.
var segmentHeadingLine = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.+?)[ \t]*$`)

// lastHeadingLine restituisce il testo dell'ultimo titolo ATX presente in
// md, o stringa vuota se non ce n'è nessuno.
func lastHeadingLine(md string) string {
	matches := segmentHeadingLine.FindAllStringSubmatch(md, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1][1])
}

// stripHeadingLines toglie dal testo le righe di titolo ATX, lasciando
// solo il corpo: è quel che permette di confrontare "il contenuto" prima
// e dopo, senza che i titoli aggiunti contino come testo nuovo.
func stripHeadingLines(md string) string {
	return segmentHeadingLine.ReplaceAllString(md, "")
}

// normalizeForComparison collassa ogni sequenza di spazi bianchi (spazi,
// tab, a capo) in un solo spazio e taglia i bordi. Senza normalizzare, un
// modello che cambia solo l'interlinea o aggiunge una riga vuota fra due
// paragrafi farebbe oscillare il confronto di lunghezza per un motivo che
// non ha nulla a che fare con contenuto riscritto o perso.
func normalizeForComparison(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// contentLengthWithinTolerance confronta la lunghezza di original (il
// testo di partenza, già piatto: nessun titolo da togliere) con quella di
// segmented una volta tolti i titoli aggiunti, entrambe con gli spazi
// normalizzati. Un modello che riassume o omette parti fa scendere la
// lunghezza ben oltre maxContentDeviationRatio; i soli titoli aggiunti la
// fanno crescere di poco.
func contentLengthWithinTolerance(original, segmented string) bool {
	origNorm := normalizeForComparison(original)
	segNorm := normalizeForComparison(stripHeadingLines(segmented))

	origLen := len(origNorm)
	if origLen == 0 {
		return len(segNorm) == 0
	}
	diff := len(segNorm) - origLen
	if diff < 0 {
		diff = -diff
	}
	return float64(diff)/float64(origLen) <= maxContentDeviationRatio
}

// paragraphLookback è quanto ci si allontana all'indietro dal limite di
// finestra cercando una riga vuota (un confine di paragrafo) prima di
// arrendersi e tagliare a metà frase. Troppo corto perde confini validi
// appena fuori portata; troppo lungo produce finestre molto più corte del
// limite anche quando un confine più vicino non c'è.
const paragraphLookback = 2000

// splitIntoWindows spezza text in pezzi da al più maxChars caratteri,
// tagliando su un confine di paragrafo ("\n\n") quando ce n'è uno entro
// paragraphLookback caratteri dal limite: un taglio a metà frase
// produrrebbe, nella finestra successiva, un titolo inventato a cavallo
// del punto di sutura, perché il modello vede un frammento di frase senza
// il suo inizio. Quando non c'è un confine abbastanza vicino (un unico
// paragrafo enorme, per esempio) si taglia comunque al limite: rispettare
// il tetto di caratteri vale più che allungare la finestra all'infinito
// in cerca di un confine che potrebbe non esistere.
func splitIntoWindows(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}

	var windows []string
	remaining := text
	for len(remaining) > maxChars {
		cut := -1
		lookbackStart := maxChars - paragraphLookback
		if lookbackStart < 0 {
			lookbackStart = 0
		}
		if idx := strings.LastIndex(remaining[lookbackStart:maxChars], "\n\n"); idx >= 0 {
			cut = lookbackStart + idx + 2 // dopo la riga vuota: il taglio lascia il confine alla finestra corrente
		}
		if cut < 0 {
			cut = maxChars
		}
		windows = append(windows, remaining[:cut])
		remaining = strings.TrimLeft(remaining[cut:], "\n")
	}
	if remaining != "" {
		windows = append(windows, remaining)
	}
	return windows
}
