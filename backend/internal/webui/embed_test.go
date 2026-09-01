package webui_test

import (
	"net/http/httptest"
	"strings"
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
	// A 200 alone used to be satisfiable by a directory listing served out of
	// an unbuilt dist/, so assert we really got the SPA shell back.
	if body := rec.Body.String(); !strings.Contains(body, `id="app"`) {
		t.Fatalf("expected the SPA index.html for a client route, got: %s", body)
	}
}

// TestHandler_SucceedsWithEmbeddedBuildOutput is the positive half of the
// "frontend really got built" contract; see handlerFor's own tests for the
// missing-index.html case, which needs an alternate filesystem to exercise.
func TestHandler_SucceedsWithEmbeddedBuildOutput(t *testing.T) {
	if _, err := webui.Handler(); err != nil {
		t.Fatalf("Handler() failed — is the frontend built? %v", err)
	}
}
