package auth_test

import (
	"testing"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerifyPassword(t *testing.T) {
	pw := "correct-horse-battery-staple"

	hash, err := auth.HashPassword(pw)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.Contains(t, hash, "$argon2id$")

	ok, err := auth.VerifyPassword(pw, hash)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestVerifyPasswordWrongPassword(t *testing.T) {
	hash, err := auth.HashPassword("right-password")
	require.NoError(t, err)

	ok, err := auth.VerifyPassword("wrong-password", hash)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	ok, err := auth.VerifyPassword("anything", "not-a-valid-hash")
	require.Error(t, err)
	require.False(t, ok)
}
