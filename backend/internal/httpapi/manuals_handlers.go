package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// minAvgUsableCharsPerPage separa un layer testo che è davvero la prosa
// del manuale da uno che è solo rumore dello scanner (un'intestazione o un
// numero di pagina impressi su una pagina altrimenti scansionata).
//
// È una media per pagina, non un totale: un totale non scala. Una
// scansione di quattro pagine la cui unica "estrazione" è un'intestazione
// ripetuta tipo "Wingspan — Regolamento" (~22 caratteri) somma a ~88
// caratteri, che un tetto sul totale di 20 chiamerebbe "usabile" a
// prescindere da quante pagine ha la scansione — è la scala che tradisce
// la soglia, non la soglia in sé. Mediando invece il segnale per pagina
// resta stabile: un'intestazione o un rumore restano sulle decine di
// caratteri qualunque sia il numero di pagine, mentre una vera pagina di
// manuale (un paragrafo o più di regole) media sulle centinaia. 100 sta
// comodamente sopra il tetto del rumore e comodamente sotto la prosa vera.
const minAvgUsableCharsPerPage = 100

// transcribeConcurrency è quante pagine di un manuale scansionato si
// mandano al modello vision contemporaneamente. Un manuale di trenta
// pagine trascritto in sequenza sono minuti d'attesa dentro una singola
// request HTTP; due alla volta li dimezzano.
//
// Due e non cinque, che era il primo valore, per una misura e non per una
// stima: sul manuale reale del club (4 pagine, tutte ~2110x3100 e ~0,5 MB)
// con quattro richieste in volo il provider ne ha servite DUE e ha lasciato
// le altre due morire nel timeout. Una concorrenza che il provider non
// serve non è throughput: sono pagine perse dall'indice più il tempo del
// timeout buttato. Meglio due che tornano di cinque che stallano — e il
// guadagno non è lineare comunque: da 1 a 2 si dimezza, da 2 a 5 si
// aggiunge solo se il provider risponde davvero.
//
// Il numero giusto è una proprietà del provider configurato, non di questo
// codice: se un domani si vuole spingere, questa costante è il punto da
// rendere configurabile nelle impostazioni.
const transcribeConcurrency = 2

// pageAnchorChars è la lunghezza dell'ancora usata per ritrovare l'inizio
// di una pagina dentro il testo segmentato (vedi pageStartsInSegmented):
// abbastanza lunga da essere quasi certamente unica nel documento,
// abbastanza corta da restare quasi sempre dentro il margine di
// tolleranza con cui Segment può aver leggermente toccato gli spazi
// intorno a una frase.
const pageAnchorChars = 40

// errPDFNoContent è l'esito di un PDF su cui NÉ il percorso testo NÉ
// quello vision hanno prodotto niente di usabile: l'unico caso in cui
// l'ingestione di un PDF risponde con un errore invece di indicizzare
// qualcosa.
//
// Va distinto da errPDFTranscriptionFailed: qui le pagine (quelle passate
// per vision) sono state lette senza errori ma non contenevano testo — il
// documento è il sospetto. errPDFTranscriptionFailed è l'esatto contrario:
// il provider ha rifiutato ogni pagina, e il documento non c'entra.
var errPDFNoContent = errors.New("index: il pdf non ha né testo né pagine leggibili")

// errPDFTranscriptionFailed è l'esito di un PDF scansionato per cui il
// percorso vision ha provato OGNI pagina e OGNI tentativo è fallito con un
// errore del provider (vedi pdfVisionChunks): un log come
//
//	index: transcribe page 1: ai provider returned status 500: ...
//	index: transcribe page 2: ai provider returned status 500: ...
//	index: transcribe page 3: ai provider returned status 500: ...
//
// per un PDF perfettamente sano (un modello vision configurato che quel
// provider non serve, per esempio). Senza questo caso a parte,
// buildPDFChunks confonderebbe questo guasto del servizio AI con
// errPDFNoContent e manderebbe l'admin a convertire un file che non ha
// nessun bisogno di esserlo.
var errPDFTranscriptionFailed = errors.New("index: la trascrizione di ogni pagina è fallita per un errore del modello")

// transcriber restituisce il trascrittore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di translator() in translate.go, e per la stessa ragione:
// cambiare modello non deve richiedere un riavvio.
func (s *Server) transcriber(ctx context.Context) ai.Transcriber {
	if s.Vision != nil {
		return s.Vision
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("index: could not load settings: %v", err)
		return ai.NewHTTPClientWithVision("", "", "", "")
	}
	return ai.NewHTTPClientWithVision(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel, cfg.AIVisionModel)
}

