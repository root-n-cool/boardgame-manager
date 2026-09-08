package manuals_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

// buildPDF assembla un PDF valido, xref compresa, dagli oggetti dati.
// objs[i] è il corpo dell'oggetto numero i+1, già serializzato.
func buildPDF(t *testing.T, objs []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf,
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objs)+1, xref)
	return buf.Bytes()
}

// tinyJPEG restituisce un JPEG valido di w x h, decodificabile da image/jpeg.
func tinyJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// imgObj serializza un XObject immagine DCTDecode: dizionario più stream
// JPEG grezzo, esattamente come appare in un PDF vero (il flusso non è
// mai ricodificato).
func imgObj(jpg []byte, w, h int) string {
	return fmt.Sprintf(
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream",
		w, h, len(jpg), jpg)
}

// scannedPDF: due pagine, ognuna un solo XObject JPEG a piena pagina.
// È la forma del manuale reale del club, verificata in fase di design.
func scannedPDF(t *testing.T) []byte {
	t.Helper()
	jpg1 := tinyJPEG(t, 24, 32)
	jpg2 := tinyJPEG(t, 20, 28)
	content := "q 200 0 0 260 0 0 cm /Im0 Do Q"
	page := func(imgRef, contentRef string) string {
		return fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] "+
				"/Resources << /XObject << /Im0 %s >> >> /Contents %s >>", imgRef, contentRef)
	}
	streamObj := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	return buildPDF(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",               // 1
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>", // 2
		page("5 0 R", "7 0 R"),                            // 3
		page("6 0 R", "8 0 R"),                            // 4
		imgObj(jpg1, 24, 32),                              // 5
		imgObj(jpg2, 20, 28),                              // 6
		streamObj(content),                                // 7
		streamObj(content),                                // 8
	})
}

// textPDF: una pagina con un vero layer testo, font standard non embeddato.
func textPDF(t *testing.T) []byte {
	t.Helper()
	content := "BT /F1 12 Tf 20 200 Td (Fase di Upkeep) Tj 0 -20 Td (Ogni giocatore paga una moneta.) Tj ET"
	return buildPDF(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})
}

func TestHasTextLayer(t *testing.T) {
	if manuals.HasTextLayer(scannedPDF(t)) {
		t.Fatal("una scansione non ha layer testo, ma HasTextLayer ha detto sì")
	}
	if !manuals.HasTextLayer(textPDF(t)) {
		t.Fatal("un PDF con operatori Tj ha layer testo, ma HasTextLayer ha detto no")
	}
}

// TestHasTextLayer_IsNotFooledByBinaryImageData codifica la lezione del
// manuale reale: dentro i byte binari di un JPEG le sequenze `)'` e `)"`
// compaiono per puro caso, e un tempo bastavano da sole a far dire "ha
// layer testo" a una scansione pura. Qui non c'è nessun /Font: deve
// vincere l'assenza del font, non la coincidenza sui byte.
func TestHasTextLayer_IsNotFooledByBinaryImageData(t *testing.T) {
	binaryImageData := []byte(
		"%PDF-1.4\n1 0 obj\n<< /Type /XObject /Subtype /Image /Filter /DCTDecode >>\n" +
			"stream\n\xff\xd8\xff\xe0\x00\x10JFIF)'\x00\x01\x02)\"\xff\xd9\nendstream\nendobj\n")
	if manuals.HasTextLayer(binaryImageData) {
		t.Fatal("byte binari con )' e )\" ma senza /Font non hanno layer testo, ma HasTextLayer ha detto sì")
	}
}

// TestHasTextLayer_RequiresBothConditions pinna la congiunzione: ogni
// fixture usata altrove nel file ha o entrambe le condizioni o nessuna
// delle due, quindi da sola la suite passerebbe anche se HasTextLayer
// degradasse silenziosamente a una sola delle due condizioni. Qui invece
// ciascun caso ne ha esattamente una.
func TestHasTextLayer_RequiresBothConditions(t *testing.T) {
	fontOnly := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	if manuals.HasTextLayer(fontOnly) {
		t.Fatal("/Font senza Tj/TJ non ha layer testo, ma HasTextLayer ha detto sì")
	}

	tjOnly := []byte("%PDF-1.4\n1 0 obj\n<< /Length 10 >>\nstream\n(ciao) Tj\nendstream\nendobj\n")
	if manuals.HasTextLayer(tjOnly) {
		t.Fatal("Tj senza /Font non ha layer testo, ma HasTextLayer ha detto sì")
	}
}

