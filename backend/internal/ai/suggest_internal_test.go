package ai

import (
	"strings"
	"testing"
)

// TestParseSuggestions fissa cosa si accetta dal modello. La validazione è
// meccanica e non un'esortazione nel prompt perché questo testo va sulla
// SCHEDA PUBBLICA: un modello che risponde "Ecco tre domande:" non deve
// poter mettere quella riga a schermo.
func TestParseSuggestions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
		ok   bool
	}{
		{
			name: "tre domande pulite",
			in:   "Come si prepara il gioco?\nCosa posso fare nel mio turno?\nCome finisce la partita?",
			want: []string{"Come si prepara il gioco?", "Cosa posso fare nel mio turno?", "Come finisce la partita?"},
			ok:   true,
		},
		{
			name: "righe vuote in mezzo si ignorano",
			in:   "Come si prepara?\n\nCosa faccio nel turno?\n\n\nCome finisce?\n",
			want: []string{"Come si prepara?", "Cosa faccio nel turno?", "Come finisce?"},
			ok:   true,
		},
		{
			name: "spazi ai bordi si tolgono",
			in:   "  Come si prepara?  \n\tCosa faccio?\t\nCome finisce?",
			want: []string{"Come si prepara?", "Cosa faccio?", "Come finisce?"},
			ok:   true,
		},
		{name: "due domande non bastano", in: "Come si prepara?\nCome finisce?", ok: false},
		{
			name: "quattro domande sono troppe",
			in:   "Uno?\nDue?\nTre?\nQuattro?",
			ok:   false,
		},
		{
			// Il caso che la validazione esiste per fermare.
			name: "un preambolo conta come riga e sfora",
			in:   "Ecco tre domande:\nCome si prepara?\nCosa faccio?\nCome finisce?",
			ok:   false,
		},
		{
			name: "una riga che non è una domanda",
			in:   "Come si prepara?\nQuesto gioco è bello.\nCome finisce?",
			ok:   false,
		},
		{
			// La riga lunga si costruisce con strings.Repeat invece di
			// scriverla a mano: una domanda "abbastanza lunga" contata a
			// occhio può finire sotto il tetto e far passare il test per
			// il motivo sbagliato.
			name: "una domanda troppo lunga per un bottone",
			in:   "Come si prepara?\n" + strings.Repeat("x", MaxSuggestionChars+1) + "?\nCome finisce?",
			ok:   false,
		},
		{
			// L'accento italiano ("è") occupa due byte in UTF-8 ma resta
			// una sola rune. Una domanda accentata vicino al tetto ha quindi
			// più byte di quanti caratteri ne conti chi la legge: se il
			// tetto fosse contato in byte invece che in rune, questa domanda
			// legittima verrebbe rifiutata solo perché scritta in italiano.
			// strings.Repeat("è", 100) fa 101 rune (dentro i 120 di
			// MaxSuggestionChars) ma 201 byte (ben oltre): il caso fallisce
			// se qualcuno cambia len([]rune(q)) in len(q).
			name: "una domanda accentata conta in caratteri non in byte",
			in:   "Come si prepara?\n" + strings.Repeat("è", 100) + "?\nCome finisce?",
			want: []string{"Come si prepara?", strings.Repeat("è", 100) + "?", "Come finisce?"},
			ok:   true,
		},
		{name: "risposta vuota", in: "", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSuggestions(tc.in)
			if tc.ok && err != nil {
				t.Fatalf("atteso valido, rifiutato con %v", err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("atteso rifiuto, accettato %v", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("attese %d domande, ottenute %d: %v", len(tc.want), len(got), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("domanda %d: atteso %q, ottenuto %q", i, tc.want[i], got[i])
				}
			}
		})
	}
}
