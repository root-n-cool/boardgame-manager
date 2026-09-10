package manuals_test

import (
	"bytes"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"boardgames-manager/internal/manuals"
)

// TestDownscale_RimpiccioliscePaginaScansionata esercita la misura reale:
// le quattro pagine del manuale del club sono ~2110x3100 e 0,43-0,52 MB,
// che in base64 diventano 0,57-0,69 MB di payload per pagina — abbastanza
// per stare al limite di quello che il provider serve in tempo.
func TestDownscale_RimpiccioliscePaginaScansionata(t *testing.T) {
	src := manuals.NewTestJPEG(2110, 3100)

	out, err := manuals.Downscale(src, 1500)
	if err != nil {
		t.Fatalf("Downscale: %v", err)
	}

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("il risultato non è un JPEG decodificabile: %v", err)
	}
	if cfg.Width != 1021 || cfg.Height != 1500 {
		t.Fatalf("dimensioni %dx%d, attese 1021x1500", cfg.Width, cfg.Height)
	}
	if len(out) >= len(src) {
		t.Fatalf("byte %d, non meno dei %d originali", len(out), len(src))
	}
	// Decodifica completa e non solo l'header: un flusso troncato supera
	// DecodeConfig e fallisce dal provider, dove l'errore non dice niente.
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("il risultato non si decodifica per intero: %v", err)
	}
	t.Logf("2110x3100: %d byte → 1021x1500: %d byte", len(src), len(out))
}

// TestDownscale_LasciaIntattaUnaPaginaGiaPiccola è il ramo che tiene ferma
// tutta la suite httpapi (i cui fixture sono 24x32) e, più in generale,
// evita una perdita generazionale gratuita: ricodificare un JPEG che è già
// sotto la soglia lo peggiora senza fargli risparmiare niente.
func TestDownscale_LasciaIntattaUnaPaginaGiaPiccola(t *testing.T) {
	src := manuals.NewTestJPEG(24, 32)

	out, err := manuals.Downscale(src, 1500)
	if err != nil {
		t.Fatalf("Downscale: %v", err)
	}
	if !bytes.Equal(out, src) {
		t.Fatalf("byte cambiati (%d → %d): una pagina sotto soglia va restituita così com'è",
			len(src), len(out))
	}
}

// TestDownscale_ByteNonJPEGTornanoIntatti fissa il contratto che serve al
// chiamante: qualunque cosa vada storto, i byte restituiti sono
// inviabili al modello. Perdere una pagina dell'indice perché la riduzione
// è inciampata sarebbe un danno peggiore del problema che risolve.
func TestDownscale_ByteNonJPEGTornanoIntatti(t *testing.T) {
	src := []byte("questo non è un JPEG")

	out, err := manuals.Downscale(src, 1500)
	if err == nil {
		t.Fatal("errore nullo: il chiamante non ha modo di loggare il guasto")
	}
	if !bytes.Equal(out, src) {
		t.Fatalf("byte %q, attesi quelli originali", out)
	}
}

// TestDownscale_OnTheRealManual gira solo se in ./data c'è un manuale
// scansionato, esattamente come TestExtractPageImages_OnTheRealManual, e
// per la stessa ragione: la riduzione va misurata su pagine prodotte da
// uno scanner vero, non sulla fixture sintetica, che è un gradiente a
// dente di sega e si comprime in un modo che non somiglia a niente di
// reale. Salta in CI.
//
// Non asserisce una soglia di risparmio: quanto si guadagna dipende dallo
// scanner. Verifica le due cose che devono valere su qualunque scansione —
// il lato lungo entra nella soglia e i byte non crescono — e logga la
// misura, che è il motivo per cui il test esiste.
func TestDownscale_OnTheRealManual(t *testing.T) {
	paths, _ := filepath.Glob("../../../data/uploads/*.pdf")
	if len(paths) == 0 {
		t.Skip("nessun PDF in ./data/uploads: misura saltata")
	}
	const maxLongSide = 1500
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		imgs, err := manuals.ExtractPageImages(raw)
		if err != nil || len(imgs) == 0 {
			t.Logf("%s: nessuna pagina immagine, non è una scansione", filepath.Base(p))
			continue
		}
		for _, img := range imgs {
			out, err := manuals.Downscale(img.JPEG, maxLongSide)
			if err != nil {
				t.Errorf("%s pagina %d: %v", filepath.Base(p), img.Number, err)
				continue
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
			if err != nil {
				t.Errorf("%s pagina %d: il risultato non è decodificabile: %v",
					filepath.Base(p), img.Number, err)
				continue
			}
			long := cfg.Width
			if cfg.Height > long {
				long = cfg.Height
			}
			if long > maxLongSide {
				t.Errorf("%s pagina %d: lato lungo %d, oltre la soglia di %d",
					filepath.Base(p), img.Number, long, maxLongSide)
			}
			if len(out) > len(img.JPEG) {
				t.Errorf("%s pagina %d: %d byte, più dei %d originali",
					filepath.Base(p), img.Number, len(out), len(img.JPEG))
			}
			t.Logf("%s pagina %d: %dx%d %d byte → %dx%d %d byte (%.0f%% in meno, base64 %.2f → %.2f MB)",
				filepath.Base(p), img.Number,
				img.Width, img.Height, len(img.JPEG),
				cfg.Width, cfg.Height, len(out),
				100*(1-float64(len(out))/float64(len(img.JPEG))),
				float64(len(img.JPEG))*4/3/(1<<20), float64(len(out))*4/3/(1<<20))
		}
	}
}
