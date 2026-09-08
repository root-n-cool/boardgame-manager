// Package manuals gestisce il ciclo di vita del testo di un manuale di
// gioco: come si estrae da un PDF, come si spezza in chunk cercabili e come
// si cerca dentro con FTS5.
package manuals

import (
	"bytes"
	"image"
	_ "image/jpeg" // registra il decoder JPEG per image.DecodeConfig
	"regexp"
)

// Page è una pagina di manuale come testo. Number è 1-based, come la
// numerazione che l'utente legge sul PDF: è quella che finisce nella
// citazione della risposta.
type Page struct {
	Number int
	Text   string
}

// PageImage è una pagina scansionata: il JPEG così com'era dentro il PDF,
// pronto da mandare in base64 a un modello multimodale.
type PageImage struct {
	Number int
	JPEG   []byte
	Width  int
	Height int
}

// textShowingOperator trova gli operatori PDF che disegnano testo:
// `(...) Tj` e `[...] TJ`. Sono volutamente esclusi `'` e `"`: sono rari nei
// content stream reali e, dentro un flusso binario JPEG, la sequenza
// `)'` o `)"` compare per puro caso — è così che un manuale scansionato
// reale di questo progetto veniva letto come "ha layer testo".
var textShowingOperator = regexp.MustCompile(`\)\s*Tj|\]\s*TJ`)

// fontResource trova un riferimento a `/Font` nel documento. Un content
// stream che disegna testo deve appoggiarsi a una risorsa Font dichiarata
// nel dizionario delle risorse della pagina: uno scan puro non ne ha
// nessuna, quindi la sua assenza è una seconda prova indipendente
// dall'operatore di disegno, che da solo può capitare per caso nei byte di
// un'immagine.
var fontResource = regexp.MustCompile(`/Font\b`)

// HasTextLayer dice se vale la pena provare l'estrazione testo. Richiede
// **sia** un operatore di disegno testo **sia** un riferimento a /Font,
// perché il solo operatore basta a produrre falsi positivi: dentro un
// flusso JPEG (dati binari) le sequenze `)'` o `)"` compaiono per caso, e
// così un manuale scansionato del club risultava "con layer testo".
//
// La sbilanciatura è deliberata e va nella direzione sicura: quando è
// incerto, HasTextLayer risponde false e il PDF va sul percorso vision, che
// funziona su qualsiasi PDF (scansione o no) e nel peggiore dei casi costa
// solo una chiamata al modello in più. Rispondere true per errore è invece
// il guasto grave: l'estrazione testo su una scansione restituisce stringa
// vuota, il percorso vision non parte mai, e la funzione tace — nessun
// errore, nessuna citazione, il manuale è muto. Un PDF con vero layer testo
// che nasconde il suo /Font dentro un object stream compresso finirà anche
// lui sul percorso vision: costa una chiamata in più, non rompe niente.
func HasTextLayer(pdf []byte) bool {
	return fontResource.Match(pdf) && textShowingOperator.Match(pdf)
}

// dctImage individua un XObject immagine con filtro DCTDecode. I JPEG
// dentro un PDF non sono ricodificati: il flusso è il file JPEG, quindi
// estrarlo è una copia. I due gruppi fra /Subtype e /Filter e fra /Filter
// e "stream" non catturano nulla: servono solo a delimitare la ricerca,
// le dimensioni vengono da image.DecodeConfig più sotto. Il limite di 400
// byte per gruppo è stato verificato sul manuale reale del club: il
// dizionario XObject di ciascuna delle sue 4 pagine ci sta comodamente,
// e le 4 immagini vengono estratte correttamente.
var dctImage = regexp.MustCompile(
	`(?s)/Subtype\s*/Image(?:.{0,400}?)/Filter\s*/DCTDecode(?:.{0,400}?)stream\r?\n`)

// ExtractPageImages restituisce le immagini a piena pagina di un PDF
// scansionato, nell'ordine in cui compaiono nel file — che per uno scan
// prodotto da uno scanner è l'ordine delle pagine.
//
// Non usa una libreria PDF di proposito: uno scan è "una immagine per
// pagina", e per quel caso bastano il dizionario dell'XObject e una copia
// del flusso. Un PDF con più immagini per pagina qui non è supportato, ed è
// coerente: quello è un PDF impaginato, che ha un layer testo e va
// sull'altro percorso.
func ExtractPageImages(pdf []byte) ([]PageImage, error) {
	matches := dctImage.FindAllIndex(pdf, -1)
	out := make([]PageImage, 0, len(matches))
	for i, m := range matches {
		// Number viene dalla posizione nel file (i+1), non dal numero di
		// immagini estratte con successo finora: se una pagina in mezzo
		// viene scartata, le pagine dopo di lei non devono scivolare
		// indietro di uno. Il numero finisce nella citazione mostrata a
		// chi sta dirimendo una regola al tavolo: sbagliarlo silenziosamente
		// sarebbe peggio che saltare la pagina.
		number := i + 1

		// m[1] è la fine dell'intero match, cioè subito dopo "stream\n".
		start := m[1]
		end := bytes.Index(pdf[start:], []byte("endstream"))
		if end < 0 {
			// Un flusso senza endstream è malformato quanto uno che non
			// decodifica: si salta la singola pagina, non tutto il
			// manuale — vale lo stesso principio del ramo sotto.
			continue
		}
		jpg := bytes.TrimRight(pdf[start:start+end], "\r\n")

		cfg, _, err := image.DecodeConfig(bytes.NewReader(jpg))
		if err != nil {
			// Un flusso che si dichiara DCTDecode ma non è un JPEG leggibile
			// non è utilizzabile dal modello: si salta invece di far
			// fallire tutto il manuale.
			continue
		}

		out = append(out, PageImage{
			Number: number,
			JPEG:   jpg,
			Width:  cfg.Width,
			Height: cfg.Height,
		})
	}
	return out, nil
}
