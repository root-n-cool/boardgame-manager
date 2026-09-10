package manuals

import (
	"image"
	"image/color"
	"testing"
)

// TestFitLongSide copre le proporzioni, che sono la parte della riduzione
// che si può sbagliare in silenzio: un'immagine schiacciata resta
// perfettamente leggibile a occhio in un log di dimensioni, ma il modello
// la vede deformata e trascrive peggio.
func TestFitLongSide(t *testing.T) {
	cases := []struct {
		name         string
		w, h, max    int
		wantW, wantH int
	}{
		{
			// La pagina reale del club: 3100 è il lato lungo, 2110 scende
			// in proporzione a 1021 (2110 × 1500 / 3100 = 1020,97).
			name: "pagina verticale scansionata", w: 2110, h: 3100, max: 1500,
			wantW: 1021, wantH: 1500,
		},
		{
			// Lo stesso caso ruotato: il lato lungo è la larghezza, e la
			// soglia deve applicarsi a quello, non sempre all'altezza.
			name: "pagina orizzontale", w: 3100, h: 2110, max: 1500,
			wantW: 1500, wantH: 1021,
		},
		{
			// I fixture dei test httpapi sono di questo ordine: devono
			// passare intatti, altrimenti tutta quella suite ricodifica
			// immagini per niente.
			name: "già sotto la soglia", w: 24, h: 32, max: 1500,
			wantW: 24, wantH: 32,
		},
		{
			name: "esattamente alla soglia", w: 1000, h: 1500, max: 1500,
			wantW: 1000, wantH: 1500,
		},
		{
			// Una soglia assurda non deve produrre un lato zero: un
			// image.Rect vuoto passerebbe a jpeg.Encode e fallirebbe lì,
			// molto lontano dalla causa.
			name: "soglia minuscola", w: 100, h: 10, max: 1,
			wantW: 1, wantH: 1,
		},
		{
			name: "soglia non impostata", w: 2110, h: 3100, max: 0,
			wantW: 2110, wantH: 3100,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := fitLongSide(tc.w, tc.h, tc.max)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Fatalf("fitLongSide(%d, %d, %d) = %dx%d, atteso %dx%d",
					tc.w, tc.h, tc.max, gotW, gotH, tc.wantW, tc.wantH)
			}
		})
	}
}

// TestBoxDownscale_MediaOgniPixelCheContribuisce è il test che conta: una
// scacchiera a pixel alternati ridotta a metà deve uscire tutta grigio
// medio. Un'implementazione che campiona invece di mediare — il modo
// sbagliato ovvio, e quello che si ottiene "per caso" scrivendo il ciclo
// senza sommare — la farebbe uscire tutta nera o tutta bianca, mai grigia.
//
// L'immagine è costruita a mano e non passa da JPEG di proposito: su una
// scacchiera a pixel singoli la quantizzazione DCT smorza da sola il
// motivo fino a un grigio quasi uniforme, e un test che decodifica un JPEG
// passerebbe anche con il campionamento.
func TestBoxDownscale_MediaOgniPixelCheContribuisce(t *testing.T) {
	const size = 4
	src := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			shade := uint8(0)
			if (x+y)%2 == 0 {
				shade = 255
			}
			src.Set(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}

	dst := boxDownscale(src, size/2, size/2)

	if got := dst.Bounds(); got != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds = %v, atteso 0,0-2,2", got)
	}
	// Ogni blocco 2x2 della sorgente contiene due pixel neri e due
	// bianchi: la media di 0 e 65535 su quattro pixel è 32767, che letta
	// a 8 bit è 127.
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			r, g, b, _ := dst.At(x, y).RGBA()
			if r>>8 != 127 || g>>8 != 127 || b>>8 != 127 {
				t.Fatalf("pixel (%d,%d) = %d,%d,%d a 8 bit, atteso 127 su ogni canale "+
					"(sta campionando invece di mediare?)", x, y, r>>8, g>>8, b>>8)
			}
		}
	}
}

// TestBoxDownscale_RapportoFrazionario verifica il caso che ha deciso la
// forma di srcRange: 3100 → 1500 non è un rapporto intero, e con un
// rapporto frazionario è facile scrivere un ciclo che salta un pixel
// sorgente oppure lo conta due volte. Tre pixel in due: il primo pixel di
// destinazione prende solo il sorgente 0, il secondo la media di 1 e 2.
func TestBoxDownscale_RapportoFrazionario(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 1))
	src.Set(0, 0, color.RGBA{A: 255})                         // nero
	src.Set(1, 0, color.RGBA{A: 255})                         // nero
	src.Set(2, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255}) // bianco

	dst := boxDownscale(src, 2, 1)

	first, _, _, _ := dst.At(0, 0).RGBA()
	if first>>8 != 0 {
		t.Fatalf("primo pixel = %d a 8 bit, atteso 0 (solo il sorgente 0)", first>>8)
	}
	second, _, _, _ := dst.At(1, 0).RGBA()
	if second>>8 != 127 {
		t.Fatalf("secondo pixel = %d a 8 bit, atteso 127 (media di 0 e 255)", second>>8)
	}
}

// TestBoxDownscale_UnaSorgenteGrigiaRestaGrigia protegge metà del guadagno
// su una scansione in bianco e nero: promuovere un JPEG a 1 componente in
// uno a 3 lo fa ricrescere proprio mentre si sta cercando di rimpicciolirlo.
// Verifica anche che la conversione non sposti il valore: sulle scale di
// grigio i pesi di luminanza sommano esattamente a 1, quindi 0x40 deve
// tornare 0x40 e non 0x3f.
func TestBoxDownscale_UnaSorgenteGrigiaRestaGrigia(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.SetGray(x, y, color.Gray{Y: 0x40})
		}
	}

	dst := boxDownscale(src, 2, 2)

	gray, ok := dst.(*image.Gray)
	if !ok {
		t.Fatalf("tipo di destinazione %T, atteso *image.Gray", dst)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if got := gray.GrayAt(x, y).Y; got != 0x40 {
				t.Fatalf("pixel (%d,%d) = %#x, atteso 0x40", x, y, got)
			}
		}
	}
}
