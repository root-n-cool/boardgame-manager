package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/httpapi"
)

// imageUploadBody builds a multipart body holding a real 2x2 PNG under the
// "file" field, which is what both the cover and the event image endpoints
// expect. Returns the body and the matching Content-Type.
func imageUploadBody(t *testing.T, filename string) (*bytes.Buffer, string) {
	t.Helper()
	var imgBuf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	if err := png.Encode(&imgBuf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return multipartFileBody(t, filename, imgBuf.Bytes())
}

func multipartFileBody(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	writer.Close()
	return &buf, writer.FormDataContentType()
}

func TestUploadEventImage_ValidImageShowsUpInTheEventList(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2099-01-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	body, contentType := imageUploadBody(t, "evento.png")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/image", event.ID), body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var uploaded struct {
		ImagePath *string `json:"imagePath"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if uploaded.ImagePath == nil || *uploaded.ImagePath == "" {
		t.Fatalf("expected imagePath to be set, got %v", uploaded.ImagePath)
	}

	listed := listEvents(t, router, "/api/events", nil)
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 upcoming event, got %+v", listed.Items)
	}
	if listed.Items[0].ImagePath == nil || *listed.Items[0].ImagePath != *uploaded.ImagePath {
		t.Fatalf("expected the list to carry the uploaded image, got %v", listed.Items[0].ImagePath)
	}
}

func TestUploadEventImage_RejectsUnsupportedType(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2099-01-01",
		StartTime: "20:00",
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	body, contentType := multipartFileBody(t, "evento.pdf", []byte("%PDF-1.4 not an image"))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/image", event.ID), body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadEventImage_UnknownEvent(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	body, contentType := imageUploadBody(t, "evento.png")
	req := httptest.NewRequest(http.MethodPost, "/api/events/999/image", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadEventImage_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2099-01-01",
		StartTime: "20:00",
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	body, contentType := imageUploadBody(t, "evento.png")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/image", event.ID), body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}
