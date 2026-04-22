package library_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/library"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	contents   map[string]*library.ArticleContent
	items      map[int64]*library.Item
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
	if c, ok := m.contents[p.URL]; ok {
		return c, nil
	}
	m.nextCID++
	c := &library.ArticleContent{
		ID:    m.nextCID,
		URL:   p.URL,
		Title: p.Title,
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
		URL:   url,
		Title: "Extracted: " + url,
		Text:  "Some extracted text for " + url,
		HTML:  "<p>Some extracted text for " + url + "</p>",
	}, nil
}

// --- tests ---

func TestServiceSaveURL(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/article")
	require.NoError(t, err)
	require.Equal(t, int64(1), item.UserID)
	require.Equal(t, "https://example.com/article", item.URL)
	require.Equal(t, "unread", item.State)
}

func TestServiceSaveURLDuplicate(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	_, err := svc.SaveURL(context.Background(), 1, "https://example.com/dup")
	require.NoError(t, err)

	_, err = svc.SaveURL(context.Background(), 1, "https://example.com/dup")
	require.ErrorIs(t, err, library.ErrAlreadySaved)
}

func TestServiceSaveURLExtractionFailure(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	ext.err = errors.New("network error")
	svc := library.NewService(store, ext)

	// Even if extraction fails, we still save the item with whatever we got (URL-only record with fetch_error)
	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/broken")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/broken", item.URL)
}

// compile-time interface check
var _ library.StoreAPI = (*mockStore)(nil)
