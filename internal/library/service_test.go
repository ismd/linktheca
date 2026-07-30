package library_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/media"
	"github.com/ismd/linktheca/internal/library"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	contents   map[string]*library.ArticleContent
	items      map[int64]*library.Item
	upserts    []library.UpsertContentParams
	nextCID    int64
	nextItemID int64
}

func newMockStore() *mockStore {
	return &mockStore{
		contents: make(map[string]*library.ArticleContent),
		items:    make(map[int64]*library.Item),
	}
}

func (m *mockStore) UpsertContent(_ context.Context, p library.UpsertContentParams) (*library.ArticleContent, error) {
	m.upserts = append(m.upserts, p)

	if c, ok := m.contents[p.URL]; ok {
		return c, nil
	}
	m.nextCID++
	c := &library.ArticleContent{
		ID:    m.nextCID,
		URL:   p.URL,
		Title: p.Title,
		Image: p.Image,
	}
	m.contents[p.URL] = c

	return c, nil
}

func (m *mockStore) GetContentByURL(_ context.Context, url string) (*library.ArticleContent, error) {
	c, ok := m.contents[url]
	if !ok {
		return nil, library.ErrNotFound
	}

	return c, nil
}

func (m *mockStore) CreateItem(_ context.Context, userID, contentID int64) (*library.Item, error) {
	for _, item := range m.items {
		if item.UserID == userID && item.ContentID == contentID {
			return nil, library.ErrAlreadySaved
		}
	}

	m.nextItemID++
	item := &library.Item{
		ID:        m.nextItemID,
		UserID:    userID,
		ContentID: contentID,
		State:     "unread",
		URL:       m.contentURL(contentID),
		Title:     m.contentTitle(contentID),
	}

	m.items[item.ID] = item
	return item, nil
}

func (m *mockStore) GetItemByID(_ context.Context, userID, itemID int64) (*library.Item, error) {
	item, ok := m.items[itemID]
	if !ok || item.UserID != userID {
		return nil, library.ErrNotFound
	}

	return item, nil
}

func (m *mockStore) ListItems(_ context.Context, p library.ListParams) (*library.ListResult, error) {
	var items []library.Item
	for _, item := range m.items {
		if item.UserID != p.UserID {
			continue
		}
		if p.State != "" && item.State != p.State {
			continue
		}
		items = append(items, *item)
	}

	return &library.ListResult{Items: items, Total: len(items)}, nil
}

func (m *mockStore) UpdateItem(_ context.Context, userID, itemID int64, p library.UpdateParams) (*library.Item, error) {
	item, ok := m.items[itemID]

	if !ok || item.UserID != userID {
		return nil, library.ErrNotFound
	}

	if p.State != nil {
		item.State = *p.State
	}

	if p.IsFavorite != nil {
		item.IsFavorite = *p.IsFavorite
	}

	if p.Note != nil {
		item.Note = p.Note
	}

	return item, nil
}

func (m *mockStore) DeleteItem(_ context.Context, userID, itemID int64) error {
	item, ok := m.items[itemID]
	if !ok || item.UserID != userID {
		return library.ErrNotFound
	}

	delete(m.items, itemID)
	return nil
}

func (m *mockStore) contentURL(contentID int64) string {
	for _, c := range m.contents {
		if c.ID == contentID {
			return c.URL
		}
	}

	return ""
}

func (m *mockStore) contentTitle(contentID int64) *string {
	for _, c := range m.contents {
		if c.ID == contentID {
			return c.Title
		}
	}

	return nil
}

func (m *mockStore) GetItemDetail(_ context.Context, userID, itemID int64) (*library.ItemDetail, error) {
	item, ok := m.items[itemID]
	if !ok || item.UserID != userID {
		return nil, library.ErrNotFound
	}

	return &library.ItemDetail{Item: *item}, nil
}

// --- mock extractor ---

type mockExtractor struct {
	results map[string]*content.Article
	err     error
}

func newMockExtractor() *mockExtractor {
	return &mockExtractor{results: make(map[string]*content.Article)}
}

func (m *mockExtractor) Extract(_ context.Context, url string) (*content.Article, error) {
	if m.err != nil {
		return nil, m.err
	}

	if a, ok := m.results[url]; ok {
		return a, nil
	}

	return &content.Article{
		URL:      url,
		Title:    "Extracted: " + url,
		Text:     "Some extracted text for " + url,
		HTML:     "<p>Some extracted text for " + url + "</p>",
		ImageURL: url + "/og.png",
	}, nil
}

// --- mock fetcher ---

type mockFetcher struct {
	results map[string]*media.Image
	calls   []string
	err     error
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{results: make(map[string]*media.Image)}
}