// segmenter restituisce il segmentatore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di transcriber() e translator().
func (s *Server) segmenter(ctx context.Context) ai.Segmenter {
	if s.Segmenter != nil {
		return s.Segmenter
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("index: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// aiProviderConfigured dice se il provider di testo (quello che serve a
// Segment, non necessariamente il modello vision, che è a parte e
// facoltativo) è configurato, senza fare nessuna chiamata. Governa il
// gate del Task 5: senza provider l'intera funzione di indicizzazione non
// esiste, esattamente come askHandler senza Asker configurato — POST
// .../index risponde 404, la stessa degradazione silenziosa di SMTP e
// dell'arricchimento automatico del catalogo.
//
// s.Segmenter iniettato conta come "configurato" (è il punto di aggancio
// dei test, come s.Asker per askHandler), indipendentemente da cosa dicano
// le impostazioni vere.
func (s *Server) aiProviderConfigured(ctx context.Context) bool {
	if s.Segmenter != nil {
		return true
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return false
	}
	return cfg.AIBaseURL != "" && cfg.AIAPIKey != "" && cfg.AIModel != ""
}

// manualTarget risolve i tre parametri di rotta in un manuale concreto, e
// verifica che il media appartenga davvero a quel gioco e a quella lingua:
// senza il controllo, l'id di un media di un altro gioco passerebbe.
//
// Un caso "non trovato" (lingua o media inesistenti per questo gioco)
// avvolge games.ErrNotFound, cosicché writeManualTargetError possa
// distinguerlo da un parametro di rotta malformato: stesso schema di
// translateLanguageHandler (translate.go), che differenzia allo stesso
// modo per lo stesso genere di lookup.
func (s *Server) manualTarget(r *http.Request) (gameID int64, mediaID int64, lang string, media games.GameMedia, err error) {
	gameID, err = parseIDParam(r, "id")
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("invalid game id")
	}
	mediaID, err = parseIDParam(r, "mediaId")
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("invalid media id")
	}
	lang = strings.ToLower(strings.TrimSpace(chi.URLParam(r, "lang")))
	if lang == "" {
		return 0, 0, "", games.GameMedia{}, errors.New("language code is required")
	}

	gl, err := s.Games.GetLanguage(r.Context(), gameID, lang)
	if errors.Is(err, games.ErrNotFound) {
		return 0, 0, "", games.GameMedia{}, fmt.Errorf("language not found: %w", games.ErrNotFound)
	}
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("could not load language")
	}
	list, err := s.Games.ListMedia(r.Context(), gl.ID)
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("could not load media")
	}
	for _, m := range list {
		if m.ID == mediaID {
			return gameID, mediaID, lang, m, nil
		}
	}
	return 0, 0, "", games.GameMedia{}, fmt.Errorf("media not found for this game and language: %w", games.ErrNotFound)
}

