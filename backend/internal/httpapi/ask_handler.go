package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/faq"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
	"boardgames-manager/internal/websearch"
)

// asker restituisce l'agente per questa richiesta: quello iniettato se c'è,
// altrimenti uno costruito dalle impostazioni. Stesso schema di
// translator() e transcriber().
func (s *Server) asker(ctx context.Context) ai.Asker {
	if s.Asker != nil {
		return s.Asker
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("ask: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// aiConfigured dice se un provider è impostato, senza fare richieste. Serve
// a chatAvailability: le schede pubbliche devono sapere se mostrare la chat
// prima che qualcuno faccia una domanda.
func (s *Server) aiConfigured(ctx context.Context) bool {
	if s.Asker != nil {
		return true
	}
	// Un generatore iniettato (test) vale come provider configurato: stesso
	// schema del caso Asker qui sopra.
	if s.Suggester != nil {
		return true
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return false
	}
	return cfg.AIBaseURL != "" && cfg.AIAPIKey != "" && cfg.AIModel != ""
}

// webSearcher restituisce la ricerca web per le FAQ, o nil se non c'è una
// chiave: nil vuol dire che il tool FAQ non si dichiara.
func (s *Server) webSearcher(ctx context.Context) websearch.Searcher {
	if s.WebSearch != nil {
		return s.WebSearch
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil || cfg.TavilyAPIKey == "" {
		return nil
	}
	return websearch.NewTavily(cfg.TavilyAPIKey)
}

// tavilyConfigured dice se la ricerca nel forum è possibile, senza fare
// richieste. Con hasBGGID è la metà "forum" della disponibilità.
func (s *Server) tavilyConfigured(ctx context.Context) bool {
	return s.webSearcher(ctx) != nil
}

func hasBGGID(g games.Game) bool {
	return g.BGGID != nil && *g.BGGID != ""
}

// chatAvailability è la SOLA regola che decide quali agenti ha un gioco:
// la usano l'handler (404 per un agente assente) e le schede pubbliche
// (chat.rules / chat.strategy), così la chat non può promettere un agente
// che poi risponde 404. forumOK = chiave Tavily e bggId.
func chatAvailability(aiOK, hasChunks, forumOK bool) (rules, strategy bool) {
	if !aiOK {
		return false, false
	}
	return hasChunks || forumOK, forumOK
}

// askHTTPRequest è la forma che manda deep-chat: la conversazione intera,
// tagliata dal componente a requestBodyLimits.maxMessages.
type askHTTPRequest struct {
	Agent    string `json:"agent"`
	Messages []struct {
		Role string `json:"role"`
		Text string `json:"text"`
	} `json:"messages"`
}

// I tre tetti sotto esistono perché questa è l'unica rotta del progetto che
// trasforma byte anonimi in denaro: è pubblica, non autenticata, e quel che
// arriva finisce dentro il prompt di un provider che si paga a token. Il
// rate limit (20/min per IP) limita la frequenza, non la dimensione: senza
// questi tetti una singola richiesta da 50 MB, o cento turni da 100 KB,
// sarebbero una richiesta legittima.
//
// I valori sono larghi rispetto all'uso vero — una domanda sulle regole
// battuta al telefono sta in poche centinaia di caratteri, e deep-chat manda
// al massimo gli ultimi maxMessages turni — e stretti rispetto all'abuso.
const (
	askMaxBodyBytes  = 128 << 10 // 128 KB di JSON
	askMaxTurns      = 30
	askMaxTurnChars  = 4000
	askTotalMaxChars = 24000
	// askMaxFAQSearches tiene basso il costo Tavily di una singola domanda
	// (MaxToolIterations da solo lascerebbe fino a 5 chiamate al tool, 5
	// crediti): oltre il tetto la closure non chiama più Tavily, e dice al
	// modello di rispondere con quel che ha già.
	askMaxFAQSearches = 2
)

// trimTurns riduce la conversazione ai tetti, tagliando dalla TESTA: la
// domanda appena scritta è l'ultimo messaggio, quindi a cadere è il contesto
// più vecchio, mai la domanda. Un turno singolo non può da solo sforare il
// totale (askMaxTurnChars è molto minore di askTotalMaxChars), quindi
// l'ultimo turno sopravvive sempre.
func trimTurns(turns []ai.Turn) []ai.Turn {
	if len(turns) > askMaxTurns {
		turns = turns[len(turns)-askMaxTurns:]
	}
	total := 0
	for i := len(turns) - 1; i >= 0; i-- {
		total += len(turns[i].Text)
		if total > askTotalMaxChars {
			return turns[i+1:]
		}
	}
	return turns
}

func (s *Server) askHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, askMaxBodyBytes)
	var body askHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// MaxBytesReader fa fallire il Decode con un errore suo: per chi
		// scrive dal tavolo la differenza fra "JSON malformato" e "troppo
		// lungo" non cambia nulla, la risposta è la stessa.
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	turns := make([]ai.Turn, 0, len(body.Messages))
	for _, m := range body.Messages {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if len(text) > askMaxTurnChars {
			text = text[:askMaxTurnChars]
		}
		// deep-chat chiama "ai" quel che il formato OpenAI chiama
		// "assistant": la traduzione va fatta qui, una volta.
		role := "user"
		if m.Role == "ai" || m.Role == "assistant" {
			role = "assistant"
		}
		turns = append(turns, ai.Turn{Role: role, Text: text})
	}
	turns = trimTurns(turns)
	if len(turns) == 0 {
		writeError(w, http.StatusBadRequest, "serve una domanda")
		return
	}

	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}

	// Summary porta insieme, in una lettura sola, se il gioco ha fonti
	// indicizzate e i titoli raggruppati per fonte: il secondo dato
	// costruisce l'indice del prompt (sotto), quindi non serve una seconda
	// query solo per sapere se hasChunks è vero.
	summary, err := s.Manuals.Summary(r.Context(), gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load the manual")
		return
	}
	// Un agente sconosciuto o assente è il Manuale: una scheda rimasta
	// aperta durante un aggiornamento manda ancora il body di prima.
	agent := ai.AgentRules
	if body.Agent == string(ai.AgentStrategy) {
		agent = ai.AgentStrategy
	}
	searcher := s.webSearcher(r.Context())
	forumOK := searcher != nil && hasBGGID(game)
	// aiOK = true: senza provider è l'asker a rispondere ErrNotConfigured,
	// che diventa lo stesso 404 più sotto.
	rulesOK, strategyOK := chatAvailability(true, summary.HasChunks, forumOK)
	if (agent == ai.AgentRules && !rulesOK) || (agent == ai.AgentStrategy && !strategyOK) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// La lingua preferita è quella base del gioco: è quella in cui la
	// scheda pubblica mostra tutto il resto.
	preferLang := "it"
	if langs, err := s.Games.ListLanguages(r.Context(), gameID); err == nil {
		for _, l := range langs {
			if l.IsBaseLanguage {
				preferLang = l.LanguageCode
				break
			}
		}
	}

	// citations accumula, mentre la ricerca gira, la mappa reference →
	// bersaglio del link, presa SOLO dalle hit che il modello ha davvero
	// ricevuto in questa richiesta (mai da un elenco di media letto a
	// parte): è quel che rende il link corretto anche con due manuali
	// dello stesso gioco, dove la reference è già disambiguata dal Task 5
	// e non corrisponde più a game_media.title.
	citations := map[string]*citationTarget{}

	// La closure di ricerca è legata al gioco: il game_id NON è un
	// parametro del tool, così il modello non può leggere il manuale di un
	// altro gioco.
	search := func(ctx context.Context, keywords []string) (string, error) {
		hits, missing, err := s.Manuals.Search(ctx, gameID, preferLang, keywords)
		if err != nil {
			return "", err
		}
		for _, h := range hits {
			if h.Reference == "" {
				continue
			}
			if _, ok := citations[h.Reference]; !ok {
				citations[h.Reference] = &citationTarget{
					referenceType: h.ReferenceType,
					mediaPath:     h.MediaPath,
				}
			}
		}
		return manuals.MarshalHits(hits, missing)
	}

	// faqRefs tiene, in ordine d'arrivo, le reference dei thread restituiti:
	// servono ad appendForumSources quando il modello non li cita alla lettera.
	var faqRefs []string

	// forumSearch costruisce la closure di ricerca nel forum, legata al
	// gioco come search: né il nome né il bggId sono parametri del tool. Un
	// solo contatore per richiesta, qualunque forum si interroghi: il tetto
	// di askMaxFAQSearches vale per la domanda, non per il forum.
	forumSearches := 0
	forumSearch := func(forum faq.Forum) ai.FAQSearchFunc {
		bggID := *game.BGGID
		return func(ctx context.Context, query string) (string, error) {
			forumSearches++
			if forumSearches > askMaxFAQSearches {
				// Nessuna chiamata a Tavily oltre il tetto: un risultato
				// normale (non un errore), così il modello risponde con
				// quel che ha invece di vedere un guasto che non c'è.
				return "Hai già cercato nel forum abbastanza per questa domanda: rispondi con quello che hai.", nil
			}
			hits, err := faq.Search(ctx, searcher, s.BGG, game.Name, bggID, forum, query)
			if err != nil {
				return "", err
			}
			out := make([]manuals.SourceHit, 0, len(hits))
			for _, h := range hits {
				// La reference è unica per RICHIESTA, non per chiamata al
				// tool: faq.Search non disambigua più da sola (vede solo le
				// hit della propria chiamata), quindi due cerca_nelle_faq
				// nella stessa domanda possono trovare due thread diversi
				// con lo stesso Subject. Se la reference è già presa da
				// un'ALTRA FAQ (thread diverso), si disambigua qui con l'id
				// del thread prima di registrarla e prima di metterla nel
				// payload per il modello; lo stesso thread, ritrovato in una
				// chiamata successiva, riusa la sua reference invariata.
				ref := h.Reference
				if existing, ok := citations[ref]; ok && existing.referenceType == "faq" && existing.url != h.ThreadURL {
					ref = h.Reference + " #" + h.ThreadID
				}
				target, ok := citations[ref]
				if !ok {
					target = &citationTarget{referenceType: "faq", url: h.ThreadURL, commentURLs: map[string]string{}}
					citations[ref] = target
					faqRefs = append(faqRefs, ref)
				}
				// Una reference già presa da un documento non si tocca (non
				// succede in pratica: le FAQ cominciano con "BGG: ").
				if target.referenceType == "faq" && h.CommentURL != "" {
					if _, seen := target.commentURLs[h.ReferenceDetail]; !seen {
						target.commentURLs[h.ReferenceDetail] = h.CommentURL
					}
				}
				out = append(out, manuals.SourceHit{
					ReferenceType: "faq", Reference: ref,
					ReferenceDetail: h.ReferenceDetail, Text: h.Text,
				})
			}
			var missing []string
			if len(out) == 0 {
				missing = []string{query}
			}
			return manuals.MarshalHits(out, missing)
		}
	}

	req := ai.AskRequest{
		Agent:       agent,
		GameName:    game.Name,
		Turns:       turns,
		CorpusIndex: formatCorpusIndex(summary.Sources),
	}
	if summary.HasChunks {
		req.Search = search
	}
	if forumOK {
		if agent == ai.AgentStrategy {
			req.SearchStrategy = forumSearch(faq.ForumStrategy)
		} else {
			req.SearchFAQ = forumSearch(faq.ForumRules)
		}
	}
	answer, err := s.asker(r.Context()).Ask(r.Context(), req)
	if errors.Is(err, ai.ErrNotConfigured) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		log.Printf("ask about game %d: %v", gameID, err)
		writeError(w, http.StatusBadGateway,
			"Non riesco a rispondere in questo momento. Riprova tra poco.")
		return
	}

	// deep-chat legge {"text": ...}.
	text := appendForumSources(linkifyCitations(answer, citations), faqRefs, citations)
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}

