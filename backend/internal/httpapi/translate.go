package httpapi

import (
	"context"
	"errors"
	"log"
	"strings"

	"boardgames-manager/internal/ai"
)

// translator restituisce il traduttore da usare per questa richiesta:
// quello iniettato se c'è (i test), altrimenti uno costruito al volo dalle
// impostazioni salvate. Costruirlo per richiesta è ciò che permette
// all'admin di cambiare provider senza riavviare il container.
func (s *Server) translator(ctx context.Context) ai.Translator {
	if s.AI != nil {
		return s.AI
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		// Senza impostazioni non si traduce; il chiamante tratta la cosa
		// come "AI non configurata", che è l'esito giusto.
		log.Printf("translate: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// translateDescription è l'unico posto che decide se una descrizione va
// tradotta. Non restituisce mai un errore: se non si può tradurre, il
// testo originale è il risultato giusto — un gioco senza descrizione
// tradotta è comunque un gioco in catalogo, e la scheda ha il bottone per
// riprovare.
func (s *Server) translateDescription(ctx context.Context, source, targetLang string) string {
	if strings.TrimSpace(source) == "" {
		return source
	}
	// L'originale BGG è già in inglese: chiedere di tradurlo in inglese
	// costerebbe una chiamata per riavere lo stesso testo, peggiorato.
	if strings.EqualFold(strings.TrimSpace(targetLang), "en") {
		return source
	}

	out, err := s.translator(ctx).Translate(ctx, source, targetLang)
	if errors.Is(err, ai.ErrNotConfigured) {
		return source
	}
	if err != nil {
		log.Printf("translate into %q: %v", targetLang, err)
		return source
	}
	return out
}