// writeManualTargetError traduce l'errore di manualTarget nello status
// giusto: 404 per un gioco/lingua/media che davvero non esiste (come fa
// translateLanguageHandler per lo stesso genere di lookup), 400 per un
// parametro di rotta malformato (id non numerico, lingua mancante).
func writeManualTargetError(w http.ResponseWriter, err error) {
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

// averageUsableTextChars è la media dei caratteri di testo (dopo trim) per
// pagina: è la misura con cui buildPDFChunks decide se il layer testo
// letto da ExtractText vale davvero, o se è meglio ripiegare su vision.
// Vedi il commento su minAvgUsableCharsPerPage per il perché di una media
// e non di un totale.
func averageUsableTextChars(pages []manuals.Page) float64 {
	if len(pages) == 0 {
		return 0
	}
	total := 0
	for _, p := range pages {
		total += len(strings.TrimSpace(p.Text))
	}
	return float64(total) / float64(len(pages))
}

// detailedChunk è un manuals.SectionChunk a cui è già stato risolto il
// reference_detail: "pagina N" per un PDF, 'sezione «...»' (o "") per gli
// altri formati. È il punto in cui SectionChunk (quel che produce il
// chunker, Task 2) comincia a diventare SourceChunk (quel che si
// persiste, Task 1): manca solo Reference e LanguageCode, che
// indexMediaHandler aggiunge una volta sola per tutti i chunk di un file.
type detailedChunk struct {
	manuals.SectionChunk
	detail string
}

// sectionDetails calcola il reference_detail "a sezione" per i formati
// senza pagine (md, docx, txt): il titolo della sezione fra caporali, o
// stringa vuota quando la sezione non ne ha uno (il preambolo prima del
// primo titolo del documento).
func sectionDetails(chunks []manuals.SectionChunk) []detailedChunk {
	out := make([]detailedChunk, 0, len(chunks))
	for _, c := range chunks {
		detail := ""
		if h := strings.TrimSpace(c.Heading); h != "" {
			detail = "sezione «" + h + "»"
		}
		out = append(out, detailedChunk{SectionChunk: c, detail: detail})
	}
	return out
}

// pdfPageStats riporta, solo per il percorso vision di un PDF scansionato
// (pdfVisionChunks), quante pagine hanno contribuito testo all'indice e
// quante sono state saltate per un errore di trascrizione (isolamento
// guasti: vedi il "continue" in pdfVisionChunks). Per gli altri tre
// formati e per il percorso testo di un PDF resta il valore zero — non
// c'è nessuna pagina persa da segnalare, quindi indexMediaHandler non
// aggiunge nulla alla risposta in quei casi.
type pdfPageStats struct {
	indexedPages int
	skippedPages int
}

// buildSourceChunks instrada dal contenuto grezzo del file ai chunk
// pronti da persistere, secondo l'estensione: il cuore del Task 5. Le
// cinque strade sono quelle del brief:
//
//	.md    → il contenuto è già markdown
//	.docx  → DocxToMarkdown
//	.txt   → Segment(testo)
//	.pdf   → percorso testo o percorso vision, vedi buildPDFChunks
func (s *Server) buildSourceChunks(ctx context.Context, ext string, raw []byte) ([]detailedChunk, pdfPageStats, error) {
	switch ext {
	case ".md":
		return sectionDetails(manuals.ChunkSections(manuals.ParseSections(string(raw)))), pdfPageStats{}, nil
	case ".docx":
		md, err := manuals.DocxToMarkdown(raw)
		if err != nil {
			return nil, pdfPageStats{}, err
		}
		return sectionDetails(manuals.ChunkSections(manuals.ParseSections(md))), pdfPageStats{}, nil
	case ".txt":
		segmented, err := s.segmenter(ctx).Segment(ctx, string(raw))
		if err != nil {
			return nil, pdfPageStats{}, err
		}
		return sectionDetails(manuals.ChunkSections(manuals.ParseSections(segmented))), pdfPageStats{}, nil
	case ".pdf":
		return s.buildPDFChunks(ctx, raw)
	default:
		return nil, pdfPageStats{}, fmt.Errorf("index: estensione non supportata %q", ext)
	}
}

// buildPDFChunks riproduce la cascata di preferenza già decisa per
// l'estrazione di un PDF (era in extractManualHandler, prima che questo
// task sostituisse le quattro rotte a pagina con l'indicizzazione unica):
//
//  1. Se HasTextLayer dice che c'è un layer testo, si prova ExtractText.
//     Se il risultato è buono in media (vedi minAvgUsableCharsPerPage) si
//     segmenta quel testo (percorso testo).
//  2. Altrimenti (nessun layer testo, ExtractText fallito, o testo troppo
//     debole in media) si prova il percorso vision.
//  3. Se il percorso vision non trova nemmeno un'immagine da trascrivere
//     ma il passo 1 aveva comunque estratto del testo, per quanto debole
//     in media, quel testo diventa comunque la base del percorso testo
//     invece di un errore: è il caso di un PDF di solo testo davvero
//     corto (un cartoncino di riferimento di una pagina). Un errore
//     (errPDFNoContent) si restituisce solo quando *nessuno* dei due
//     percorsi ha prodotto niente.
func (s *Server) buildPDFChunks(ctx context.Context, raw []byte) ([]detailedChunk, pdfPageStats, error) {
	var extractedPages []manuals.Page
	if manuals.HasTextLayer(raw) {
		pages, textErr := manuals.ExtractText(raw)
		if textErr != nil {
			log.Printf("index: extract text: %v", textErr)
		} else {
			extractedPages = pages
			if averageUsableTextChars(pages) >= minAvgUsableCharsPerPage {
				chunks, err := s.pdfTextChunks(ctx, pages)
				return chunks, pdfPageStats{}, err
			}
		}
	}

	images, _ := manuals.ExtractPageImages(raw)
	if len(images) == 0 {
		if len(extractedPages) > 0 {
			chunks, err := s.pdfTextChunks(ctx, extractedPages)
			return chunks, pdfPageStats{}, err
		}
		return nil, pdfPageStats{}, errPDFNoContent
	}
	return s.pdfVisionChunks(ctx, images)
}

// pdfTextChunks è il percorso testo di un PDF: le pagine estratte da
// ExtractText non hanno titoli markdown (sono testo piatto), quindi si
// concatenano IN UNA SOLA STRINGA e si segmentano IN UNA SOLA CHIAMATA a
// Segment — non pagina per pagina. È la parte che rende possibile una
// sezione a cavallo di due pagine (il test che conta di più, vedi il
// piano): Segment vede il testo continuo e aggiunge un titolo solo dove
// comincia davvero un argomento nuovo, non a ogni riavvio di pagina.
//
// Segment non garantisce una preservazione byte-esatta (solo un tetto di
// scarto sul contenuto, vedi maxContentDeviationRatio in ai/segment.go):
// per questo la pagina di un chunk si ritrova con un'ancora (vedi
// pageStartsInSegmented), non con un offset già noto.
func (s *Server) pdfTextChunks(ctx context.Context, pages []manuals.Page) ([]detailedChunk, error) {
	joined, _ := concatTextsTracked(pageTexts(pages))
	segmented, err := s.segmenter(ctx).Segment(ctx, joined)
	if err != nil {
		return nil, err
	}

	numbers := pageNumbers(pages)
	starts := pageStartsInSegmented(pages, segmented)
	return chunksWithPageDetail(manuals.ChunkSections(manuals.ParseSections(segmented)), numbers, starts), nil
}

// pdfVisionChunks è il percorso vision di un PDF: ogni pagina è già
// trascritta in markdown da Transcribe (il modello riceve l'istruzione di
// conservare i titoli), quindi qui non c'è nessun bisogno di Segment: le
// pagine si concatenano così come sono, tenendo l'offset ESATTO (non
// un'ancora: nessuna trasformazione le tocca dopo Transcribe) a cui
// ciascuna comincia.
//
// L'isolamento guasti è per pagina (una pagina che fallisce si logga e si
// salta, le altre proseguono — vedi il "continue" sotto), ma i guasti si
// contano: se OGNI pagina fallisce con un errore, len(texts) resta a 0
// esattamente come nel caso "pagine lette ma senza testo", e i due casi
// vanno raccontati diversamente all'admin (vedi errPDFTranscriptionFailed
// contro errPDFNoContent). Quando invece l'indicizzazione riesce ma
// qualche pagina è stata saltata per un errore, lo si riporta nello
// pdfPageStats restituito: è quel che permette a indexMediaHandler di
// dire "17 pagine indicizzate, 3 saltate" invece di un silenzioso
// successo pieno su un manuale a cui in realtà mancano tre pagine.
func (s *Server) pdfVisionChunks(ctx context.Context, images []manuals.PageImage) ([]detailedChunk, pdfPageStats, error) {
	vision := s.transcriber(ctx)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Un risultato per pagina, ciascuno al PROPRIO indice: ogni goroutine
	// scrive solo results[i], quindi l'ordine del documento è garantito per
	// costruzione e non serve nessun mutex. Accodare i risultati man mano
	// che arrivano sarebbe la scelta ovvia e sarebbe sbagliata: con le
	// pagine in volo insieme l'ordine di arrivo non è quello del documento,
	// e il seq dei chunk uscirebbe rimescolato — rompendo in silenzio sia
	// attachNeighbours (che cerca il chunk vicino come seq ± 1) sia
	// l'elenco dei titoli "in ordine di seq" di Summary.
	type pageResult struct {
		text string
		err  error
	}
	results := make([]pageResult, len(images))

	// notConfigured è atomico perché più pagine possono scoprire insieme
	// che il modello vision non c'è, prima che il cancel() della prima
	// fermi le altre.
	var notConfigured atomic.Bool

	// sem limita le richieste in volo a transcribeConcurrency. Le goroutine
	// si creano tutte subito e restano in attesa sul canale: una goroutine
	// bloccata costa qualche KB di stack, molto meno della richiesta HTTP
	// che rappresenta.
	sem := make(chan struct{}, transcribeConcurrency)
	var wg sync.WaitGroup
	for i, img := range images {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// Il contesto annullato qui è la pagina che non è mai partita:
			// si registra come errore e non come pagina vuota, così i
			// conteggi sotto non confondono "non tentata" con "senza testo".
			if err := ctx.Err(); err != nil {
				results[i] = pageResult{err: err}
				return
			}
			text, err := vision.Transcribe(ctx, img.JPEG, img.Number)
			if errors.Is(err, ai.ErrNotConfigured) {
				// Un modello vision non configurato fallisce identicamente
				// per ogni pagina: non ha senso provarle tutte per scoprirlo
				// N volte, quindi la prima a scoprirlo annulla il contesto e
				// le altre non partono nemmeno.
				notConfigured.Store(true)
				cancel()
				return
			}
			results[i] = pageResult{text: text, err: err}
		}()
	}
	wg.Wait()

	if notConfigured.Load() {
		return nil, pdfPageStats{}, ai.ErrNotConfigured
	}

	// Da qui in giù è la stessa logica di prima della parallelizzazione,
	// solo letta dai risultati invece che prodotta dentro il ciclo:
	// isolamento guasti per pagina, pagine di sole illustrazioni saltate
	// senza contarle come errore.
	var texts []string
	var numbers []int
	failed := 0
	for i, img := range images {
		switch r := results[i]; {
		case r.err != nil:
			log.Printf("index: transcribe page %d: %v", img.Number, r.err)
			failed++
		case strings.TrimSpace(r.text) == "":
			// pagina di sole illustrazioni: niente da indicizzare
		default:
			texts = append(texts, r.text)
			numbers = append(numbers, img.Number)
		}
	}
	if len(texts) == 0 {
		if failed > 0 && failed == len(images) {
			// Nessuna pagina ha prodotto testo E ogni singolo tentativo è
			// fallito con un errore: è il provider che non ha risposto, non
			// il documento che non ha contenuto leggibile.
			return nil, pdfPageStats{}, errPDFTranscriptionFailed
		}
		return nil, pdfPageStats{}, errPDFNoContent
	}

	joined, starts := concatTextsTracked(texts)
	chunks := chunksWithPageDetail(manuals.ChunkSections(manuals.ParseSections(joined)), numbers, starts)
	stats := pdfPageStats{indexedPages: len(images) - failed, skippedPages: failed}
	return chunks, stats, nil
}

