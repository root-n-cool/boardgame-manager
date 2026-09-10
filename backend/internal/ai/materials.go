package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"boardgames-manager/internal/games"
)

// materialsTimeout sta fra suggestTimeout e segmentTimeout: l'input è
// qualche passaggio di manuale e l'output può essere trenta righe, quindi
// più di tre domande e molto meno di un manuale intero.
const materialsTimeout = 45 * time.Second

// ErrMaterialsRejected dice che nella risposta non c'era nemmeno una riga
// utilizzabile. È distinto da un guasto di rete perché il pannello admin
// deve poter dire "non l'ho trovato nel manuale" invece di "riprova".
var ErrMaterialsRejected = errors.New("ai materials rejected: no usable line in the response")

// SuggestedMaterial è una voce proposta dal modello. Non è ancora una
// games.Material: non ha id, non ha posizione, e soprattutto non è salvata —
// l'admin la conferma prima.
type SuggestedMaterial struct {
	Name     string
	Quantity int
}

// MaterialLister è l'astrazione che serve all'handler; HTTPClient la
// implementa, i test iniettano un finto. Stesso schema di QuestionSuggester.
type MaterialLister interface {
	ListMaterials(ctx context.Context, gameName string, passages []string) ([]SuggestedMaterial, error)
}

// materialsSystemPrompt chiede righe e vieta tutto il resto. Non è una
// garanzia — la mitigazione vera è parseMaterials — ma è ciò che rende lo
// scarto raro invece che normale.
var materialsSystemPrompt = "Ricevi il nome di un gioco da tavolo e alcuni passaggi del suo regolamento. " +
	"Estrai il CONTENUTO DELLA SCATOLA: l'elenco dei pezzi fisici con la loro quantità.\n" +
	"Regole assolute:\n" +
	"1. Una voce per riga, nella forma NOME<TAB>QUANTITÀ. Nessuna numerazione, nessun elenco puntato, nessun preambolo, nessun commento.\n" +
	"2. La quantità è un numero intero. Se il regolamento non la dice, salta la voce.\n" +
	"3. Nomi in italiano, al plurale, come li direbbe un giocatore al tavolo: \"tessere\", \"meeple\", \"carte\".\n" +
	fmt.Sprintf("4. Al massimo %d voci, ogni nome sotto i %d caratteri.\n", games.MaxMaterialsPerGame, games.MaxMaterialNameChars) +
	"5. Una riga per tipo di pezzo: \"meeple\t40\", non una riga per colore, a meno che il regolamento non li elenchi già separati.\n" +
	"6. IGNORA tutto ciò che non è un pezzo dentro la scatola: regole, autori, crediti, ringraziamenti, siti web."

// ListMaterials chiede al modello il contenuto della scatola partendo da
// passaggi del manuale già indicizzato. Non salva niente: la proposta la
// conferma l'admin.
func (c *HTTPClient) ListMaterials(ctx context.Context, gameName string, passages []string) ([]SuggestedMaterial, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	if len(passages) == 0 {
		// Nessun passaggio da leggere: il chiamante deve dirlo all'admin
		// ("indicizza prima un manuale"), non ricevere una lista inventata.
		return nil, ErrMaterialsRejected
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Gioco: %s\n\nPassaggi dal regolamento:\n", gameName)
	for _, p := range passages {
		fmt.Fprintf(&b, "---\n%s\n", p)
	}

	payload, err := json.Marshal(chatRequest{
		Model: c.Model,
		// temperature 0 come SuggestQuestions: il contenuto di una scatola
		// è un fatto, non deve cambiare a ogni generazione.
		Temperature:     0,
		ReasoningEffort: reasoningEffortNone,
		Messages: []chatMessage{
			{Role: "system", Content: materialsSystemPrompt},
			{Role: "user", Content: b.String()},
		},
	})
	if err != nil {
		return nil, err
	}

	raw, err := c.postChat(ctx, payload, materialsTimeout)
	if err != nil {
		return nil, err
	}
	out := parseMaterials(raw)
	if len(out) == 0 {
		return nil, ErrMaterialsRejected
	}
	return out, nil
}

// trailingNumber trova l'ultimo intero della riga: è lì che sta la quantità
// sia in "tessere 72" sia in "tessere paesaggio: 72", e cercare il primo
// numero inciamperebbe su "1." di una numerazione.
var trailingNumber = regexp.MustCompile(`(\d+)\s*$`)

// leadingBullet è la numerazione o il puntino che il modello aggiunge anche
// quando gli si dice di non farlo.
var leadingBullet = regexp.MustCompile(`^\s*(?:[-*•]|\d+[.)])\s*`)

// parseMaterials è la mitigazione vera contro una risposta fuori formato:
// tiene solo le righe che portano un nome e un intero nei limiti dello
// store, e scarta in silenzio tutto il resto. Se non resta niente, chi
// chiama lo racconta con ErrMaterialsRejected.
func parseMaterials(raw string) []SuggestedMaterial {
	seen := map[string]bool{}
	out := []SuggestedMaterial{}
	for _, line := range strings.Split(raw, "\n") {
		line = leadingBullet.ReplaceAllString(strings.TrimSpace(line), "")
		if line == "" {
			continue
		}
		m := trailingNumber.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		quantity, err := strconv.Atoi(m[1])
		if err != nil || quantity < 1 || quantity > games.MaxMaterialQuantity {
			continue
		}
		name := strings.TrimSpace(line[:len(line)-len(m[0])])
		name = strings.TrimRight(name, " \t:—-–")
		name = strings.TrimSpace(name)
		if name == "" || len([]rune(name)) > games.MaxMaterialNameChars {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, SuggestedMaterial{Name: name, Quantity: quantity})
		if len(out) == games.MaxMaterialsPerGame {
			break
		}
	}
	return out
}
