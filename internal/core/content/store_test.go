package content_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func newContentStore(t *testing.T) *content.Store {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	return content.NewStore(testdb.New(t))
}

func TestStore_UpsertAndGet(t *testing.T) {
	s := newContentStore(t)
	ctx := context.Background()

	_, err := s.GetByURL(ctx, "https://a.example/post")
	require.ErrorIs(t, err, content.ErrNotFound)

	rec, err := s.Upsert(ctx, content.UpsertParams{
		URL: "https://a.example/post", Title: ptr("Title"), Text: ptr("Body"),
	})
	require.NoError(t, err)

	got, err := s.GetByURL(ctx, "https://a.example/post")
	require.NoError(t, err)
	require.Equal(t, rec.ID, got.ID)
	require.Equal(t, "Title", *got.Title)
	require.Equal(t, "Body", *got.Text)
}

// Radar stores a page without images; saving it to the Library later fills
// them in and refreshes the text, keeping what the second fetch did not find.
func TestStore_SecondFetchFillsIn(t *testing.T) {
	s := newContentStore(t)
	ctx := context.Background()

	first, err := s.Upsert(ctx, content.UpsertParams{
		URL: "https://a.example/post", Title: ptr("Title"), Byline: ptr("Ann"), Text: ptr("Old body"),
	})
	require.NoError(t, err)

	second, err := s.Upsert(ctx, content.UpsertParams{
		URL: "https://a.example/post", Text: ptr("New body"), Image: ptr("img.png"),
	})
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "Title", *second.Title)
	require.Equal(t, "Ann", *second.Byline)
	require.Equal(t, "New body", *second.Text)
	require.Equal(t, "img.png", *second.Image)
	require.False(t, second.FetchedAt.Before(first.FetchedAt))
}

func TestStore_FailedFetchKeepsExistingContent(t *testing.T) {
	s := newContentStore(t)
	ctx := context.Background()

	good, err := s.Upsert(ctx, content.UpsertParams{URL: "https://a.example/post", Text: ptr("Body")})
	require.NoError(t, err)

	after, err := s.Upsert(ctx, content.UpsertParams{URL: "https://a.example/post", FetchError: ptr("fetch: status 503")})
	require.NoError(t, err)

	require.Equal(t, "Body", *after.Text)
	require.Nil(t, after.FetchError)
	require.Equal(t, good.FetchedAt, after.FetchedAt)
}

func TestStore_SuccessClearsEarlierError(t *testing.T) {
	s := newContentStore(t)
	ctx := context.Background()

	failed, err := s.Upsert(ctx, content.UpsertParams{URL: "https://a.example/post", FetchError: ptr("fetch: status 503")})
	require.NoError(t, err)
	require.Equal(t, "fetch: status 503", *failed.FetchError)

	again, err := s.Upsert(ctx, content.UpsertParams{URL: "https://a.example/post", FetchError: ptr("fetch: status 404")})
	require.NoError(t, err)
	require.Equal(t, "fetch: status 404", *again.FetchError, "the latest failure is reported")

	time.Sleep(10 * time.Millisecond)
	ok, err := s.Upsert(ctx, content.UpsertParams{URL: "https://a.example/post", Text: ptr("Body")})
	require.NoError(t, err)
	require.Nil(t, ok.FetchError)
	require.Equal(t, "Body", *ok.Text)
	require.True(t, ok.FetchedAt.After(failed.FetchedAt))
}

func TestParamsFromArticle(t *testing.T) {
	published := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

	p := content.ParamsFromArticle(&content.Article{
		URL: "https://a.example/post", Title: "Title", Text: "Body", Favicon: "https://a.example/f.ico",
		PublishedTime: published, ReadingTimeSecs: 60,
	})

	require.Equal(t, "https://a.example/post", p.URL)
	require.Equal(t, "Title", *p.Title)
	require.Equal(t, "Body", *p.Text)
	require.Equal(t, "https://a.example/f.ico", *p.FaviconURL)
	require.Equal(t, published, *p.PublishedTime)
	require.Equal(t, 60, *p.ReadingTimeSecs)
	require.Nil(t, p.Byline, "empty fields stay nil so they do not overwrite stored ones")
	require.Nil(t, p.HTML)
	require.Nil(t, p.ModifiedTime)
	require.Nil(t, p.Image)
}