// chunksWithPageDetail applica ReferenceDetail = "pagina N" a ogni chunk,
// trovando N con pageForOffset.
func chunksWithPageDetail(chunks []manuals.SectionChunk, pageNumbers, starts []int) []detailedChunk {
	out := make([]detailedChunk, 0, len(chunks))
	for _, c := range chunks {
		page := pageForOffset(pageNumbers, starts, c.Offset)
		out = append(out, detailedChunk{SectionChunk: c, detail: fmt.Sprintf("pagina %d", page)})
	}
	return out
}

func pageTexts(pages []manuals.Page) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.Text
	}
	return out
}

func pageNumbers(pages []manuals.Page) []int {
	out := make([]int, len(pages))
	for i, p := range pages {
		out[i] = p.Number
	}
	return out
}

// concatTextsTracked unisce texts in ordine con una riga vuota di
// separazione, restituendo insieme il testo unito e, per ciascun testo in
// ingresso, l'offset ESATTO in cui comincia dentro quel testo unito.
func concatTextsTracked(texts []string) (string, []int) {
	var b strings.Builder
	starts := make([]int, len(texts))
	for i, t := range texts {
		if i > 0 {
			b.WriteString("\n\n")
		}
		starts[i] = b.Len()
		b.WriteString(t)
	}
	return b.String(), starts
}

