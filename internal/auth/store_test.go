package auth_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/auth"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationUsersStoreCreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	created, err := store.CreateUser(ctx, "alice@example.com", "hashed", "Alice", true)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, "alice@example.com", created.Email)
	require.True(t, created.IsAdmin)
	require.Equal(t, "hashed", created.PasswordHash)

	got, err := store.GetUserByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "hashed", got.PasswordHash)

	gotByID, err := store.GetUserByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Email, gotByID.Email)
}

func TestIntegrationUsersStoreCountUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	n, err := store.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)

	_, err = store.CreateUser(ctx, "a@example.com", "h", "A", false)
	require.NoError(t, err)
	_, err = store.CreateUser(ctx, "b@example.com", "h", "B", false)
	require.NoError(t, err)

	n, err = store.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
}

func TestIntegrationUsersStoreDuplicateEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	_, err := store.CreateUser(ctx, "dup@example.com", "h", "Dup", false)
	require.NoError(t, err)

	_, err = store.CreateUser(ctx, "dup@example.com", "h", "Dup2", false)
	require.ErrorIs(t, err, auth.ErrEmailTaken)
}

func TestIntegrationUsersStoreGetUnknownEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	_, err := store.GetUserByEmail(ctx, "nobody@example.com")
	require.ErrorIs(t, err, auth.ErrNotFound)
}
