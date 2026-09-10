package manuals

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
)

// downscaleQuality è la qualità del JPEG ricodificato. Ottanta e non il 75
// di default di Go: un'immagine già ridotta ha più dettaglio per pixel di
// quella da cui viene, e comprimerla di più rimangerebbe in leggibilità
// quello che il ridimensionamento ha già guadagnato in byte. Il grosso del
// risparmio viene dai pixel, non dalla quantizzazione.
const downscaleQuality = 80

// Downscale riduce a maxLongSide il lato lungo di un JPEG, mantenendo le
// proporzioni, e lo ricodifica. Serve al percorso vision: gli image token
// che il modello conta in ingresso dipendono dai pixel, quindi ridurli è
// la leva che accorcia davvero la risposta — non il peso del file, che
// pesa solo sull'upload.
//
// Restituisce SEMPRE byte inviabili al modello. Su un JPEG illeggibile
// torna l'originale insieme all'errore, perché una pagina grande che
// arriva vale più di una pagina persa: è lo stesso principio del "continue"
// in ExtractPageImages, dove una singola immagine guasta non fa fallire
// tutto il manuale.
//
// Non usa golang.org/x/image/draw, che sarebbe una dipendenza nuova:
// boxDownscale è la media a box, che per una riduzione è il filtro
// corretto e non un ripiego (vedi il suo commento).
func Downscale(src []byte, maxLongSide int) ([]byte, error) {
	// DecodeConfig legge solo l'header: se l'immagine è già sotto la
	// soglia si esce senza pagare la decodifica completa, che sul caso
	// più comune (i fixture dei test, le pagine piccole) è tutto il costo.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return src, fmt.Errorf("manuals: header immagine illeggibile: %w", err)
	}
	w, h := fitLongSide(cfg.Width, cfg.Height, maxLongSide)
	if w == cfg.Width && h == cfg.Height {
		return src, nil
	}

	img, err := jpeg.Decode(bytes.NewReader(src))
	if err != nil {
		return src, fmt.Errorf("manuals: immagine illeggibile: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, boxDownscale(img, w, h), &jpeg.Options{Quality: downscaleQuality}); err != nil {
		return src, fmt.Errorf("manuals: ricodifica JPEG: %w", err)
	}
	if buf.Len() >= len(src) {
		// Patologico ma possibile: una scansione compressa fino
		// all'osso può ricrescere in ricodifica. Mandare byte in più E
		// pixel in meno sarebbe il peggio di entrambi.
		return src, nil
	}
	return buf.Bytes(), nil
}

// fitLongSide dà le dimensioni ridotte che stanno in maxLongSide sul lato
// lungo, a proporzioni invariate. Restituisce le dimensioni originali
// quando non c'è niente da fare (già sotto la soglia, o soglia non
// impostata), così che il chiamante possa riconoscere quel caso
// confrontandole.
func fitLongSide(w, h, maxLongSide int) (int, int) {
	long := w
	if h > long {
		long = h
	}
	if maxLongSide <= 0 || long <= maxLongSide {
		return w, h
	}
	scale := float64(maxLongSide) / float64(long)
	return atLeastOne(math.Round(float64(w) * scale)),
		atLeastOne(math.Round(float64(h) * scale))
}

// atLeastOne evita il lato zero che una soglia molto piccola su
// un'immagine molto allungata produrrebbe: image.Rect resterebbe vuoto e
// l'errore salterebbe fuori da jpeg.Encode, lontano dalla causa.
func atLeastOne(v float64) int {
	if v < 1 {
		return 1
	}
	return int(v)
}

// boxDownscale riduce src a w x h mediando, per ogni pixel di
// destinazione, tutti i pixel sorgente che gli confluiscono.
//
// La media a box non è un compromesso per non tirarsi in casa una libreria
// di resampling: per una RIDUZIONE è il filtro giusto. Ogni pixel di
// destinazione copre un'area della sorgente, e la media di quell'area è
// esattamente ciò che quel pixel deve rappresentare — mentre campionarne
// uno solo butta via il resto e trasforma il testo fine in aliasing, che è
// il modo in cui una riduzione fatta male rovina proprio quello che il
// modello deve leggere.
//
// Un sorgente in scala di grigio produce una destinazione in scala di
// grigio: molte scansioni lo sono, e promuoverle a colore le farebbe
// ricrescere di due componenti su tre proprio mentre si cerca di
// rimpicciolirle. image.Gray.Set converte il color.RGBA64 con i pesi di
// luminanza, che su R=G=B sommano esattamente a 1: il valore non si sposta.
func boxDownscale(src image.Image, w, h int) image.Image {
	sb := src.Bounds()

	var dst draw.Image
	if _, gray := src.(*image.Gray); gray {
		dst = image.NewGray(image.Rect(0, 0, w, h))
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}

	for y := 0; y < h; y++ {
		y0, y1 := srcRange(y, h, sb.Dy())
		for x := 0; x < w; x++ {
			x0, x1 := srcRange(x, w, sb.Dx())

			// I valori di RGBA() sono premoltiplicati per l'alfa, ed è la
			// forma in cui la media è corretta; color.RGBA64 è
			// premoltiplicato allo stesso modo, quindi il giro non ha
			// conversioni. Su un JPEG l'alfa è comunque sempre opaco.
			var r, g, b, a uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, pa := src.At(sb.Min.X+sx, sb.Min.Y+sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					b += uint64(pb)
					a += uint64(pa)
				}
			}
			n := uint64((x1 - x0) * (y1 - y0))
			dst.Set(x, y, color.RGBA64{
				R: uint16(r / n),
				G: uint16(g / n),
				B: uint16(b / n),
				A: uint16(a / n),
			})
		}
	}
	return dst
}

// srcRange dà l'intervallo [lo, hi) di pixel sorgente che confluiscono nel
// pixel di destinazione i, su un asse che va da srcLen a dstLen.
//
// Il calcolo è in interi di proposito: i confini di pixel adiacenti
// coincidono per costruzione — l'hi di i è il lo di i+1 — quindi nessun
// pixel sorgente viene contato due volte e nessuno viene saltato, anche
// quando il rapporto è frazionario (3100 → 1500 è 2,067, non 2). Con i
// float lo stesso confine calcolato due volte può cadere da due parti
// diverse per un errore di arrotondamento.
func srcRange(i, dstLen, srcLen int) (int, int) {
	lo := i * srcLen / dstLen
	hi := (i + 1) * srcLen / dstLen
	if hi <= lo {
		// Un ingrandimento non è il mestiere di questa funzione, ma se
		// arriva qui deve produrre un intervallo non vuoto: n = 0 sotto
		// sarebbe una divisione per zero.
		hi = lo + 1
	}
	if hi > srcLen {
		hi = srcLen
	}
	return lo, hi
}
