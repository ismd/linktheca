//go:build smoke

package embeddings_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestTEI_RealEmbedding(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/huggingface/text-embeddings-inference:cpu-1.9",
		Cmd:          []string{"--model-id", "BAAI/bge-m3", "--port", "8080"},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080/tcp").WithStartupTimeout(10 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	client := embeddings.NewTEIClient("http://"+host+":"+port.Port(), 60*time.Second)

	v, err := client.Embed(ctx, "Linktheca is a read-it-later service")
	require.NoError(t, err)
	require.Len(t, v, 1024, "bge-m3 must produce 1024-dim vectors")

	other, err := client.Embed(ctx, "totally unrelated random sentence about penguins")
	require.NoError(t, err)
	require.NotEqual(t, v, other, "different inputs must produce different vectors")
}
