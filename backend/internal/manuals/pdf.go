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
	"strings"

	"github.com/ledongthuc/pdf"
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

// repeatedBlank e blankLines sono compilate a livello di package: sono
// usate una volta per pagina da normalizeWhitespace, e ricompilarle a ogni
// chiamata rifarebbe lo stesso lavoro per ogni pagina di ogni manuale.
var (
	repeatedBlank = regexp.MustCompile(`[ \t]+`)
	blankLines    = regexp.MustCompile(`\n{3,}`)
)

// ExtractText legge il layer testo di un PDF, una Page per pagina.
//
// Non ricostruisce l'impaginazione: su un manuale a più colonne le colonne
// possono uscire interlacciate. È un limite accettato, non un difetto da
// aggirare qui — un manuale che esce male si manda per il percorso vision,
// che su impaginazioni dense dà comunque risultati migliori di qualunque
// estrattore di testo.
//
// Limite noto e accettato — una entry xref corrotta blocca il testo anche
// delle pagine sane dopo di lei, non solo di quella corrotta: Page(n) di
// ledongthuc/pdf deve attraversare in ordine tutte le entry di Kids da 0 a
// n per distinguere foglie Page da sotto-alberi Pages con un proprio Count,
// quindi una entry corrotta a posizione i fa fallire Page(n) per ogni
// n >= i, sempre — anche riaprendo il PDF da zero, perché resolve() non
// tiene nessuna cache a livello di reader: è la stessa lettura degli stessi
// byte che ripete lo stesso esito. Non è un bug del recover per-pagina qui
// sotto, è una proprietà strutturale di come la libreria cammina l'albero
// delle pagine — verificato leggendone il sorgente, non solo osservato.
// Correggerlo camminando l'array Kids da soli, bypassando Page(n),
// funzionerebbe solo per il caso comune (Kids piatto di sole foglie Page) e
// rischierebbe l'errore che qui conta di più: un numero di pagina sbagliato
// in una citazione letta al tavolo. Il rimedio previsto non è qui: un
// manuale che con questo estrattore produce poco o niente testo va
// trascritto dal percorso vision (ExtractPageImages), che non tocca
// l'albero delle pagine e non ha questo limite.
//
// Invariante: len(pages) == numero di pagine del PDF, sempre — anche
// quando una pagina non produce testo (perché non esiste nell'albero
// Pages, perché GetPlainText segnala un errore, o perché il parser va in
// panic su quella pagina, incluse tutte le pagine successive per il limite
// descritto sopra). Una pagina mancante deve essere visibile come buco
// nell'anteprima pagina-per-pagina che l'admin userà per riempirlo a mano
// (Task 8), non sparire silenziosamente facendo scivolare la numerazione
// delle pagine successive.
func ExtractText(raw []byte) ([]Page, error) {
	reader, total, err := openPDF(raw)
	if err != nil {
		return nil, err
	}

	pages := make([]Page, 0, total)
	for n := 1; n <= total; n++ {
		pages = append(pages, Page{Number: n, Text: normalizeWhitespace(extractPageText(reader, n))})
	}
	return pages, nil
}

// openPDF apre il reader e legge il numero di pagine, contenendo i panic:
// ledongthuc/pdf è un parser a basso livello che non fa controlli
// difensivi sui bound e va in panic — non in errore — quando la
// cross-reference table o l'albero Pages sono corrotti in un modo che il
// resolver non si aspetta. Qui succede prima che il ciclo per pagina
// esista, quindi va convertito in un errore normale invece di far
// crashare l'intera richiesta (il router monta middleware.Recoverer, ma
// perdere l'intero manuale per una xref corrotta resta un esito peggiore
// di un errore gestito).
func openPDF(raw []byte) (reader *pdf.Reader, total int, err error) {
	defer func() {
		if r := recover(); r != nil {
			reader, total, err = nil, 0, fmt.Errorf("apertura pdf: panic nel parser: %v", r)
		}
	}()
	reader, err = pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, 0, fmt.Errorf("apertura pdf: %w", err)
	}
	return reader, reader.NumPage(), nil
}

// extractPageText estrae il testo della pagina n, contenendo i panic allo
// stesso modo di openPDF: una singola pagina con un content stream o un
// riferimento xref corrotto non deve costare le altre pagine del manuale,
// esattamente come ExtractPageImages salta una singola immagine
// indecodificabile senza abortire l'intero file. Il recover è per-pagina
// (un defer dentro questa funzione, richiamata una volta per iterazione)
// apposta: un recover messo direttamente nel corpo del ciclo di
// ExtractText scatterebbe solo all'uscita dell'intera funzione, non
// all'uscita di ogni iterazione, e un secondo panic su una pagina
// successiva non verrebbe più contenuto.
func extractPageText(reader *pdf.Reader, n int) (text string) {
	defer func() {
		if recover() != nil {
			text = ""
		}
	}()
	page := reader.Page(n)
	if page.V.IsNull() {
		return ""
	}
	// GetPlainText vuole una mappa di font condivisa fra le pagine:
	// passarne una nuova per pagina rifarebbe lo stesso lavoro N volte.
	t, err := page.GetPlainText(nil)
	if err != nil {
		return ""
	}
	return t
}

// normalizeWhitespace compatta gli spazi ripetuti e uniforma gli a capo,
// senza fondere i paragrafi: il chunking (chunk.go) taglia sui paragrafi,
// quindi la riga vuota fra due paragrafi è informazione da conservare.
func normalizeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = repeatedBlank.ReplaceAllString(s, " ")
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