// pageStartsInSegmented ritrova, per ciascuna pagina (tranne la prima, che
// comincia sempre a 0: qualunque cosa preceda l'ancora della pagina 2,
// incluso un titolo che il modello ha messo in cima al documento, è
// contenuto della pagina 1), l'offset a cui il suo testo comincia dentro
// segmented — il markdown che Segment ha prodotto dal testo unito delle
// pagine originali.
//
// Segment può solo INSERIRE righe di titolo, non riscrivere il resto (è
// il suo contratto, verificato da un tetto sullo scarto di lunghezza): il
// testo originale di ciascuna pagina dovrebbe quindi comparire ancora,
// nello stesso ordine, dentro segmented. Si cerca perciò un'ancora (i primi
// pageAnchorChars caratteri del testo ORIGINALE della pagina, non del
// segmentato) con una ricerca SOLO IN AVANTI a partire da un cursore che
// avanza pagina dopo pagina: è quel che impedisce a una frase che si
// ripete nel documento di essere scambiata per l'inizio di una pagina
// successiva — lo stesso principio, applicato qui alle pagine invece che
// ai chunk, del commento su chunkPiece in chunk.go.
//
// Se l'ancora di una pagina non si ritrova (il modello l'ha toccata più di
// quanto il tetto di tolleranza dovrebbe permettere), quella pagina eredita
// l'offset della precedente: i suoi chunk finiscono attribuiti alla pagina
// prima, una degradazione ragionevole per un caso che il tetto di
// tolleranza di Segment dovrebbe già rendere raro. Questa garanzia dipende
// da pageForOffset, che deve saltare i confini ripetuti prodotti qui:
// senza quel salto un confronto ingenuo attribuirebbe l'ESATTO CONTRARIO
// (alla pagina il cui ancoraggio è fallito, non a quella prima) — vedi il
// commento su pageForOffset.
func pageStartsInSegmented(pages []manuals.Page, segmented string) []int {
	starts := make([]int, len(pages))
	cursor := 0
	for i, p := range pages {
		if i == 0 {
			starts[0] = 0
			continue
		}
		anchor := anchorText(p.Text)
		if anchor == "" {
			starts[i] = starts[i-1]
			continue
		}
		idx := strings.Index(segmented[min(cursor, len(segmented)):], anchor)
		if idx < 0 {
			starts[i] = starts[i-1]
			continue
		}
		starts[i] = cursor + idx
		cursor = starts[i] + len(anchor)
	}
	return starts
}

