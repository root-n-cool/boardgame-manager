package manuals

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// documentXMLPart è il nome, fisso nello standard OOXML, della parte zip
// che contiene il corpo del documento.
const documentXMLPart = "word/document.xml"

// maxDocumentXMLBytes limita quanti byte leggiamo da word/document.xml UNA
// VOLTA DECOMPRESSO. Il limite di 20 MB sull'upload (storage.ManualCategory)
// vale sul file zip compresso: un document.xml è XML ripetitivo (gli stessi
// tag <w:p><w:r><w:t> migliaia di volte), che comprime moltissimo — un
// archivio piccolo può nascondere un document.xml enorme (uno zip bomb).
// Il limite si applica DURANTE la lettura (io.LimitReader), non confrontando
// la dimensione dichiarata nell'header dello zip (UncompressedSize64): quel
// numero è metadato scritto dall'archivio stesso e non garantisce nulla
// finché non si arriva alla fine dello stream — un archivio costruito ad
// arte potrebbe dichiarare poco e produrre in lettura molto di più. L'unico
// limite affidabile è quello imposto mentre si legge.
//
// Il valore è generoso rispetto a qualunque regolamento reale: anche un
// manuale molto lungo, con tutta la formattazione OOXML intorno al testo,
// sta comodamente sotto qualche MB di XML.
const maxDocumentXMLBytes = 5 * 1024 * 1024

// ErrDocumentXMLTooLarge è l'errore restituito quando word/document.xml
// supera maxDocumentXMLBytes una volta decompresso. È distinto da "il
// documento non contiene testo": sono due cause diverse, e il Task 5 deve
// poter dire all'admin cose diverse per ciascuna. Esportato (non un
// semplice fmt.Errorf) perché il chiamante deve poterlo riconoscere con
// errors.Is: un confronto sul testo del messaggio si romperebbe al primo
// refactoring della frase, e — come il round di review precedente ha
// mostrato empiricamente — un file troncato da maxDocumentXMLBytes può
// produrre anche un errore di parsing XML dal tutt'altro aspetto: solo
// l'identità del sentinel distingue in modo affidabile "era troppo grande"
// da "era troncato in un punto che ha rotto l'XML".
var ErrDocumentXMLTooLarge = errors.New("docx: word/document.xml supera il limite consentito una volta decompresso")

