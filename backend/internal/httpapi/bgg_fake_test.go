package httpapi_test

import (
	"context"

	"boardgames-manager/internal/bgg"
)

type fakeBGGClient struct {
	searchResults []bgg.SearchResult
	searchErr     error
	thing         bgg.ThingDetail
	thingErr      error
}

func (f *fakeBGGClient) Search(ctx context.Context, token, query string) ([]bgg.SearchResult, error) {
	return f.searchResults, f.searchErr
}

func (f *fakeBGGClient) GetThing(ctx context.Context, token, id string) (bgg.ThingDetail, error) {
	return f.thing, f.thingErr
}
