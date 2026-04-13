package auth_test

import (
	"testing"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

func TestGenerateRefreshToken(t *testing.T) {
	a, err := auth.GenerateRefreshToken()
	require.NoError(t, err)
	require.NotEmpty(t, a)

	b, err := auth.GenerateRefreshToken()
	require.NoError(t, err)
	require.NotEqual(t, a, b, "tokens must be unique per call")
}

func TestHashRefreshTokenIsDeterministic(t *testing.T) {
	token := "sample-refresh-token-value"
	h1 := auth.HashRefreshToken(token)
	h2 := auth.HashRefreshToken(token)
	require.Equal(t, h1, h2)
	require.NotEqual(t, token, h1, "hash must differ from plaintext")
}

func TestHashRefreshTokenDifferentInputs(t *testing.T) {
	h1 := auth.HashRefreshToken("token-a")
	h2 := auth.HashRefreshToken("token-b")
	require.NotEqual(t, h1, h2)
}
