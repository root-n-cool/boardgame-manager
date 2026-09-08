package manuals_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
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

// scannedPDF: due pagine, ognuna un solo XObject JPEG a piena pagina.
// È la forma del manuale reale del club, verificata in fase di design.
func scannedPDF(t *testing.T) []byte {
	t.Helper()
	jpg1 := tinyJPEG(t, 24, 32)
	jpg2 := tinyJPEG(t, 20, 28)
	content := "q 200 0 0 260 0 0 cm /Im0 Do Q"
	imgObj := func(jpg []byte, w, h int) string {
		return fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
				"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream",
			w, h, len(jpg), jpg)
	}
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
		if manuals.HasTextLayer(raw) {
			continue // questo va sull'altro percorso
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