func TestExtractText_ReadsOnePageOfRealText(t *testing.T) {
	pages, err := manuals.ExtractText(textPDF(t))
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("attesa 1 pagina, ottenute %d", len(pages))
	}
	if pages[0].Number != 1 {
		t.Fatalf("numero pagina atteso 1, ottenuto %d", pages[0].Number)
	}
	if !strings.Contains(pages[0].Text, "Upkeep") {
		t.Fatalf("il testo della pagina non contiene 'Upkeep': %q", pages[0].Text)
	}
	if !strings.Contains(pages[0].Text, "moneta") {
		t.Fatalf("il testo della pagina non contiene la seconda riga: %q", pages[0].Text)
	}
}

func TestExtractText_OnAScanReturnsNoText(t *testing.T) {
	pages, err := manuals.ExtractText(scannedPDF(t))
	// Una scansione può far restituire pagine vuote o un errore di parsing:
	// entrambi sono esiti accettabili. Ciò che NON deve accadere è tornare
	// testo inventato, o andare in panic.
	if err != nil {
		return
	}
	for _, p := range pages {
		if strings.TrimSpace(p.Text) != "" {
			t.Fatalf("pagina %d di una scansione ha prodotto testo: %q", p.Number, p.Text)
		}
	}
}

// corruptThirdPageXref costruisce un PDF di 3 pagine di testo valide e poi
// corrompe, byte a byte, l'unica entry della cross-reference table che
// punta alla terza pagina: invece del suo vero offset, ce ne scrive uno
// che punta al vero inizio dell'oggetto 4 (la seconda pagina). L'offset
// non è zero (la libreria tratta zero come "voce libera" e restituisce un
// valore nullo senza leggere nulla), quindi il resolver tenta comunque la
// lettura, trova "4 0 obj" dove si aspettava "5 0 obj" e va in panic — è
// esattamente la classe di corruzione xref di cui parla la review: bassa
// verosimiglianza di orchestrarla a mano, ma un PDF scaricato a metà o
// generato da uno strumento bacato produce xref sbagliate di questo tipo
// tutti i giorni.
func corruptThirdPageXref(t *testing.T) []byte {
	t.Helper()
	content := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	page := func(contentsRef string) string {
		return "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " +
			"/Resources << /Font << /F1 9 0 R >> >> /Contents " + contentsRef + " >>"
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",                     // 1
		"<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>", // 2
		page("6 0 R"),                       // 3: buona
		page("7 0 R"),                       // 4: buona
		page("8 0 R"),                       // 5: la sua xref entry verrà corrotta
		content("BT /F1 12 Tf (uno) Tj ET"), // 6
		content("BT /F1 12 Tf (due) Tj ET"), // 7
		content("BT /F1 12 Tf (tre) Tj ET"), // 8
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", // 9
	}
	pdf := buildPDF(t, objs)

	// Ricalcola l'offset reale dell'oggetto 4 con la stessa formula usata
	// da buildPDF, per scriverlo al posto di quello dell'oggetto 5.
	pos := len("%PDF-1.4\n")
	var obj4Offset int
	for i, body := range objs {
		if i+1 == 4 {
			obj4Offset = pos
		}
		pos += len(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, body))
	}

	entriesHeader := fmt.Sprintf("xref\n0 %d\n", len(objs)+1)
	idx := bytes.Index(pdf, []byte(entriesHeader))
	if idx < 0 {
		t.Fatalf("intestazione xref non trovata")
	}
	// Ogni entry xref occupa 20 byte fissi ("%010d 00000 n \n"); l'entry 0
	// è quella libera, l'entry N è l'oggetto N.
	const entryWidth = 20
	entryStart := idx + len(entriesHeader) + 5*entryWidth
	copy(pdf[entryStart:entryStart+10], fmt.Sprintf("%010d", obj4Offset))

	return pdf
}