// anchorText restituisce i primi pageAnchorChars caratteri (tagliati su un
// confine di rune valido, mai a metà di un carattere multi-byte) del testo
// di una pagina, dopo trim: l'ancora usata da pageStartsInSegmented per
// ritrovare dove comincia quella pagina nel testo segmentato. Stringa
// vuota per una pagina senza testo (nessun'ancora possibile).
func anchorText(pageText string) string {
	t := strings.TrimSpace(pageText)
	if t == "" {
		return ""
	}
	if len(t) <= pageAnchorChars {
		return t
	}
	cut := pageAnchorChars
	for cut > 0 && !utf8.RuneStart(t[cut]) {
		cut--
	}
	return t[:cut]
}

// pageForOffset trova, per un offset dentro il testo unito, l'ultima
// pagina il cui inizio (starts[i]) è <= offset: esattamente "la pagina il
// cui intervallo di offset contiene chunk.Offset" del brief. starts è per
// costruzione non decrescente (sia in concatTextsTracked sia in
// pageStartsInSegmented il cursore avanza sempre), quindi l'ultima che
// soddisfa la condizione è quella giusta — MA solo fra i confini VERI
// (starts[i] > starts[i-1]): un confine ripetuto è il fallback silenzioso
// di un'ancora non trovata in pageStartsInSegmented (starts[i] =
// starts[i-1]), non un nuovo inizio di pagina davvero letto. Senza questo
// filtro, un confronto ingenuo "offset >= starts[i]" per ogni i farebbe
// vincere sempre l'INDICE PIÙ ALTO fra due starts uguali — cioè la pagina
// il cui ancoraggio è FALLITO — e quel guasto si mangerebbe all'indietro
// anche i chunk che appartengono davvero alla pagina precedente, ben
// ancorata: l'opposto di "eredita l'offset della precedente" che
// pageStartsInSegmented promette nel suo commento. Saltando i confini
// ripetuti, un'ancora fallita per la pagina i lascia correttamente i suoi
// chunk (e quelli della pagina prima) attribuiti all'ultimo confine vero
// trovato, cioè alla pagina precedente.
func pageForOffset(pageNumbers, starts []int, offset int) int {
	page := pageNumbers[0]
	for i := 1; i < len(starts); i++ {
		if starts[i] <= starts[i-1] {
			continue // confine non vero: un'ancora fallita, non un nuovo inizio di pagina
		}
		if offset < starts[i] {
			break
		}
		page = pageNumbers[i]
	}
	return page
}

// sourceReference decide la Reference da salvare per questo media: parte
// dal titolo (game_media.title — MAI da url_or_path, che è uno sha256
// senza nessun significato per chi legge una citazione), e la disambigua
// contro le fonti GIÀ indicizzate dello stesso gioco (used, letto da
// Summary DOPO aver già cancellato le fonti precedenti di QUESTO stesso
// media: così re-indicizzare lo stesso file con lo stesso titolo non si
// scontra con la propria vecchia voce e non guadagna un suffisso senza
// motivo).
//
// title è testo libero e nullable (game_media.title): quando è vuoto si
// ripiega su "Documento", per non salvare mai una reference vuota. Senza
// collisione la reference è il titolo così com'è; con una collisione si
// aggiunge la lingua; se collide ancora (due fonti con lo stesso titolo E
// nella stessa lingua) un ordinale progressivo. Senza questa
// disambiguazione la mappa reference → percorso file che il Task 7
// costruisce dalle hit di ricerca punterebbe al file sbagliato per uno dei
// due media — è precisamente il bug che quel task esiste per chiudere.
func sourceReference(title, langCode string, used map[string]bool) string {
	base := strings.TrimSpace(title)
	if base == "" {
		base = "Documento"
	}
	candidate := base
	if used[strings.ToLower(candidate)] {
		candidate = fmt.Sprintf("%s (%s)", base, langCode)
	}
	if used[strings.ToLower(candidate)] {
		for i := 2; ; i++ {
			try := fmt.Sprintf("%s (%s) #%d", base, langCode, i)
			if !used[strings.ToLower(try)] {
				candidate = try
				break
			}
		}
	}
	return candidate
}

