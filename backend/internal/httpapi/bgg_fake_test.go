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
	details       map[string]bgg.ThingDetail
	detailsErr    error
	detailsIDs    []string
	detailsCalls  int
}

func (f *fakeBGGClient) Search(ctx context.Context, token, query string) ([]bgg.SearchResult, error) {
	return f.searchResults, f.searchErr
}

func (f *fakeBGGClient) GetThing(ctx context.Context, token, id string) (bgg.ThingDetail, error) {
	return f.thing, f.thingErr
}

func (f *fakeBGGClient) Details(ctx context.Context, token string, ids []string) (map[string]bgg.ThingDetail, error) {
	f.detailsCalls++
	f.detailsIDs = ids
	return f.details, f.detailsErr
}