// formatCorpusIndex costruisce l'indice per fonte che finisce in
// AskRequest.CorpusIndex: è quel che evita al modello la chiamata
// esplorativa al tool quando l'indice già basta a scegliere le parole
// chiave giuste. Raggruppato per reference (fonte), coi titoli
// nell'ordine di seq che Summary già restituisce.
//
// Esempio con una sola fonte:
//
//	Fonti: Carcassonne_Base_&_Fiume_ITA.pdf — Preparazione · Turno del giocatore · Fase di Upkeep
//
// Con più fonti le voci si accodano separate da "; ". Una fonte senza
// titoli (nessun heading rilevato) non produce una voce vuota.
func formatCorpusIndex(sources []manuals.SourceHeadings) string {
	var parts []string
	for _, src := range sources {
		if len(src.Headings) == 0 {
			continue
		}
		parts = append(parts, src.Reference+" — "+strings.Join(src.Headings, " · "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Fonti: " + strings.Join(parts, "; ")
}

// citationTarget è dove porta il link di una reference conosciuta:
// referenceType decide la forma dell'URL (documento vs FAQ), mediaPath è
// game_media.url_or_path per un documento ("" per una FAQ, il cui link
// vive in url/commentURLs invece che nella reference).
type citationTarget struct {
	referenceType string
	mediaPath     string
	// url è il thread di una FAQ; commentURLs porta ogni reference_detail
	// ("commento del 07/01/2019") al link del suo commento. Se due commenti
	// dello stesso thread hanno la stessa data vince il primo.
	url         string
	commentURLs map[string]string
}

// appendForumSources chiude il buco che linkifyCitations non può chiudere:
// il modello riporta quel che dice il forum ma parafrasa la citazione («un
// commento del 25/03/2020…») invece di copiare "BGG: <titolo>, …", e senza
// il testo esatto non c'è niente a cui attaccare il link. I thread li
// conosciamo comunque (sono quelli restituiti in questa richiesta, in
// faqRefs nell'ordine in cui sono arrivati): se la risposta parla del forum
// e non linka nessuno di loro, li si elenca in fondo.
//
// «Parla del forum» è volutamente grezzo (forum/BGG nel testo): il prompt
// chiede di presentare il materiale come «sul forum di BGG…», e senza questo
// filtro una risposta presa tutta dal manuale si porterebbe dietro thread
// che il modello ha letto e scartato.
func appendForumSources(answer string, faqRefs []string, citations map[string]*citationTarget) string {
	if len(faqRefs) == 0 {
		return answer
	}
	lower := strings.ToLower(answer)
	if !strings.Contains(lower, "forum") && !strings.Contains(lower, "bgg") {
		return answer
	}
	links := make([]string, 0, len(faqRefs))
	for _, ref := range faqRefs {
		target := citations[ref]
		if target == nil || target.url == "" {
			continue
		}
		if strings.Contains(answer, "]("+target.url) {
			return answer
		}
		for _, u := range target.commentURLs {
			if strings.Contains(answer, "]("+u) {
				return answer
			}
		}
		// Le parentesi quadre in un titolo chiuderebbero il testo del link.
		title := strings.NewReplacer("[", "", "]", "").Replace(strings.TrimPrefix(ref, "BGG: "))
		links = append(links, fmt.Sprintf("[%s](%s)", title, target.url))
	}
	if len(links) == 0 {
		return answer
	}
	return answer + "\n\nDal forum di BGG: " + strings.Join(links, " · ")
}

// minReferenceLength è la soglia (in rune) sotto la quale una reference
// NON si sostituisce. game_media.title è testo libero: un titolo cortissimo
// come "A" trasformerebbe OGNI "A" della risposta in un link, rendendola
// illeggibile — il costo di un falso positivo qui è più alto del beneficio
// di linkare un titolo che, nella pratica, un admin non sceglie mai così
// corto. Sotto la soglia si preferisce lasciare la citazione come testo
// semplice piuttosto che rischiare di linkare mezza risposta.
const minReferenceLength = 4

// linkifyCitations trasforma le citazioni della risposta in link markdown,
// usando SOLO la mappa reference → bersaglio costruita dalle hit
// EFFETTIVAMENTE restituite al modello in questa richiesta (vedi search()
// sopra). È il pezzo che chiude il cerchio: una risposta generata non va
// creduta sulla fiducia, si apre la fonte giusta e si verifica — e in una
// discussione sulle regole è la differenza fra un aiuto e un oracolo.
//
// La riscrittura è nostra e non del modello: chiedere a un modello
// economico di costruire URL corretti è un modo affidabile di ottenere URL
// sbagliati. Il modello cita "reference, reference_detail" esattamente
// come richiesto dal prompt di sistema (vedi askSystemPrompt in
// internal/ai/ask.go): qui si cercano le occorrenze letterali di ogni
// reference conosciuta, opzionalmente seguite da ", pagina N", e si
// sostituiscono con un link.
//
// Prima (con la regex "pag. N") la riscrittura era cieca al manuale a cui
// la citazione si riferiva: con due manuali sostituiva ogni "pag. N" con lo
// stesso file, e "il regolamento inglese, pag. 12" diventava un link a
// pagina 12 di quello ITALIANO — un link sbagliato, peggio di nessun link.
// Ora la mappa viene dalle hit vere: ogni reference porta il proprio
// mediaPath, quindi due manuali distinti non collidono più.
//
// Le reference più lunghe si provano per prime: un'alternanza regex sceglie
// la prima che combacia, e senza quest'ordine una reference che è prefisso
// letterale di un'altra (raro, ma non impossibile) troncherebbe il link a
// metà nome.
//
// Riconosce anche ", pagina N" e ", commento del DD/MM/YYYY": il primo per
// un documento, il secondo per una FAQ, il cui dettaglio è la data del
// commento citato (vedi faq.Hit.ReferenceDetail).
func linkifyCitations(answer string, citations map[string]*citationTarget) string {
	// Se il modello ha già prodotto un link, non si raddoppia.
	if strings.Contains(answer, "](/api/uploads/") || strings.Contains(answer, "](https://boardgamegeek.com/") {
		return answer
	}
	if len(citations) == 0 {
		return answer
	}

	references := make([]string, 0, len(citations))
	for r := range citations {
		if r == "" || utf8.RuneCountInString(r) < minReferenceLength {
			continue
		}
		if strings.Contains(r, "]") {
			// Un "]" dentro il testo del link chiuderebbe la sintassi
			// markdown in anticipo (il resto della reference finirebbe
			// come testo normale, seguito da un "(url)" letterale): si
			// salta la sostituzione piuttosto che produrre markdown
			// corrotto. Caso remoto (richiede un game_media.title con
			// quel carattere), ma il controllo costa una riga.
			continue
		}
		references = append(references, r)
	}
	if len(references) == 0 {
		return answer
	}
	sort.Slice(references, func(i, j int) bool { return len(references[i]) > len(references[j]) })

	quoted := make([]string, len(references))
	for i, r := range references {
		quoted[i] = regexp.QuoteMeta(r)
	}
	// Il pattern si ricompila a ogni richiesta, e non può diventare una
	// variabile di pacchetto: incorpora l'alternanza delle reference
	// EFFETTIVAMENTE trovate in QUESTA richiesta (vedi search() sopra), che
	// cambia per gioco e per domanda. Una variabile statica sarebbe o
	// sbagliata (reference di un altro gioco) o ricostruita comunque a ogni
	// chiamata, vanificando la cache.
	pattern := regexp.MustCompile(`(?:` + strings.Join(quoted, "|") + `)(?:, pagina (\d+)|, (commento del \d{2}/\d{2}/\d{4}))?`)

	return pattern.ReplaceAllStringFunc(answer, func(match string) string {
		sub := pattern.FindStringSubmatch(match)
		page := sub[1]
		comment := sub[2]

		var reference string
		for _, r := range references {
			if strings.HasPrefix(match, r) {
				reference = r
				break
			}
		}
		target, ok := citations[reference]
		if !ok {
			return match
		}

		switch target.referenceType {
		case "document":
			if target.mediaPath == "" {
				return match
			}
			href := "/api/uploads/" + target.mediaPath
			if page != "" {
				href += "#page=" + page
			}
			return fmt.Sprintf("[%s](%s)", match, href)
		case "faq":
			href := target.url
			if u, ok := target.commentURLs[comment]; ok && comment != "" {
				href = u
			}
			if href == "" {
				return match
			}
			return fmt.Sprintf("[%s](%s)", match, href)
		default:
			return match
		}
	})
}
