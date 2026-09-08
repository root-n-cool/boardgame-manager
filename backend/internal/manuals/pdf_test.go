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
