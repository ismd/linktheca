package library_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/library"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestIntegrationUpsertContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	// First insert
	c1, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:             "https://example.com/article-1",
		Title:           ptr("Test Article"),
		Text:            ptr("Some text content here."),
		HTML:            ptr("<p>Some text content here.</p>"),
		ReadingTimeSecs: intPtr(60),
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/article-1", c1.URL)
	require.NotZero(t, c1.ID)

	// Second insert with same URL — returns existing row without updating
	c2, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/article-1",
		Title: ptr("Updated Title"),
	})
	require.NoError(t, err)
	require.Equal(t, c1.ID, c2.ID, "upsert must return the same row for the same URL")
}

func TestIntegrationUpsertContentWithFetchError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	c, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:        "https://example.com/broken",
		FetchError: ptr("connection refused"),
	})
	require.NoError(t, err)
	require.NotZero(t, c.ID)
	require.NotNil(t, c.FetchError)
	require.Equal(t, "connection refused", *c.FetchError)
}

func TestIntegrationCreateItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/save-me",
		Title: ptr("Saved Article"),
	})
	require.NoError(t, err)

	item, err := store.CreateItem(ctx, userID, content.ID)
	require.NoError(t, err)
	require.Equal(t, userID, item.UserID)
	require.Equal(t, content.ID, item.ContentID)
	require.Equal(t, "unread", item.State)
	require.False(t, item.IsFavorite)
}

func TestIntegrationCreateItemDuplicate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/dup",
		Title: ptr("Dup Article"),
	})
	require.NoError(t, err)

	_, err = store.CreateItem(ctx, userID, content.ID)
	require.NoError(t, err)

	_, err = store.CreateItem(ctx, userID, content.ID)
	require.ErrorIs(t, err, library.ErrAlreadySaved)
}

func createTestUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		"test@example.com", "fakehash", "Test User",
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func ptr(s string) *string { return &s }
func intPtr(n int) *int    { return &n }
