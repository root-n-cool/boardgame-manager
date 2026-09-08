// Package manuals gestisce il ciclo di vita del testo di un manuale di
// gioco: come si estrae da un PDF, come si spezza in chunk cercabili e come
// si cerca dentro con FTS5.
package manuals

import (
	"bytes"
	"fmt"
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

// textOperators trova gli operatori PDF che disegnano testo: `(...) Tj`,
// `[...] TJ`, `' ` e `"`. La loro presenza è ciò che distingue un PDF con
// layer testo da una scansione, dove le pagine sono solo immagini.
var textOperators = regexp.MustCompile(`\)\s*Tj|\]\s*TJ|\)\s*'|\)\s*"`)

// HasTextLayer dice se vale la pena provare l'estrazione testo. Guarda i
// content stream non compressi; un PDF che comprime tutto in FlateDecode
// risponde false e finisce sul percorso vision, che è la degradazione
// giusta: peggio sarebbe estrarre stringa vuota e non accorgersene.
func HasTextLayer(pdf []byte) bool {
	return textOperators.Match(pdf)
}

// dctImage individua un XObject immagine con filtro DCTDecode e cattura
// larghezza e altezza dal dizionario. I JPEG dentro un PDF non sono
// ricodificati: il flusso è il file JPEG, quindi estrarlo è una copia.
var dctImage = regexp.MustCompile(
	`(?s)/Subtype\s*/Image(.{0,400}?)/Filter\s*/DCTDecode(.{0,400}?)stream\r?\n`)

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
	matches := dctImage.FindAllSubmatchIndex(pdf, -1)
	out := make([]PageImage, 0, len(matches))
	for _, m := range matches {
		// m[1] è la fine dell'intero match, cioè subito dopo "stream\n".
		start := m[1]
		end := bytes.Index(pdf[start:], []byte("endstream"))
		if end < 0 {
			return nil, fmt.Errorf("immagine a pagina %d: stream senza endstream", len(out)+1)
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
			Number: len(out) + 1,
			JPEG:   jpg,
			Width:  cfg.Width,
			Height: cfg.Height,
		})
	}
	return out, nil
}