// indexErrorResponse traduce un errore di buildSourceChunks nello status e
// nel messaggio che arrivano all'admin: la tabella del brief, in codice.
// Ogni caso dice una cosa diversa e vera, mai un guasto generico — è
// l'unica cosa che sta fra l'admin e un vicolo cieco quando l'ingestione
// non riesce.
func indexErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, ai.ErrNotConfigured):
		// Il gate all'inizio dell'handler garantisce che il provider di
		// TESTO sia configurato: se ai.ErrNotConfigured emerge comunque da
		// buildSourceChunks, può venire solo dal percorso vision di un PDF
		// scansionato (vedi pdfVisionChunks), che ha il suo campo a parte
		// nelle impostazioni.
		return http.StatusUnprocessableEntity,
			`Questo file è un PDF scansionato: serve un modello che legga le immagini. ` +
				`Configuralo nel campo "Modello per i manuali scansionati" nelle impostazioni.`
	case errors.Is(err, errPDFTranscriptionFailed):
		// A differenza del caso sopra (nessun modello configurato), qui un
		// modello per i manuali scansionati C'È: ha solo risposto con un
		// errore su ogni pagina (il caso reale: un modello che quel
		// provider non serve per le immagini, con il provider che risponde
		// 500 su ogni pagina). Il documento non è sospetto, quindi niente
		// suggerimento di conversione — sarebbe mandare l'admin a fare
		// l'unica cosa che non serve.
		return http.StatusUnprocessableEntity,
			`La lettura di questo PDF scansionato non è riuscita: il modello configurato per i manuali scansionati ha risposto con un errore su ogni pagina. Controlla il modello nelle impostazioni e riprova.`
	case errors.Is(err, errPDFNoContent):
		return http.StatusUnprocessableEntity,
			"Questo PDF non ha né testo né immagini leggibili: se hai il documento originale, prova a convertirlo in .docx o .txt invece che in PDF."
	case errors.Is(err, ai.ErrSegmentationRejected):
		return http.StatusUnprocessableEntity,
			"La lettura di questo file non è affidabile: il risultato non corrisponde al testo originale del documento, come se il modello lo avesse riassunto invece di limitarsi ad aggiungere titoli. Riprova; se continua a succedere, prova un file più semplice o in un altro formato."
	case errors.Is(err, manuals.ErrDocumentXMLTooLarge):
		return http.StatusUnprocessableEntity,
			"Questo file .docx è troppo grande per essere letto: prova a semplificarlo o a esportarlo in un altro formato."
	case errors.Is(err, manuals.ErrDocxNoText):
		return http.StatusUnprocessableEntity,
			"Questo file .docx non contiene testo: se sono pagine scansionate o immagini incollate nel documento, salvalo come PDF invece di .docx — quel formato legge anche le immagini."
	default:
		return http.StatusUnprocessableEntity,
			"Non è stato possibile leggere questo file: riprova, oppure prova a salvarlo in un altro formato tra quelli supportati (PDF, txt, md, docx)."
	}
}

