package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllow_BlocksAfterMaxWithinWindow(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if !rl.allow("1.2.3.4", now) {
		t.Fatal("expected first attempt to be allowed")
	}
	if !rl.allow("1.2.3.4", now) {
		t.Fatal("expected second attempt to be allowed")
	}
	if rl.allow("1.2.3.4", now) {
		t.Fatal("expected third attempt within the window to be blocked")
	}
}

func TestRateLimiterAllow_ResetsAfterWindow(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if !rl.allow("1.2.3.4", start) {
		t.Fatal("expected first attempt to be allowed")
	}
	if rl.allow("1.2.3.4", start.Add(30*time.Second)) {
		t.Fatal("expected attempt still within the window to be blocked")
	}
	if !rl.allow("1.2.3.4", start.Add(90*time.Second)) {
		t.Fatal("expected attempt after the window to be allowed again")
	}
}

func TestRateLimiterAllow_TracksKeysIndependently(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if !rl.allow("1.2.3.4", now) {
		t.Fatal("expected first IP's first attempt to be allowed")
	}
	if !rl.allow("5.6.7.8", now) {
		t.Fatal("expected a different IP's first attempt to be allowed regardless of the first IP's count")
	}
}

func TestRateLimiterMiddleware_Returns429WhenExceeded(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	handler := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "9.9.9.9:12345"

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request to pass through, got %d", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec2.Code)
	}
}
