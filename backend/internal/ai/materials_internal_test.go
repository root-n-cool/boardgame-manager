package ai

import "testing"

func TestParseMaterialsReadsCleanLines(t *testing.T) {
	got := parseMaterials("tessere\t72\nmeeple\t40\n")
	if len(got) != 2 {
		t.Fatalf("volevo 2 voci, ho %+v", got)
	}
	if got[0].Name != "tessere" || got[0].Quantity != 72 {
		t.Errorf("prima voce inattesa: %+v", got[0])
	}
	if got[1].Name != "meeple" || got[1].Quantity != 40 {
		t.Errorf("seconda voce inattesa: %+v", got[1])
	}
}

func TestParseMaterialsSurvivesSloppyFormatting(t *testing.T) {
	raw := "Ecco il contenuto:\n" +
		"1. tessere paesaggio: 72\n" +
		"- meeple — 40\n" +
		"* dadi 5\n" +
		"plancia punteggio  1\n"
	got := parseMaterials(raw)
	want := map[string]int{
		"tessere paesaggio": 72,
		"meeple":            40,
		"dadi":              5,
		"plancia punteggio": 1,
	}
	if len(got) != len(want) {
		t.Fatalf("volevo %d voci, ho %+v", len(want), got)
	}
	for _, m := range got {
		if want[m.Name] != m.Quantity {
			t.Errorf("voce inattesa: %+v", m)
		}
	}
}

func TestParseMaterialsDropsWhatItCannotUse(t *testing.T) {
	raw := "Contenuto della scatola\n" + // nessun numero: non è una voce
		"regolamento\n" + // idem
		"carte 40\n" +
		"tessere 0\n" + // sotto il minimo
		"segnalini 99999\n" + // sopra il massimo
		"Carte 12\n" // duplicato di "carte", vince il primo
	got := parseMaterials(raw)
	if len(got) != 1 || got[0].Name != "carte" || got[0].Quantity != 40 {
		t.Fatalf("volevo solo carte 40, ho %+v", got)
	}
}

func TestParseMaterialsStopsAtTheCap(t *testing.T) {
	raw := ""
	for i := 0; i < 200; i++ {
		raw += "pezzo" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + " 2\n"
	}
	if got := parseMaterials(raw); len(got) > 60 {
		t.Fatalf("il tetto non è stato applicato: %d voci", len(got))
	}
}