// indexMediaHandler è l'ingestione unica per i quattro formati (Task 5):
// da un file già caricato (createMediaHandler/createFileMediaHandler, non
// tocco questo task) ai chunk cercabili, in una sola richiesta. Sostituisce
// le quattro rotte a pagina (extract/pages GET/PUT/DELETE) che il manuale
// scansionato usava prima: niente più bozza da correggere a mano, niente
// più testo conservato — solo i chunk.
func (s *Server) indexMediaHandler(w http.ResponseWriter, r *http.Request) {
	// Gate unico sul provider: senza un provider di testo configurato
	// questa funzione non esiste, esattamente come askHandler senza Asker.
	// Il pannello non mostra nemmeno il bottone in quel caso: nessun
	// partecipante ci arriva navigando.
	if !s.aiProviderConfigured(r.Context()) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	gameID, mediaID, lang, media, err := s.manualTarget(r)
	if err != nil {
		writeManualTargetError(w, err)
		return
	}
	if media.Type != games.MediaTypeFile {
		writeError(w, http.StatusConflict, "questo media non è un file caricato")
		return
	}

	ext := strings.ToLower(filepath.Ext(media.URLOrPath))
	switch ext {
	case ".md", ".docx", ".txt", ".pdf":
	default:
		writeError(w, http.StatusConflict, "formato non supportato per l'indicizzazione")
		return
	}

	f, err := s.Storage.Open(media.URLOrPath)
	if err != nil {
		log.Printf("index: open %s: %v", media.URLOrPath, err)
		writeError(w, http.StatusNotFound, "il file non è più sul disco")
		return
	}
	raw, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		log.Printf("index: read %s: %v", media.URLOrPath, err)
		writeError(w, http.StatusInternalServerError, "non è stato possibile leggere il file")
		return
	}

	chunks, pageStats, err := s.buildSourceChunks(r.Context(), ext, raw)
	if err != nil {
		status, msg := indexErrorResponse(err)
		writeError(w, status, msg)
		return
	}
	if len(chunks) == 0 {
		writeError(w, http.StatusUnprocessableEntity,
			"Questo file non contiene testo utilizzabile: prova a convertirlo o a scriverlo in un altro formato.")
		return
	}

	// Si cancellano PRIMA le eventuali vecchie fonti di QUESTO media: la
	// lettura di Summary subito dopo deve vedere le referenze delle ALTRE
	// fonti del gioco, non anche la propria di prima del re-indicizzare —
	// altrimenti una riesecuzione con lo stesso titolo si scontrerebbe con
	// se stessa e guadagnerebbe un suffisso di lingua senza nessun secondo
	// media coinvolto.
	if err := s.Manuals.DeleteSource(r.Context(), mediaID); err != nil {
		log.Printf("index: delete previous chunks of media %d: %v", mediaID, err)
		writeError(w, http.StatusInternalServerError, "could not replace the source")
		return
	}
	summary, err := s.Manuals.Summary(r.Context(), gameID)
	if err != nil {
		log.Printf("index: summary for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not check existing sources")
		return
	}
	used := make(map[string]bool, len(summary.Sources))
	for _, src := range summary.Sources {
		used[strings.ToLower(src.Reference)] = true
	}
	title := ""
	if media.Title != nil {
		title = *media.Title
	}
	reference := sourceReference(title, lang, used)

	sourceChunks := make([]manuals.SourceChunk, 0, len(chunks))
	for _, c := range chunks {
		sourceChunks = append(sourceChunks, manuals.SourceChunk{
			ReferenceType:   "document",
			Reference:       reference,
			ReferenceDetail: c.detail,
			Heading:         c.Heading,
			LanguageCode:    lang,
			Seq:             c.Seq,
			Text:            c.Text,
		})
	}
	if err := s.Manuals.ReplaceSource(r.Context(), gameID, &mediaID, sourceChunks); err != nil {
		log.Printf("index: replace source for media %d: %v", mediaID, err)
		writeError(w, http.StatusInternalServerError, "could not save the index")
		return
	}

	// "chunks"/"reference" bastano per un'indicizzazione piena. Ma quando
	// pageStats dice che alcune pagine di un PDF scansionato sono state
	// saltate per un errore di trascrizione, un successo pieno e uno
	// parziale sarebbero indistinguibili per l'admin: senza pagesIndexed/
	// pagesSkipped vedrebbe solo "N sezioni indicizzate" senza sapere che
	// al manuale mancano delle pagine — e la chat risponderebbe poi con
	// sicurezza da un regolamento incompleto. Campi additivi in
	// camelCase, aggiunti SOLO quando c'è davvero qualcosa da segnalare:
	// il resto del formato risposta (e il frontend che lo legge) resta
	// invariato per ogni altro caso.
	// Le tre domande suggerite si rigenerano qui, best-effort: un errore si
	// logga e si ignora. Aggiungere qualche secondo a un'operazione che su
	// un manuale scansionato ne dura più di cento non si nota, ma
	// trasformare un'indicizzazione riuscita in un errore per una domanda
	// suggerita sarebbe fuori scala rispetto al valore della feature.
	//
	// Le posizioni che l'admin ha riscritto a mano non si toccano (all =
	// false), e se sono tutte e tre a mano la chiamata al modello non parte
	// nemmeno.
	if existing, qErr := s.Manuals.SuggestedQuestions(r.Context(), gameID); qErr != nil {
		log.Printf("index: read suggested questions for game %d: %v", gameID, qErr)
	} else if !allQuestionsEdited(existing) {
		if qErr := s.regenerateQuestions(r.Context(), gameID, false); qErr != nil {
			log.Printf("index: suggested questions for game %d: %v", gameID, qErr)
		}
	}

	resp := map[string]any{"reference": reference, "chunks": len(sourceChunks)}
	if pageStats.skippedPages > 0 {
		resp["pagesIndexed"] = pageStats.indexedPages
		resp["pagesSkipped"] = pageStats.skippedPages
	}
	writeJSON(w, http.StatusOK, resp)
}

// deleteMediaIndexHandler rimuove tutti i chunk indicizzati di un media:
// niente gate sul provider, perché ripulire un'indicizzazione esistente
// deve restare possibile anche se nel frattempo l'admin ha tolto la
// configurazione del provider AI.
func (s *Server) deleteMediaIndexHandler(w http.ResponseWriter, r *http.Request) {
	_, mediaID, _, _, err := s.manualTarget(r)
	if err != nil {
		writeManualTargetError(w, err)
		return
	}
	if err := s.Manuals.DeleteSource(r.Context(), mediaID); err != nil {
		log.Printf("index: delete source for media %d: %v", mediaID, err)
		writeError(w, http.StatusInternalServerError, "could not delete the index")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
