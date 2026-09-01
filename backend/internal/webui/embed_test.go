package webui_test

import (
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/webui"
)

// TestHandler_SPAFallbackDoesNotMutateOriginalRequest guards against a
// regression where constructing the index.html fallback request aliased the
// original request's *url.URL (via a shallow struct copy), causing writes to
// the fallback request's URL.Path to also mutate the caller's r.URL.Path as
// a side effect. Any middleware that inspects r.URL.Path after ServeHTTP
// returns (access logging, tracing, etc.) would then see the wrong path.
func TestHandler_SPAFallbackDoesNotMutateOriginalRequest(t *testing.T) {
	handler, err := webui.Handler()
	if err != nil {
		t.Fatalf("webui.Handler: %v", err)
	}

	const clientRoute = "/users"
	req := httptest.NewRequest("GET", clientRoute, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if req.URL.Path != clientRoute {
		t.Fatalf("original request URL.Path mutated: got %q, want %q", req.URL.Path, clientRoute)
	}
	if rec.Code != 200 {
		t.Fatalf("unexpected status code: got %d, want 200", rec.Code)
	}
}
