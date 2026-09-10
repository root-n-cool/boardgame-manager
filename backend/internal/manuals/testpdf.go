package manuals

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
)

// Questo file esiste solo per i test: costruisce PDF minimi ma validi,
// byte a byte, usati sia dai test di questo package (pdf_test.go) sia da
// quelli di internal/httpapi (che devono esercitare le rotte admin su un
// manuale scansionato vero). Non è un file _test.go perché gli helper di
// test di un package non sono importabili dai test di un altro package:
// tenerne una sola implementazione qui evita che le due suite finiscano
// con due copie della stessa costruzione byte-a-byte del PDF, destinate a
// divergere silenziosamente nel tempo.

// BuildTestPDF assembla un PDF valido, xref compresa, dai corpi di oggetto
// dati. objs[i] diventa l'oggetto numero i+1, serializzato così com'è.
func BuildTestPDF(objs []string) []byte {
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

// NewTestJPEG restituisce un JPEG valido di w x h, decodificabile da
// image/jpeg. Va in panic su un errore di codifica: può succedere solo per
// un bug in questo fixture, mai per un input esterno, quindi un panic a
// tempo di test (con tanto di stack trace) è preferibile a portarsi dietro
// un *testing.T solo per un percorso che in pratica non fallisce mai.
func NewTestJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic("manuals: build fixture jpeg: " + err.Error())
	}
	return buf.Bytes()
}

// ImageObject serializza un XObject immagine DCTDecode: dizionario più
// stream JPEG grezzo, esattamente come appare in un PDF vero (il flusso
// non è mai ricodificato).
func ImageObject(jpg []byte, w, h int) string {
	return fmt.Sprintf(
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream",
		w, h, len(jpg), jpg)
}

// NewScannedPDF restituisce un manuale scansionato di due pagine, ognuna un
// solo XObject JPEG a piena pagina e nessun layer testo. È la forma del
// manuale reale del club, verificata in fase di design, e il fixture che
// entrambi i package usano per esercitare il percorso vision.
func NewScannedPDF() []byte {
	return NewScannedPDFPages(2)
}

// NewScannedPDFPages è NewScannedPDF con un numero di pagine a scelta. Serve
// ai test in cui l'esito di UNA pagina in mezzo alle altre deve poter essere
// diverso: con due sole pagine la pagina "diversa" è per forza la prima o
// l'ultima, e un test così non distingue una numerazione presa da img.Number
// da una presa dall'indice di append.
//
// Ogni pagina ha un'immagine di dimensioni diverse dalle altre. Nessun test
// esistente lo verifica oggi (fakeTranscriber in internal/httpapi ignora i
// byte JPEG ed echeggia solo l'argomento page): la variazione resta qui
// pronta per un futuro test che debba distinguere le pagine anche
// dall'immagine, non dal solo numero.
func NewScannedPDFPages(n int) []byte {
	content := "q 200 0 0 260 0 0 cm /Im0 Do Q"
	page := func(imgRef, contentRef string) string {
		return fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] "+
				"/Resources << /XObject << /Im0 %s >> >> /Contents %s >>", imgRef, contentRef)
	}
	streamObj := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}

	// Numerazione degli oggetti: 1 catalogo, 2 albero pagine, poi le n
	// pagine, poi le n immagini, poi gli n contenuti.
	firstPage, firstImage, firstContent := 3, 3+n, 3+2*n
	kids := make([]string, 0, n)
	pages := make([]string, 0, n)
	images := make([]string, 0, n)
	contents := make([]string, 0, n)
	for i := 0; i < n; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", firstPage+i))
		pages = append(pages, page(
			fmt.Sprintf("%d 0 R", firstImage+i), fmt.Sprintf("%d 0 R", firstContent+i)))
		w, h := 24-4*i, 32-4*i
		images = append(images, ImageObject(NewTestJPEG(w, h), w, h))
		contents = append(contents, streamObj(content))
	}

	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n),
	}
	objs = append(objs, pages...)
	objs = append(objs, images...)
	objs = append(objs, contents...)
	return BuildTestPDF(objs)
}

// NewScannedPDFPageSized restituisce un manuale scansionato di UNA pagina
// con l'immagine delle dimensioni date. Serve al test che verifica la
// riduzione delle pagine prima dell'invio al modello (vedi
// manuals.Downscale): i fixture qui sopra sono volutamente minuscoli — 24x32
// e giù di lì — e cadono tutti nel ramo "già sotto la soglia", quindi con
// quelli un aggancio rimosso non si noterebbe.
func NewScannedPDFPageSized(w, h int) []byte {
	content := "q 200 0 0 260 0 0 cm /Im0 Do Q"
	return BuildTestPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " +
			"/Resources << /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
		ImageObject(NewTestJPEG(w, h), w, h),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	})
}

// NewTextPDF restituisce un PDF di una pagina con un vero layer testo: un
// font standard non embeddato e due righe disegnate con Tj. È il fixture
// che entrambi i package usano per esercitare il percorso di estrazione
// testo.
func NewTextPDF() []byte {
	content := "BT /F1 12 Tf 20 200 Td (Fase di Upkeep) Tj 0 -20 Td (Ogni giocatore paga una moneta.) Tj ET"
	return BuildTestPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})
}