func (m *mockFetcher) Fetch(_ context.Context, imageURL string) (*media.Image, error) {
	m.calls = append(m.calls, imageURL)

	if m.err != nil {
		return nil, m.err
	}

	if img, ok := m.results[imageURL]; ok {
		return img, nil
	}

	return &media.Image{Filename: "downloaded.png"}, nil
}

// --- tests ---

func TestServiceSaveURL(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/article")
	require.NoError(t, err)
	require.Equal(t, int64(1), item.UserID)
	require.Equal(t, "https://example.com/article", item.URL)
	require.Equal(t, "unread", item.State)
}

func TestServiceSaveURLStoresDownloadedImage(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	ext.results["https://example.com/with-image"] = &content.Article{
		URL:      "https://example.com/with-image",
		Title:    "With image",
		ImageURL: "https://cdn.example.com/preview.png",
	}
	fetch.results["https://cdn.example.com/preview.png"] = &media.Image{Filename: "a1b2c3.png"}

	_, err := svc.SaveURL(context.Background(), 1, "https://example.com/with-image")
	require.NoError(t, err)

	// The og:image URL is what gets downloaded, not the article URL
	require.Equal(t, []string{"https://cdn.example.com/preview.png"}, fetch.calls)

	// The local filename is persisted alongside the original URL
	require.Len(t, store.upserts, 1)
	require.Equal(t, "a1b2c3.png", *store.upserts[0].Image)
	require.Equal(t, "https://cdn.example.com/preview.png", *store.upserts[0].ImageURL)
}

func TestServiceSaveURLWithoutImage(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	ext.results["https://example.com/no-image"] = &content.Article{
		URL:   "https://example.com/no-image",
		Title: "No image",
	}

	// Most pages have no og:image; that must not stop us from saving them
	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/no-image")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/no-image", item.URL)

	require.Empty(t, fetch.calls, "nothing to download without an image URL")
	require.Len(t, store.upserts, 1)
	require.Nil(t, store.upserts[0].Image)
}

func TestServiceSaveURLImageFetchFailure(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	fetch.err = errors.New("fetch: status 404")
	svc := library.NewService(store, ext, fetch)

	ext.results["https://example.com/broken-image"] = &content.Article{
		URL:      "https://example.com/broken-image",
		Title:    "Broken image",
		Text:     "Article body",
		ImageURL: "https://cdn.example.com/gone.png",
	}

	// The preview image is decoration: losing it must not lose the article
	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/broken-image")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/broken-image", item.URL)

	require.Len(t, store.upserts, 1)
	saved := store.upserts[0]
	require.Nil(t, saved.Image)
	require.Equal(t, "Broken image", *saved.Title)
	require.Equal(t, "Article body", *saved.Text)

	// Extraction succeeded, so the record must not be marked as failed
	require.Nil(t, saved.FetchError)
}

func TestServiceSaveURLDuplicate(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	_, err := svc.SaveURL(context.Background(), 1, "https://example.com/dup")
	require.NoError(t, err)

	_, err = svc.SaveURL(context.Background(), 1, "https://example.com/dup")
	require.ErrorIs(t, err, library.ErrAlreadySaved)
}

func TestServiceSaveURLExtractionFailure(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	ext.err = errors.New("network error")
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	// Even if extraction fails, we still save the item with whatever we got (URL-only record with fetch_error)
	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/broken")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/broken", item.URL)

	// Nothing to download when there is no extracted article
	require.Empty(t, fetch.calls)
}

func TestServiceList(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	_, _ = svc.SaveURL(context.Background(), 1, "https://example.com/a")
	_, _ = svc.SaveURL(context.Background(), 1, "https://example.com/b")

	result, err := svc.List(context.Background(), library.ListParams{
		UserID: 1,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.Total)
}

func TestServiceGetByID(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/get")

	got, err := svc.GetByID(context.Background(), 1, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, got.ID)
}

func TestServiceGetByIDNotFound(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	_, err := svc.GetByID(context.Background(), 1, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestServiceUpdate(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/upd")

	state := "read"
	updated, err := svc.Update(context.Background(), 1, item.ID, library.UpdateRequest{State: &state})
	require.NoError(t, err)
	require.Equal(t, "read", updated.State)
}

func TestServiceUpdateInvalidState(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/bad")

	bad := "invalid"
	_, err := svc.Update(context.Background(), 1, item.ID, library.UpdateRequest{State: &bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid state")
}

func TestServiceDelete(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/del")

	err := svc.Delete(context.Background(), 1, item.ID)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), 1, item.ID)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestServiceDeleteNotFound(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	err := svc.Delete(context.Background(), 1, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}

// compile-time interface checks
var (
	_ library.StoreAPI  = (*mockStore)(nil)
	_ content.Extractor = (*mockExtractor)(nil)
	_ media.Fetcher     = (*mockFetcher)(nil)
)
