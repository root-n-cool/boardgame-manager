package manuals

import "testing"

// TestSortByPreferredLanguage_PutsPreferredFirstButKeepsRankOrderInsideGroups
// è un test di package interno apposta: verifica il riordino per lingua
// preferita su dati costruiti a mano, senza passare da FTS5/BM25, dove un
// pareggio di rank benevolo potrebbe nascondere che il riordino non sta
// facendo niente. Confronta la SEQUENZA esatta degli id, non l'insieme: un
// controllo sull'insieme passerebbe anche con un ordine casuale (il bug
// reale, quando il riordino viveva in una mappa Go).
func TestSortByPreferredLanguage_PutsPreferredFirstButKeepsRankOrderInsideGroups(t *testing.T) {
	all := []scannedHit{
		{id: 1, language: "en"}, // rank 0: il migliore per BM25, ma lingua non preferita
		{id: 2, language: "it"}, // rank 1
		{id: 3, language: "en"}, // rank 2
		{id: 4, language: "it"}, // rank 3
	}
	sortByPreferredLanguage(all, "it")

	ids := make([]int64, len(all))
	for i, s := range all {
		ids[i] = s.id
	}
	// Atteso: prima il gruppo "it" nell'ordine di rank originale (2, 4),
	// poi il gruppo "en" nello stesso ordine originale (1, 3).
	want := []int64{2, 4, 1, 3}
	if len(ids) != len(want) {
		t.Fatalf("lunghezza inattesa: %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ordine = %v, atteso %v: la preferenza di lingua o la stabilità del riordino sono rotte", ids, want)
		}
	}
}
