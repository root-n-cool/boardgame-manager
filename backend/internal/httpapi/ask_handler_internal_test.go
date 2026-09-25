package httpapi

import (
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

// Questi test lavorano direttamente su linkifyCitations e formatCorpusIndex
// (non sull'handler HTTP intero): il comportamento che verificano dipende
// solo dalla mappa citations/dai SourceHeadings passati, non dal database o
// da un fakeAsker che simuli una chiamata al tool — la copertura
// end-to-end di ask_handler_test.go resta per la proprietà che DAVVERO
// richiede l'intero giro (la mappa costruita dalle hit di ricerca vere).
func TestLinkifyCitations(t *testing.T) {
	cases := []struct {
		name      string
		answer    string
		citations map[string]*citationTarget
		want      string
	}{
		{
			name:   "documento con pagina diventa un link con #page=N",
			answer: "Vedi Regolamento base, pagina 7.",
			citations: map[string]*citationTarget{
				"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
			},
			want: "Vedi [Regolamento base, pagina 7](/api/uploads/manuale.pdf#page=7).",
		},
		{
			// reference_detail nella forma "sezione «...»": non è una
			// pagina, quindi il link non porta nessun #page=. Il testo
			// del link è la sola reference: il dettaglio resta fuori,
			// come testo semplice.
			name:   "reference_detail non numerico non genera un #page=",
			answer: `Vedi Regolamento base, sezione «Preparazione».`,
			citations: map[string]*citationTarget{
				"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
			},
			want: `Vedi [Regolamento base](/api/uploads/manuale.pdf), sezione «Preparazione».`,
		},
		{
			name:   "reference_detail assente: link senza frammento",
			answer: "Lo dice Regolamento base.",
			citations: map[string]*citationTarget{
				"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
			},
			want: "Lo dice [Regolamento base](/api/uploads/manuale.pdf).",
		},
		{
			name:   "faq: il commento citato porta al suo link",
			answer: "Sul forum: BGG: Refreshing the birdfeeder, commento del 07/01/2019.",
			citations: map[string]*citationTarget{
				"BGG: Refreshing the birdfeeder": &citationTarget{
					referenceType: "faq",
					url:           "https://boardgamegeek.com/thread/100",
					commentURLs: map[string]string{
						"commento del 06/01/2019": "https://boardgamegeek.com/thread/100/article/1#1",
						"commento del 07/01/2019": "https://boardgamegeek.com/thread/100/article/2#2",
					},
				},
			},
			want: "Sul forum: [BGG: Refreshing the birdfeeder, commento del 07/01/2019](https://boardgamegeek.com/thread/100/article/2#2).",
		},
		{
			name:   "faq: data che non corrisponde a nessun commento, il link va al thread",
			answer: "Vedi BGG: Refreshing the birdfeeder, commento del 01/01/2020.",
			citations: map[string]*citationTarget{
				"BGG: Refreshing the birdfeeder": &citationTarget{
					referenceType: "faq",
					url:           "https://boardgamegeek.com/thread/100",
					commentURLs:   map[string]string{"commento del 06/01/2019": "https://boardgamegeek.com/thread/100/article/1#1"},
				},
			},
			want: "Vedi [BGG: Refreshing the birdfeeder, commento del 01/01/2020](https://boardgamegeek.com/thread/100).",
		},
		{
			name:   "faq citata senza dettaglio: link al thread",
			answer: "Ne parlano in BGG: Refreshing the birdfeeder.",
			citations: map[string]*citationTarget{
				"BGG: Refreshing the birdfeeder": &citationTarget{referenceType: "faq", url: "https://boardgamegeek.com/thread/100"},
			},
			want: "Ne parlano in [BGG: Refreshing the birdfeeder](https://boardgamegeek.com/thread/100).",
		},
		{
			// Una reference che è prefisso letterale di un'altra: l'ordine
			// (le più lunghe per prime) evita che sostituire la corta
			// tronchi la lunga a metà nome.
			name:   "una reference prefisso di un'altra non corrompe quella lunga",
			answer: "Vedi Regolamento base, pagina 2, oppure Regolamento, pagina 5.",
			citations: map[string]*citationTarget{
				"Regolamento":      &citationTarget{referenceType: "document", mediaPath: "corto.pdf"},
				"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "lungo.pdf"},
			},
			want: "Vedi [Regolamento base, pagina 2](/api/uploads/lungo.pdf#page=2), " +
				"oppure [Regolamento, pagina 5](/api/uploads/corto.pdf#page=5).",
		},
		{
			// Il modello cita qualcosa che non è mai stato tra le hit
			// restituite dalla ricerca (inventato, o mai passato dal tool):
			// nessun link, la risposta resta testo semplice.
			name:   "una reference inventata non produce nessun link",
			answer: "Solo il Manuale Segreto lo dice, pagina 9.",
			citations: map[string]*citationTarget{
				"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
			},
			want: "Solo il Manuale Segreto lo dice, pagina 9.",
		},
		{
			// Il modello cita solo UNA delle due fonti conosciute: solo
			// quella diventa un link, l'altra reference (mai menzionata)
			// non compare comunque nella risposta.
			name:   "citarne una sola linka solo quella",
			answer: "Regolamento base, pagina 7, risponde alla domanda.",
			citations: map[string]*citationTarget{
				"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
				"English rulebook": &citationTarget{referenceType: "document", mediaPath: "rules-en.pdf"},
			},
			want: "[Regolamento base, pagina 7](/api/uploads/manuale.pdf#page=7), risponde alla domanda.",
		},
		{
			// Una reference troppo corta (sotto minReferenceLength) non si
			// sostituisce: linkare ogni "A" della risposta la renderebbe
			// illeggibile.
			name:   "una reference troppo corta non si sostituisce",
			answer: "Vedi A, pagina 3.",
			citations: map[string]*citationTarget{
				"A": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
			},
			want: "Vedi A, pagina 3.",
		},
		{
			// Una reference con "]" romperebbe la sintassi del link
			// markdown: si salta la sostituzione piuttosto che produrre
			// markdown corrotto.
			name:   "una reference con ] non si sostituisce",
			answer: "Vedi Regol]amento, pagina 2.",
			citations: map[string]*citationTarget{
				"Regol]amento": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
			},
			want: "Vedi Regol]amento, pagina 2.",
		},
		{
			name:      "nessuna citazione nota: la risposta non cambia",
			answer:    "Non c'è nessuna fonte per questo gioco.",
			citations: map[string]*citationTarget{},
			want:      "Non c'è nessuna fonte per questo gioco.",
		},
		{
			// Guardia anti-doppio-link: un link già presente non si
			// riscrive, anche se la risposta cita anche altre reference
			// note.
			name:   "un link già presente non si riscrive",
			answer: "Vedi [Regolamento base, pagina 7](/api/uploads/manuale.pdf#page=7).",
			citations: map[string]*citationTarget{
				"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "manuale.pdf"},
			},
			want: "Vedi [Regolamento base, pagina 7](/api/uploads/manuale.pdf#page=7).",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := linkifyCitations(tc.answer, tc.citations)
			if got != tc.want {
				t.Fatalf("linkifyCitations:\n got:  %q\n want: %q", got, tc.want)
			}
		})
	}
}

