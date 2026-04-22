package config_test

import (
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/config"
	"github.com/stretchr/testify/require"
)

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LINKTHECA_HTTP_ADDR", ":9999")
	t.Setenv("LINKTHECA_DB_DSN", "postgres://u:p@h:5432/db")
	t.Setenv("LINKTHECA_JWT_SECRET", "testsecret-with-enough-bytes-to-pass-checks")
	t.Setenv("LINKTHECA_JWT_ACCESS_TTL", "5m")
	t.Setenv("LINKTHECA_JWT_REFRESH_TTL", "24h")
	t.Setenv("LINKTHECA_REGISTRATION_ENABLED", "false")
	t.Setenv("LINKTHECA_LOG_LEVEL", "debug")
	t.Setenv("LINKTHECA_LOG_FORMAT", "json")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, ":9999", cfg.HTTPAddr)
	require.Equal(t, "postgres://u:p@h:5432/db", cfg.DBDSN)
	require.Equal(t, "testsecret-with-enough-bytes-to-pass-checks", cfg.JWTSecret)
	require.Equal(t, 5*time.Minute, cfg.JWTAccessTTL)
	require.Equal(t, 24*time.Hour, cfg.JWTRefreshTTL)
	require.False(t, cfg.RegistrationEnabled)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, "json", cfg.LogFormat)
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	// Clear required var and make sure Load fails
	t.Setenv("LINKTHECA_JWT_SECRET", "")
	t.Setenv("LINKTHECA_DB_DSN", "postgres://localhost/x")

	_, err := config.Load()
	require.Error(t, err)
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LINKTHECA_JWT_SECRET", "a-strong-secret-at-least-32-bytes-long")
	t.Setenv("LINKTHECA_DB_DSN", "postgres://localhost/x")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, 15*time.Minute, cfg.JWTAccessTTL)
	require.Equal(t, 720*time.Hour, cfg.JWTRefreshTTL)
	require.True(t, cfg.RegistrationEnabled)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, "text", cfg.LogFormat)
}

func TestLoad_RadarDefaults(t *testing.T) {
	t.Setenv("LINKTHECA_DB_DSN", "postgres://x:y@localhost:5432/z?sslmode=disable")
	t.Setenv("LINKTHECA_JWT_SECRET", "dev-only-secret-that-is-at-least-32-bytes-long")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, "http://localhost:8081", cfg.TEIURL)
	require.Equal(t, 30*time.Second, cfg.TEITimeout)
	require.Equal(t, 1024, cfg.EmbeddingDim)
	require.True(t, cfg.RadarEnabled)
	require.Equal(t, 5*time.Minute, cfg.RadarSchedulerInterval)
	require.Equal(t, 5, cfg.RadarMaxWorkers)
}
