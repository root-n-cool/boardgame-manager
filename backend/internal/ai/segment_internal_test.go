package ai

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Test di package interno: splitIntoWindows non è esportata, ma è qui che
// si decide dove cade il taglio di fallback (nessun confine di paragrafo
// entro paragraphLookback). Vive a parte da segment_test.go (che è
// ai_test, sulla superficie pubblica) apposta per poter chiamare
// splitIntoWindows direttamente.
//
// La fixture è un unico, enorme paragrafo (nessun "\n\n" da nessuna
// parte: il confine di paragrafo non c'è, quindi il fallback scatta per
// forza) fatto di caratteri accentati multi-byte, costruito apposta
// perché il taglio a SegmentWindowMaxChars cada in mezzo a una rune: un
// prefisso di un byte ("x") sfalsa la parità, e ogni "à" occupa due byte
// (0xC3 0xA0) a partire da un offset dispari — quindi qualunque taglio a
// un offset PARI (SegmentWindowMaxChars = 60000 lo è) cade esattamente a
// metà di un carattere, non fra due caratteri.
//
// L'asserzione che conta è utf8.ValidString su ogni finestra: senza
// l'arretramento al confine di rune valido, il taglio produce byte UTF-8
// non validi che json.Marshal (a valle, in segmentWindow) sanerebbe in
// silenzio sostituendoli con U+FFFD — corruzione silenziosa, nessun
// errore visibile. L'asserzione sulla ricostruzione byte-per-byte da sola
// NON discrimina questo guasto: unire le finestre con strings.Join
// riottiene comunque il testo originale anche se il taglio cade a metà
// rune, perché tagliare una stringa Go a un offset di byte qualunque non
// perde né duplica byte. Per questo qui ci sono entrambe le asserzioni,
// ma quella che protegge davvero è la prima.
func TestSplitIntoWindows_NeverCutsInsideAMultiByteRune(t *testing.T) {
	const accentedRunes = 40000 // 2 byte ciascuna: ben oltre SegmentWindowMaxChars
	text := "x" + strings.Repeat("à", accentedRunes)

	windows := splitIntoWindows(text, SegmentWindowMaxChars)
	if len(windows) < 2 {
		t.Fatalf("fixture non calibrata: attese almeno 2 finestre, ottenute %d", len(windows))
	}

	for i, w := range windows {
		if !utf8.ValidString(w) {
			t.Fatalf("finestra %d non è UTF-8 valido: il taglio è caduto a metà di una rune multi-byte", i)
		}
	}

	if got := strings.Join(windows, ""); got != text {
		t.Fatalf("le finestre concatenate non ricostruiscono il testo originale byte per byte")
	}
}