// DocxToMarkdown legge un .docx (uno zip OOXML) e ne restituisce il corpo
// come markdown: un sottoinsieme deliberatamente limitato — paragrafi,
// titoli, elenchi puntati — pensato per alimentare ParseSections
// (markdown.go), non per riprodurre l'impaginazione del documento.
//
// Decisioni prese, e perché:
//
//   - Tabelle (<w:tbl>) e caselle di testo (<w:txbxContent>) sono escluse
//     di proposito, non un'omissione: la spec di design le mette
//     esplicitamente fuori dal sottoinsieme utile. Un <w:tbl> contiene a
//     sua volta <w:p> (le celle sono paragrafi), quindi senza
//     un'esclusione esplicita finirebbero incluse come testo piatto senza
//     struttura a griglia — peggio che ometterle, perché sembrerebbe
//     prosa scorretta invece di dati tabellari mancanti. Le teniamo fuori
//     contando la profondità di annidamento e saltando ogni paragrafo
//     mentre la profondità è > 0.
//   - Le note a piè di pagina non richiedono un'esclusione esplicita: il
//     loro testo vive in word/footnotes.xml, una parte diversa dello zip
//     che non apriamo. Nel corpo resta solo <w:footnoteReference>, che non
//     porta testo proprio e quindi non produce output da solo.
//   - Lo stile del titolo si riconosce da <w:pStyle w:val="...">,
//     normalizzato (minuscolo, spazi rimossi) e confrontato con i prefissi
//     "heading" e "titolo" (Word scrive "Heading1", altri produttori
//     "heading 1", i documenti italiani "Titolo1"). Se lo stile non è
//     riconosciuto ma il paragrafo porta <w:outlineLvl>, quel livello
//     (0-based) diventa comunque un titolo: è la via con cui uno stile
//     personalizzato finisce comunque nel sommario di Word.
//   - Un paragrafo con <w:numPr> (elenco puntato o numerato: la
//     distinzione vive in word/numbering.xml, una parte che non apriamo)
//     diventa una riga "- ...": non distinguiamo puntato da numerato,
//     scelta deliberata per restare dentro lo stdlib e i tre elementi che
//     contano (w:p, w:r, w:t) senza aprire una quarta parte dello zip solo
//     per un dettaglio che alla ricerca FTS non serve.
//   - Il testo di un paragrafo normale viene protetto SOLO quando il suo
//     primo carattere è "#": senza, un paragrafo Normal che comincia per
//     coincidenza con un cancelletto verrebbe riletto da ParseSections
//     come un titolo, spezzando la sezione a metà frase. Nessun altro
//     carattere ("-", "*", "+", ">", un elenco numerato "1.") viene
//     protetto: questo markdown non viene mai renderizzato, il solo
//     consumatore è ParseSections, e ParseSections guarda esclusivamente i
//     titoli ATX — non liste né blockquote. Un regolamento scritto senza
//     usare le liste di Word comincia legittimamente paragrafi con "1." o
//     "-", e un backslash spurio lì dentro finirebbe nell'indice FTS5 e
//     nelle citazioni mostrate all'utente senza che nessuno lo tolga mai.
//     Il titolo emesso da questa funzione stessa ("# " + testo) non viene
//     protetto: è un titolo apposta.
//   - Un documento senza testo utile (zip senza word/document.xml, un
//     corpo che non produce nessun paragrafo non vuoto, o un contenuto
//     interamente escluso come una tabella senza altro testo) è un
//     errore, non una stringa vuota: una stringa vuota senza errore
//     sparirebbe in silenzio, mentre l'errore arriva fino all'admin
//     (Task 5).
func DocxToMarkdown(raw []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("docx: non è un archivio zip valido: %w", err)
	}

	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == documentXMLPart {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("docx: manca %s nell'archivio: non è un .docx", documentXMLPart)
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", fmt.Errorf("docx: apertura di %s: %w", documentXMLPart, err)
	}
	defer rc.Close()

	// Il +1 permette di distinguere "esattamente al limite" da "oltre il
	// limite": se dopo la lettura data è più lungo di maxDocumentXMLBytes,
	// lo stream conteneva più byte della soglia.
	data, err := io.ReadAll(io.LimitReader(rc, maxDocumentXMLBytes+1))
	if err != nil {
		return "", fmt.Errorf("docx: lettura di %s: %w", documentXMLPart, err)
	}
	if len(data) > maxDocumentXMLBytes {
		return "", ErrDocumentXMLTooLarge
	}

	md, err := decodeDocumentXML(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if md == "" {
		return "", fmt.Errorf("docx: il documento non contiene testo")
	}
	return md, nil
}