func TestFormatCorpusIndex(t *testing.T) {
	cases := []struct {
		name    string
		sources []manuals.SourceHeadings
		want    string
	}{
		{
			name:    "nessuna fonte: stringa vuota",
			sources: nil,
			want:    "",
		},
		{
			name: "una fonte, titoli in ordine di seq",
			sources: []manuals.SourceHeadings{
				{Reference: "Regolamento base", Headings: []string{"Preparazione", "Turno del giocatore", "Fase di Upkeep"}},
			},
			want: "Fonti: Regolamento base — Preparazione · Turno del giocatore · Fase di Upkeep",
		},
		{
			// Più fonti: le voci si accodano separate da "; ", e una fonte
			// senza titoli (nessun heading rilevato) non produce una voce
			// vuota in mezzo.
			name: "più fonti, una senza titoli",
			sources: []manuals.SourceHeadings{
				{Reference: "Regolamento base", Headings: []string{"Preparazione"}},
				{Reference: "FAQ senza titoli", Headings: nil},
				{Reference: "English rulebook", Headings: []string{"Setup", "Upkeep phase"}},
			},
			want: "Fonti: Regolamento base — Preparazione; English rulebook — Setup · Upkeep phase",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatCorpusIndex(tc.sources)
			if got != tc.want {
				t.Fatalf("formatCorpusIndex:\n got:  %q\n want: %q", got, tc.want)
			}
		})
	}
}

// Verifica indipendente, senza il break/fix manuale sul test end-to-end:
// se l'ordinamento long-first di linkifyCitations si perdesse, una
// reference prefisso dell'altra corromperebbe il link della più lunga.
// Non è l'implementazione a lookup-per-titolo (quella la copre il test
// end-to-end in ask_handler_test.go), ma la stessa proprietà — prefisso
// letterale — testata al livello più semplice possibile.
func TestLinkifyCitations_PrefixOrderMattersEvenWithASingleWord(t *testing.T) {
	answer := "Vedi Regolamento base, pagina 2."
	citations := map[string]*citationTarget{
		"Regolamento":      &citationTarget{referenceType: "document", mediaPath: "corto.pdf"},
		"Regolamento base": &citationTarget{referenceType: "document", mediaPath: "lungo.pdf"},
	}
	got := linkifyCitations(answer, citations)
	if !strings.Contains(got, "[Regolamento base, pagina 2](/api/uploads/lungo.pdf#page=2)") {
		t.Fatalf("la reference lunga è stata corrotta dalla sostituzione della corta: %q", got)
	}
}
