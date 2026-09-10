package httpapi_test

import (
	"context"

	"boardgames-manager/internal/ai"
)

// fakeTranslator sta al posto del provider AI nei test, come
// fakeBGGClient sta al posto di BoardGameGeek.
type fakeTranslator struct {
	out      string
	err      error
	calls    int
	lastText string
	lastLang string
}

func (f *fakeTranslator) Translate(ctx context.Context, text, targetLang string) (string, error) {
	f.calls++
	f.lastText = text
	f.lastLang = targetLang
	return f.out, f.err
}

// fakeMaterialLister sta al posto del provider per la proposta dei
// materiali, come fakeTranslator per le traduzioni.
type fakeMaterialLister struct {
	out          []ai.SuggestedMaterial
	err          error
	calls        int
	lastGame     string
	lastPassages []string
}

func (f *fakeMaterialLister) ListMaterials(ctx context.Context, gameName string, passages []string) ([]ai.SuggestedMaterial, error) {
	f.calls++
	f.lastGame = gameName
	f.lastPassages = passages
	return f.out, f.err
}