// TestExtractText_ContainedPanicOnCorruptXref è il caso da contenere
// segnalato in review: ledongthuc/pdf è un parser a basso livello che va
// in panic (non in errore) quando la cross-reference table è corrotta in
// un modo che il resolver non si aspetta. Qui la corruzione colpisce solo
// la terza pagina: le prime due devono restare leggibili, la terza deve
// arrivare come Page vuota, e soprattutto ExtractText non deve mai far
// andare in panic il chiamante.
func TestExtractText_ContainedPanicOnCorruptXref(t *testing.T) {
	pages, err := manuals.ExtractText(corruptThirdPageXref(t))
	if err != nil {
		// Anche un errore, invece di pagine parziali, sarebbe un esito
		// accettabile: quello che non è accettabile è il panic.
		return
	}
	if len(pages) != 3 {
		t.Fatalf("attese 3 pagine (l'invariante len(pages)==pagine del PDF), ottenute %d", len(pages))
	}
	if !strings.Contains(pages[0].Text, "uno") {
		t.Fatalf("pagina 1 doveva restare leggibile: %q", pages[0].Text)
	}
	if !strings.Contains(pages[1].Text, "due") {
		t.Fatalf("pagina 2 doveva restare leggibile: %q", pages[1].Text)
	}
	if pages[2].Number != 3 {
		t.Fatalf("la pagina corrotta deve restare numerata 3, ottenuto %d", pages[2].Number)
	}
	if strings.TrimSpace(pages[2].Text) != "" {
		t.Fatalf("la pagina corrotta doveva arrivare vuota, non con testo inventato: %q", pages[2].Text)
	}
}

// corruptMiddlePageXref è la variante di corruptThirdPageXref che manca:
// qui il fixture ha 5 pagine e la corruzione colpisce la terza, in mezzo
// alle altre quattro, non l'ultima. Serve a esercitare cosa succede alle
// pagine *dopo* quella corrotta — vedi
// TestExtractText_StopsYieldingTextAtACorruptPage per cosa succede
// davvero e perché. Stessa tecnica byte-a-byte di corruptThirdPageXref:
// la entry xref dell'oggetto 5 (terza pagina) viene sovrascritta con
// l'offset vero dell'oggetto 4 (seconda pagina), cosa che fa andare in
// panic il resolver quando prova a leggere la terza pagina.
func corruptMiddlePageXref(t *testing.T) []byte {
	t.Helper()
	content := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	page := func(contentsRef string) string {
		return "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " +
			"/Resources << /Font << /F1 13 0 R >> >> /Contents " + contentsRef + " >>"
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",                                 // 1
		"<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R 6 0 R 7 0 R] /Count 5 >>", // 2
		page("8 0 R"),                           // 3: buona (pagina 1)
		page("9 0 R"),                           // 4: buona (pagina 2)
		page("10 0 R"),                          // 5: la sua xref entry verrà corrotta (pagina 3)
		page("11 0 R"),                          // 6: buona (pagina 4)
		page("12 0 R"),                          // 7: buona (pagina 5)
		content("BT /F1 12 Tf (uno) Tj ET"),     // 8
		content("BT /F1 12 Tf (due) Tj ET"),     // 9
		content("BT /F1 12 Tf (tre) Tj ET"),     // 10
		content("BT /F1 12 Tf (quattro) Tj ET"), // 11
		content("BT /F1 12 Tf (cinque) Tj ET"),  // 12
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", // 13
	}
	pdf := buildPDF(t, objs)

	// Stessa formula di corruptThirdPageXref: ricalcola l'offset reale
	// dell'oggetto 4 e lo scrive al posto di quello dell'oggetto 5.
	pos := len("%PDF-1.4\n")
	var obj4Offset int
	for i, body := range objs {
		if i+1 == 4 {
			obj4Offset = pos
		}
		pos += len(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, body))
	}

	entriesHeader := fmt.Sprintf("xref\n0 %d\n", len(objs)+1)
	idx := bytes.Index(pdf, []byte(entriesHeader))
	if idx < 0 {
		t.Fatalf("intestazione xref non trovata")
	}
	const entryWidth = 20
	entryStart := idx + len(entriesHeader) + 5*entryWidth
	copy(pdf[entryStart:entryStart+10], fmt.Sprintf("%010d", obj4Offset))

	return pdf
}

