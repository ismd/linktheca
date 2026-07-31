package library_test

import (
	"context"
	"fmt"
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

func TestIntegrationListItems(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	// Save 3 articles
	for i := 0; i < 3; i++ {
		c, err := store.UpsertContent(ctx, library.UpsertContentParams{
			URL:   fmt.Sprintf("https://example.com/list-%d", i),
			Title: ptr(fmt.Sprintf("Article %d", i)),
		})
		require.NoError(t, err)
		_, err = store.CreateItem(ctx, userID, c.ID)
		require.NoError(t, err)
	}

	result, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 3, result.Total)
	require.Len(t, result.Items, 3)
	// Items come with joined content fields
	require.NotEmpty(t, result.Items[0].URL)
}

func TestIntegrationListItemsIncludesImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	c, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/list-image",
		Title: ptr("With preview"),
		Image: ptr("a1b2c3.png"),
	})
	require.NoError(t, err)
	_, err = store.CreateItem(ctx, userID, c.ID)
	require.NoError(t, err)

	// Library cards render the downloaded preview, so the list must carry it
	result, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.NotNil(t, result.Items[0].Image)
	require.Equal(t, "a1b2c3.png", *result.Items[0].Image)
}

func TestIntegrationListItemsByState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	c1, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/s1", Title: ptr("S1")})
	c2, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/s2", Title: ptr("S2")})

	item1, _ := store.CreateItem(ctx, userID, c1.ID)
	_, _ = store.CreateItem(ctx, userID, c2.ID)

	// Mark first as read
	state := "read"
	_, err := store.UpdateItem(ctx, userID, item1.ID, library.UpdateParams{State: &state})
	require.NoError(t, err)

	// Filter by state=unread
	result, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		State:  "unread",
		Limit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Total)
}

func TestIntegrationListItemsPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	for i := 0; i < 5; i++ {
		c, _ := store.UpsertContent(ctx, library.UpsertContentParams{
			URL:   fmt.Sprintf("https://example.com/page-%d", i),
			Title: ptr(fmt.Sprintf("Page %d", i)),
		})
		_, _ = store.CreateItem(ctx, userID, c.ID)
	}

	result, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		Limit:  2,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 5, result.Total)
	require.Len(t, result.Items, 2)

	result2, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		Limit:  2,
		Offset: 2,
	})
	require.NoError(t, err)
	require.Equal(t, 5, result2.Total)
	require.Len(t, result2.Items, 2)
}

func TestIntegrationGetItemByID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/get-me",
		Title: ptr("Get Me"),
		Text:  ptr("Full article text here."),
		HTML:  ptr("<p>Full article text here.</p>"),
	})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	got, err := store.GetItemByID(ctx, userID, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, got.ID)
	require.Equal(t, "https://example.com/get-me", got.URL)
	require.NotNil(t, got.Title)
	require.Equal(t, "Get Me", *got.Title)
}

func TestIntegrationGetItemByIDIncludesImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/get-image",
		Title: ptr("With preview"),
		Image: ptr("a1b2c3.png"),
	})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	got, err := store.GetItemByID(ctx, userID, item.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Image)
	require.Equal(t, "a1b2c3.png", *got.Image)
}

func TestIntegrationGetItemByIDNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)

	_, err := store.GetItemByID(context.Background(), 999, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestIntegrationGetItemByIDAnotherUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	user1 := createTestUser(t, pool)
	user2 := createTestUserWithEmail(t, pool, "user2@example.com")

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/private", Title: ptr("Private")})
	item, _ := store.CreateItem(ctx, user1, content.ID)

	// user2 should not see user1's item
	_, err := store.GetItemByID(ctx, user2, item.ID)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestIntegrationUpdateItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/update-me", Title: ptr("Update Me")})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	state := "read"
	fav := true
	updated, err := store.UpdateItem(ctx, userID, item.ID, library.UpdateParams{
		State:      &state,
		IsFavorite: &fav,
	})
	require.NoError(t, err)
	require.Equal(t, "read", updated.State)
	require.True(t, updated.IsFavorite)
	require.NotNil(t, updated.ReadAt, "read_at should be set when state becomes read")
}

func TestIntegrationUpdateItemPartial(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/partial", Title: ptr("Partial")})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	// Only update favorite and leave state
	fav := true
	updated, err := store.UpdateItem(ctx, userID, item.ID, library.UpdateParams{
		IsFavorite: &fav,
	})
	require.NoError(t, err)
	require.True(t, updated.IsFavorite)
	require.Equal(t, "unread", updated.State, "state must not change")
}

func TestIntegrationDeleteItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/delete-me", Title: ptr("Delete Me")})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	err := store.DeleteItem(ctx, userID, item.ID)
	require.NoError(t, err)

	_, err = store.GetItemByID(ctx, userID, item.ID)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestIntegrationDeleteItemNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)

	err := store.DeleteItem(context.Background(), 999, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func createTestUserWithEmail(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		email, "fakehash", "Test User",
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestIntegrationGetItemDetail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/detail",
		Title: ptr("Detail Article"),
		Text:  ptr("Full article text for reader view."),
		HTML:  ptr("<p>Full article text for reader view.</p>"),
	})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	detail, err := store.GetItemDetail(ctx, userID, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, detail.ID)
	require.Equal(t, "https://example.com/detail", detail.Content.URL)
	require.NotNil(t, detail.Content.Text)
	require.Equal(t, "Full article text for reader view.", *detail.Content.Text)
	require.NotNil(t, detail.Content.HTML)
}

func TestIntegrationGetItemDetailIncludesImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:      "https://example.com/detail-image",
		Title:    ptr("With preview"),
		ImageURL: ptr("https://cdn.example.com/preview.png"),
		Image:    ptr("a1b2c3.png"),
	})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	detail, err := store.GetItemDetail(ctx, userID, item.ID)
	require.NoError(t, err)

	// The reader reads the local file from the content block, next to the source URL
	require.NotNil(t, detail.Content.Image)
	require.Equal(t, "a1b2c3.png", *detail.Content.Image)
	require.NotNil(t, detail.Content.ImageURL)
	require.Equal(t, "https://cdn.example.com/preview.png", *detail.Content.ImageURL)

	// The embedded item carries it too, like every other joined field
	require.NotNil(t, detail.Image)
	require.Equal(t, "a1b2c3.png", *detail.Image)
}
