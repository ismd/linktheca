package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTEIClient_Embed_Success(t *testing.T) {
	var got struct {
		Inputs string `json:"inputs"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/embed", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		require.Equal(t, "hello", got.Inputs)
		_, _ = w.Write([]byte(`[[0.1, 0.2, 0.3]]`))
	}))
	defer srv.Close()

	c := NewTEIClient(srv.URL, 2*time.Second)
	v, err := c.Embed(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, []float32{0.1, 0.2, 0.3}, v)
}

func TestTEIClient_Embed_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewTEIClient(srv.URL, 2*time.Second)
	_, err := c.Embed(context.Background(), "hello")
	require.Error(t, err)
}

func TestTEIClient_Embed_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewTEIClient(srv.URL, 2*time.Second)
	_, err := c.Embed(context.Background(), "hello")
	require.Error(t, err)
}