// TestExtractText_StopsYieldingTextAtACorruptPage pin una limitazione nota
// e accettata, non una promessa che l'estrazione recuperi tutto: quando una
// entry xref è corrotta, ledongthuc/pdf.(*Reader).Page(n) deve attraversare
// in ordine tutte le entry di Kids da 0 a n per sapere, leggendo il /Type di
// ciascuna, se sono foglie Page o sotto-alberi Pages con un proprio Count —
// non può saltare direttamente all'indice n. Quindi una entry corrotta a
// posizione i fa fallire la risoluzione di **ogni** Page(n) con n >= i,
// sempre, anche su un *pdf.Reader riaperto da zero sugli stessi byte:
// resolve() (in ledongthuc/pdf/read.go) non tiene nessuna cache a livello
// di reader, quindi non c'è stato da "ripulire" riaprendo. Verificato
// leggendo il sorgente della libreria e chiamandola direttamente, bypassando
// il nostro wrapper: reader.Page(3), reader.Page(4) e reader.Page(5) su
// questo fixture panicano tutti con lo stesso identico "loading {5 0}: found
// {4 0}", pur riferendosi a oggetti diversi e sani.
//
// Una correzione vera richiederebbe di non delegare più a Page(n) e
// camminare noi stessi l'array Kids di primo livello (Value.Index(i)
// risolve solo l'entry i, indipendentemente dalle altre) — ma solo per il
// caso comune di un Kids piatto di sole foglie Page; un Kids con
// sotto-alberi Pages annidati richiederebbe la stessa logica di conteggio
// ricorsivo della libreria. Si è deciso di non farlo (vedi il commento su
// ExtractText): il percorso vision (ExtractPageImages, codice nostro, non
// tocca l'albero delle pagine) è la rete di sicurezza già prevista per un
// PDF che produce poco o niente testo, e reimplementare la risoluzione
// dell'albero pagine contro gli interni di una libreria terza rischierebbe
// l'unico errore che qui conta davvero: un numero di pagina sbagliato in
// una citazione che qualcuno legge al tavolo.
//
// Quello che DEVE restare vero, e che questo test pin: nessun panic esce
// mai da ExtractText; le pagine prima della corrotta arrivano con il loro
// vero testo; le pagine dalla corrotta in poi arrivano presenti (non
// spariscono, non slittano) ma con testo vuoto; la numerazione resta
// sempre fedele al PDF, quindi len(pages) == total non si rompe mai in
// questo scenario (si rompe solo se riaprire il PDF stesso fallisce, vedi
// ExtractText).
func TestExtractText_StopsYieldingTextAtACorruptPage(t *testing.T) {
	pages, err := manuals.ExtractText(corruptMiddlePageXref(t))
	if err != nil {
		t.Fatalf("un panic recuperato non deve mai diventare un errore di ExtractText qui: %v", err)
	}
	if len(pages) != 5 {
		t.Fatalf("attese 5 pagine (l'invariante len(pages)==pagine del PDF), ottenute %d", len(pages))
	}
	if !strings.Contains(pages[0].Text, "uno") {
		t.Fatalf("pagina 1, prima della corrotta, doveva restare leggibile: %q", pages[0].Text)
	}
	if !strings.Contains(pages[1].Text, "due") {
		t.Fatalf("pagina 2, prima della corrotta, doveva restare leggibile: %q", pages[1].Text)
	}
	// Dalla pagina corrotta in poi: presenti, numerate correttamente, ma
	// senza testo. È il limite descritto sopra, non un bug di questo test.
	for i, want := range []int{3, 4, 5} {
		if pages[i+2].Number != want {
			t.Fatalf("pagina %d deve restare numerata %d anche senza testo, ottenuto %d",
				want, want, pages[i+2].Number)
		}
		if strings.TrimSpace(pages[i+2].Text) != "" {
			t.Fatalf("pagina %d (corrotta o successiva) doveva arrivare vuota, non con testo inventato: %q",
				want, pages[i+2].Text)
		}
	}
}

// TestExtractText_OnEmptyInput copre il caso limite più ovvio: byte non
// validi come PDF (incluso nil) non devono far andare in panic, solo
// restituire un errore e nessuna pagina.
func TestExtractText_OnEmptyInput(t *testing.T) {
	if pages, err := manuals.ExtractText(nil); err == nil {
		t.Fatalf("input nil doveva restituire un errore, ottenute %d pagine", len(pages))
	}
	if pages, err := manuals.ExtractText([]byte{}); err == nil {
		t.Fatalf("input vuoto doveva restituire un errore, ottenute %d pagine", len(pages))
	}
}

func TestExtractPageImages_ReturnsOneJPEGPerScannedPage(t *testing.T) {
	imgs, err := manuals.ExtractPageImages(scannedPDF(t))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("attese 2 immagini, ottenute %d", len(imgs))
	}
	if imgs[0].Number != 1 || imgs[1].Number != 2 {
		t.Fatalf("numerazione pagine sbagliata: %d, %d", imgs[0].Number, imgs[1].Number)
	}
	if imgs[0].Width != 24 || imgs[0].Height != 32 {
		t.Fatalf("dimensioni pagina 1 sbagliate: %dx%d", imgs[0].Width, imgs[0].Height)
	}
	// Il byte stream deve essere un JPEG davvero decodificabile: è quello
	// che finisce, in base64, nella richiesta al modello multimodale.
	if _, err := jpeg.Decode(bytes.NewReader(imgs[0].JPEG)); err != nil {
		t.Fatalf("la pagina 1 non è un JPEG valido: %v", err)
	}
}