// decodeDocumentXML scorre word/document.xml con un xml.Decoder in
// streaming: lo schema WordprocessingML è vasto e ci interessano solo i
// paragrafi, i loro run di testo e i pochi attributi che decidono se un
// paragrafo è un titolo o un elenco — un xml.Unmarshal su una struct
// dell'intero documento dovrebbe comunque modellare tutto il resto per non
// perdere silenziosamente ciò che non è stato dichiarato.
func decodeDocumentXML(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)

	var out strings.Builder
	var para strings.Builder

	inParagraph := false
	paraStyle := ""
	paraOutline := -1
	paraIsListItem := false
	tableDepth := 0
	textBoxDepth := 0

	emit := func() {
		text := strings.TrimSpace(para.String())
		if text == "" {
			return
		}
		if tableDepth > 0 || textBoxDepth > 0 {
			// Dentro una tabella o una casella di testo: fuori dal
			// sottoinsieme che questa funzione traduce, per scelta di
			// design, non per svista.
			return
		}
		if level := headingLevel(paraStyle, paraOutline); level > 0 {
			// Un titolo non attraversa mai più righe nell'output: un "\n"
			// letterale dentro un <w:t> (non previsto dallo schema, che
			// userebbe <w:br/>, ma non impossibile in un file scritto a
			// mano) spezzerebbe la riga ATX e ParseSections leggerebbe
			// solo la prima metà come titolo.
			heading := strings.ReplaceAll(text, "\n", " ")
			out.WriteString(strings.Repeat("#", level))
			out.WriteString(" ")
			out.WriteString(heading)
			out.WriteString("\n\n")
			return
		}
		if paraIsListItem {
			out.WriteString("- ")
			out.WriteString(text)
			out.WriteString("\n\n")
			return
		}
		out.WriteString(escapeLeadingMarkdown(text))
		out.WriteString("\n\n")
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("docx: analisi di %s: %w", documentXMLPart, err)
		}

		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "tbl":
				tableDepth++
			case "txbxContent":
				textBoxDepth++
			case "p":
				inParagraph = true
				para.Reset()
				paraStyle = ""
				paraOutline = -1
				paraIsListItem = false
			case "pStyle":
				if inParagraph {
					paraStyle = attrVal(el, "val")
				}
			case "outlineLvl":
				if inParagraph {
					if n, err := strconv.Atoi(attrVal(el, "val")); err == nil {
						paraOutline = n
					}
				}
			case "numPr":
				if inParagraph {
					paraIsListItem = true
				}
			case "tab":
				if inParagraph {
					// Il tab è un elemento a sé, non testo dentro un
					// <w:t>: senza questo, "Nome:<tab/>Mario" perderebbe
					// lo spazio fra i due run e diventerebbe "Nome:Mario".
					para.WriteString("\t")
				}
			case "br":
				if inParagraph {
					// Uno spazio, non un "\n": un titolo o un paragrafo
					// restano su una riga sola nell'output (vedi il
					// commento su ParseSections in emit()).
					para.WriteString(" ")
				}
			case "t":
				if inParagraph {
					var text string
					if err := dec.DecodeElement(&text, &el); err != nil {
						return "", fmt.Errorf("docx: lettura di <w:t>: %w", err)
					}
					// encoding/xml non normalizza mai gli spazi di un
					// nodo di testo, con o senza xml:space="preserve":
					// non serve trimmare qui, altrimenti "Ogni giocatore "
					// + "paga " perderebbe lo spazio fra le parole al
					// bordo fra due run.
					para.WriteString(text)
				}
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "tbl":
				tableDepth--
			case "txbxContent":
				textBoxDepth--
			case "p":
				if inParagraph {
					emit()
					inParagraph = false
				}
			}
		}
	}

	return strings.TrimSpace(out.String()), nil
}

// attrVal restituisce il valore dell'attributo local di el, ignorando il
// namespace: in word/document.xml questi attributi compaiono solo con il
// prefisso "w:", e cercarne solo il nome locale evita di dover risolvere
// esplicitamente quel namespace.
func attrVal(el xml.StartElement, local string) string {
	for _, a := range el.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// headingLevel decide se uno stile di paragrafo, o in sua assenza un
// outlineLvl, ne fanno un titolo, e a quale livello (1-6). Restituisce 0
// quando nessuno dei due lo rende un titolo.
func headingLevel(style string, outline int) int {
	norm := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(style), " ", ""))
	for _, prefix := range [...]string{"heading", "titolo"} {
		if strings.HasPrefix(norm, prefix) {
			if n, err := strconv.Atoi(strings.TrimPrefix(norm, prefix)); err == nil && n >= 1 && n <= 6 {
				return n
			}
		}
	}
	// outlineLvl è 0-based (0 = livello 1 del sommario): copre lo stile
	// personalizzato che non si chiama "Heading"/"Titolo" ma che Word
	// tratta comunque come titolo nel sommario.
	if outline >= 0 && outline <= 5 {
		return outline + 1
	}
	return 0
}

// escapeLeadingMarkdown antepone un backslash quando il primo carattere di
// un paragrafo Normal è "#": senza, un paragrafo che comincia per
// coincidenza con un cancelletto verrebbe riletto da ParseSections (che
// guarda SOLO i titoli ATX a inizio riga — markdown.go) come un titolo,
// spezzando la sezione a metà frase.
//
// Nessun altro carattere viene protetto — non "-", "*", "+", ">", non un
// elenco numerato ("1."/"1)") — perché questo markdown non viene mai
// renderizzato: l'unico consumatore è ParseSections, e ParseSections non
// guarda liste né blockquote. Proteggerli sarebbe rumore spurio dentro il
// testo indicizzato da FTS5 e mostrato nelle citazioni (Task 5): un
// regolamento scritto senza usare le liste di Word comincia legittimamente
// paragrafi con "1." o "-", e un backslash spurio lì non lo toglierebbe mai
// nessuno.
func escapeLeadingMarkdown(s string) string {
	if s != "" && s[0] == '#' {
		return "\\" + s
	}
	return s
}
