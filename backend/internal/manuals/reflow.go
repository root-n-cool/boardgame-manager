package manuals

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// sentenceAbbreviations sono le parole che in un regolamento finiscono con
// un punto senza chiudere la frase. La lista è corta di proposito: copre
// quel che ricorre davvero in un manuale italiano (rimandi di pagina,
// esempi, numerazioni, figure) e non prova a essere un dizionario. Il
// costo di un falso negativo è minimo — una riga spezzata dove non
// serviva, che al modello non toglie niente — quindi allungarla a
// indovinare non vale la manutenzione.
var sentenceAbbreviations = map[string]bool{
	"pag": true, "pagg": true, "pp": true, "p": true,
	"es": true, "ecc": true, "cfr": true,
	"fig": true, "n": true, "num": true, "art": true,
	"cap": true, "par": true, "vol": true, "ed": true,
	"sig": true, "ca": true, "vs": true, "min": true,
}

// ReflowSentences manda a capo il testo a ogni fine di frase, trasformando
// in "\n" gli spazi che la seguono. Serve al percorso testo di un PDF:
// ExtractText restituisce pagine senza a capo per frase (un PDF impaginato
// non ne ha bisogno per essere letto da un umano), e su una pagina che è
// una riga sola il modello che segmenta mette il titolo IN TESTA a quella
// riga invece che su una riga propria. Il controllo di fedeltà di
// ai.Segment scarta l'intera riga di titolo per confrontare il contenuto:
// con una riga da migliaia di caratteri questo conta come "titolo" il
// testo di un'intera pagina e fa dichiarare inaffidabile una lettura
// perfettamente fedele. Con una riga per frase quel che al più si perde è
// una frase.
//
// La trasformazione tocca SOLO spazi e tabulazioni già presenti: nessun
// carattere aggiunto, nessuno tolto. È la proprietà che tiene in piedi le
// due cose che leggono questo testo dopo — il confronto di lunghezza di
// ai.Segment (che normalizza gli spazi bianchi, quindi non vede
// differenza) e le ancore di pagina di pageStartsInSegmented (che cercano
// il testo della pagina dentro il segmentato, e vanno quindi costruite
// sullo stesso testo riformattato, non sull'originale).
//
// Un testo che gli a capo ce li ha già (.txt, .docx, markdown) resta
// identico: dopo una fine di frase seguita da un a capo non c'è niente da
// trasformare.
func ReflowSentences(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		c := s[i]
		b.WriteByte(c)
		if c != '.' && c != '!' && c != '?' && c != ':' {
			continue
		}

		// Gli spazi fra la punteggiatura e quel che segue sono i soli
		// caratteri che questa funzione può trasformare: senza spazi
		// (un a capo già presente, un numero attaccato) non si tocca
		// niente.
		j := i + 1
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j == i+1 || j >= len(s) {
			continue
		}

		// La frase successiva comincia per maiuscola. È il segnale più
		// economico che esista per distinguere una fine di frase da un
		// punto qualunque, e su un regolamento è affidabile: "pag. 7",
		// "n. 3" e "1. Pescare" non lo superano (rispettivamente una
		// cifra, una cifra, e il controllo su abbreviazioni qui sotto).
		r, _ := utf8.DecodeRuneInString(s[j:])
		if !unicode.IsUpper(r) {
			continue
		}
		if c == '.' && !endsSentence(s[:i]) {
			continue
		}

		b.WriteByte('\n')
		i = j - 1 // il ciclo riparte dal primo carattere della frase nuova
	}

	return b.String()
}

// endsSentence dice se il punto che chiude before chiude davvero una
// frase, guardando la parola che lo precede: una iniziale puntata ("Alan
// R. Moon"), il numero di una voce di elenco ("1. Pescare") e le
// abbreviazioni di sentenceAbbreviations non chiudono niente. Vale solo
// per il punto: "?" e "!" non hanno questa ambiguità.
func endsSentence(before string) bool {
	k := len(before)
	for k > 0 {
		r, size := utf8.DecodeLastRuneInString(before[:k])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		k -= size
	}
	token := before[k:]
	if token == "" {
		// Un punto preceduto da punteggiatura — "(vedi la tabella)." —
		// chiude la frase come qualunque altro.
		return true
	}
	if utf8.RuneCountInString(token) == 1 {
		// Una sola lettera è una iniziale puntata, una sola cifra è una
		// voce di elenco: in nessuno dei due casi comincia una frase
		// nuova.
		return false
	}
	if isAllDigits(token) {
		return false
	}
	return !sentenceAbbreviations[strings.ToLower(token)]
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