func TestExtractPageImages_OnAPDFWithoutImages(t *testing.T) {
	imgs, err := manuals.ExtractPageImages(textPDF(t))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(imgs) != 0 {
		t.Fatalf("attese 0 immagini su un PDF di solo testo, ottenute %d", len(imgs))
	}
}

// TestExtractPageImages_SkipsUndecodableImageButKeepsPageNumbers verifica
// che Number segua la posizione nel file (i+1 sul totale dei match), non
// il conteggio delle estrazioni riuscite: se la pagina 2 viene scartata,
// la pagina 3 deve restare "3", non scivolare a "2". Quel numero finisce
// nella citazione mostrata a chi sta dirimendo una regola al tavolo.
func TestExtractPageImages_SkipsUndecodableImageButKeepsPageNumbers(t *testing.T) {
	jpg1 := tinyJPEG(t, 24, 32)
	jpg3 := tinyJPEG(t, 20, 28)
	corrupt := []byte("questi byte dichiarano DCTDecode ma non sono un JPEG valido")
	pdf := buildPDF(t, []string{
		imgObj(jpg1, 24, 32),    // pagina 1: buona
		imgObj(corrupt, 10, 10), // pagina 2: non decodifica, va scartata
		imgObj(jpg3, 20, 28),    // pagina 3: buona
	})

	imgs, err := manuals.ExtractPageImages(pdf)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("attese 2 immagini valide su 3, ottenute %d", len(imgs))
	}
	if imgs[0].Number != 1 || imgs[1].Number != 3 {
		t.Fatalf("la pagina corrotta ha fatto scivolare la numerazione: ottenuti %d e %d, attesi 1 e 3",
			imgs[0].Number, imgs[1].Number)
	}
}

// TestExtractPageImages_SkipsPageMissingEndstream copre lo stesso
// principio della corruzione JPEG ma per un flusso tronco: una pagina
// senza "endstream" non deve costare le altre pagine del manuale.
func TestExtractPageImages_SkipsPageMissingEndstream(t *testing.T) {
	jpg1 := tinyJPEG(t, 24, 32)
	jpg2 := tinyJPEG(t, 20, 28)
	// Nessun "endstream" da nessuna parte dopo questo: un file scaricato
	// a metà avrebbe questa forma.
	truncated := "<< /Type /XObject /Subtype /Image /Width 10 /Height 10 " +
		"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length 3 >>\nstream\nabc"
	pdf := buildPDF(t, []string{
		imgObj(jpg1, 24, 32), // pagina 1: buona
		imgObj(jpg2, 20, 28), // pagina 2: buona
		truncated,            // pagina 3: tronca, va scartata
	})

	imgs, err := manuals.ExtractPageImages(pdf)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("attese le 2 pagine buone nonostante la terza tronca, ottenute %d", len(imgs))
	}
	if imgs[0].Number != 1 || imgs[1].Number != 2 {
		t.Fatalf("numerazione sbagliata: %d, %d", imgs[0].Number, imgs[1].Number)
	}
}

// TestExtractPageImages_OnTheRealManual gira solo se in ./data c'è un
// manuale scansionato: è la verifica che la logica regge su un file
// prodotto da uno scanner, non solo sulla fixture. Salta in CI.
func TestExtractPageImages_OnTheRealManual(t *testing.T) {
	paths, _ := filepath.Glob("../../../data/uploads/*.pdf")
	if len(paths) == 0 {
		t.Skip("nessun PDF in ./data/uploads: verifica saltata")
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		hasText := manuals.HasTextLayer(raw)
		t.Logf("%s: HasTextLayer=%v", filepath.Base(p), hasText)
		if hasText {
			// Un vero layer testo andrebbe sul percorso del Task 3: qui si
			// verifica solo il percorso immagine, ma il manuale del club è
			// una scansione pura, quindi ci si aspetta false. Se qui esce
			// true, è esattamente il falso positivo segnalato in review
			// (byte binari del JPEG letti come operatori di testo).
			t.Errorf("%s: attesa una scansione pura (HasTextLayer=false), ottenuto true", p)
			continue
		}
		imgs, err := manuals.ExtractPageImages(raw)
		if err != nil {
			t.Fatalf("extract %s: %v", p, err)
		}
		if len(imgs) == 0 {
			t.Fatalf("%s è una scansione ma non ne è uscita nessuna pagina", p)
		}
		t.Logf("%s: %d pagine, la prima %dx%d", filepath.Base(p), len(imgs), imgs[0].Width, imgs[0].Height)
	}
}
