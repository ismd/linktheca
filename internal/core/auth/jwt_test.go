package auth_test

import (
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-at-least-32-bytes-long-for-hmac"

func TestIssueAndParseAccessToken(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)

	token, err := issuer.Issue(42, true)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := issuer.Parse(token)
	require.NoError(t, err)
	require.Equal(t, "42", claims.Subject)
	require.True(t, claims.IsAdmin)
}

func TestParseExpiredTokenFails(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, -1*time.Second) // already expired
	token, err := issuer.Issue(1, false)
	require.NoError(t, err)

	_, err = issuer.Parse(token)
	require.Error(t, err)
}

func TestParseWithWrongSecretFails(t *testing.T) {
	issuerA := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	issuerB := auth.NewJWTIssuer("a-completely-different-secret-32bytes", 15*time.Minute)

	token, err := issuerA.Issue(1, false)
	require.NoError(t, err)

	_, err = issuerB.Parse(token)
	require.Error(t, err)
}
