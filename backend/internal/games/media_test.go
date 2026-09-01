package games_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/games"
)

func newTestLanguage(t *testing.T, store *games.Store) games.GameLanguage {
	t.Helper()
	ctx := context.Background()
	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	lang, err := store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "it", IsBaseLanguage: true, Name: "Catan",
	})
	if err != nil {
		t.Fatalf("create language: %v", err)
	}
	return lang
}

func TestCreateAndListMedia(t *testing.T) {
	store := newTestStore(t)
	lang := newTestLanguage(t, store)
	ctx := context.Background()

	title := "Manuale ufficiale"
	media, err := store.CreateMedia(ctx, games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeFile, URLOrPath: "abc123.pdf", Title: &title,
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}
	if media.Type != games.MediaTypeFile {
		t.Fatalf("unexpected type: %q", media.Type)
	}

	list, err := store.ListMedia(ctx, lang.ID)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(list) != 1 || list[0].URLOrPath != "abc123.pdf" {
		t.Fatalf("unexpected media list: %+v", list)
	}
}

func TestDeleteMedia_RemovesIt(t *testing.T) {
	store := newTestStore(t)
	lang := newTestLanguage(t, store)
	ctx := context.Background()

	media, err := store.CreateMedia(ctx, games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeLink, URLOrPath: "https://example.com/rules",
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}

	if err := store.DeleteMedia(ctx, media.ID); err != nil {
		t.Fatalf("delete media: %v", err)
	}

	list, err := store.ListMedia(ctx, lang.ID)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no media after delete, got %+v", list)
	}
}

func TestDeleteMedia_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.DeleteMedia(context.Background(), 999)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
