package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/netsafe"
	"github.com/stretchr/testify/require"
)

func robotsServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		require.Equal(t, netsafe.UserAgent, r.Header.Get("User-Agent"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv, &hits
}

func newTestRobots() *Robots {
	return NewRobots(netsafe.NewClient(netsafe.Options{Timeout: 2 * time.Second, AllowPrivate: true}))
}

func TestRobots_FollowsRules(t *testing.T) {
	srv, _ := robotsServer(t, http.StatusOK, `
User-agent: *
Disallow: /private/

User-agent: Linktheca
Disallow: /no-linktheca/
`)
	r := newTestRobots()
	ctx := context.Background()

	for path, want := range map[string]bool{
		"/posts/1":         true,
		"/private/x":       true, // the Linktheca group replaces the * group
		"/no-linktheca/x":  false,
		"/no-linktheca":    true,
		"/posts/?q=1#frag": true,
	} {
		ok, err := r.Allowed(ctx, srv.URL+path)
		require.NoError(t, err)
		require.Equal(t, want, ok, path)
	}
}

func TestRobots_WildcardGroupApplies(t *testing.T) {
	srv, _ := robotsServer(t, http.StatusOK, "User-agent: *\nDisallow: /\n")

	ok, err := newTestRobots().Allowed(context.Background(), srv.URL+"/post")

	require.NoError(t, err)
	require.False(t, ok)
}

func TestRobots_MissingFileAllowsEverything(t *testing.T) {
	srv, _ := robotsServer(t, http.StatusNotFound, "")

	ok, err := newTestRobots().Allowed(context.Background(), srv.URL+"/post")

	require.NoError(t, err)
	require.True(t, ok)
}

func TestRobots_ServerErrorIsRetryable(t *testing.T) {
	srv, hits := robotsServer(t, http.StatusServiceUnavailable, "")
	r := newTestRobots()

	_, err := r.Allowed(context.Background(), srv.URL+"/post")
	require.Error(t, err)

	_, err = r.Allowed(context.Background(), srv.URL+"/post")
	require.Error(t, err)
	require.Equal(t, int32(2), hits.Load(), "a failed fetch is not cached")
}

func TestRobots_CachesPerSiteForADay(t *testing.T) {
	srv, hits := robotsServer(t, http.StatusOK, "User-agent: *\nDisallow: /private/\n")
	r := newTestRobots()
	clock := &fakeClock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	r.now = clock.now
	ctx := context.Background()

	for _, path := range []string{"/a", "/b", "/private/c"} {
		_, err := r.Allowed(ctx, srv.URL+path)
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), hits.Load())

	clock.advance(25 * time.Hour)
	_, err := r.Allowed(ctx, srv.URL+"/a")
	require.NoError(t, err)
	require.Equal(t, int32(2), hits.Load())
}

func TestRobots_RejectsNonHTTPURLs(t *testing.T) {
	r := newTestRobots()

	for _, u := range []string{"ftp://x.example/a", "not a url", "/relative"} {
		ok, err := r.Allowed(context.Background(), u)
		require.NoError(t, err)
		require.False(t, ok, u)
	}
}
