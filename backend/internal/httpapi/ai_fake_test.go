package httpapi_test

import "context"

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
