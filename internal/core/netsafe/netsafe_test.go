package netsafe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckPublic(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"127.8.9.10",
		"::1",
		"10.1.2.3",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254", // cloud metadata
		"fe80::1",
		"fc00::1",
		"fd12:3456::1",
		"0.0.0.0",
		"0.1.2.3",
		"::",
		"100.64.0.1",
		"100.100.100.100",
		"224.0.0.1",
		"ff02::1",
		"255.255.255.255",
		"240.0.0.1",
		"198.18.0.1",
		"192.0.0.8",
		"::ffff:127.0.0.1",   // IPv4-mapped loopback
		"::ffff:10.0.0.1",    // IPv4-mapped private
		"::ffff:169.254.1.1", // IPv4-mapped link-local
		"fec0::1",
	}
	for _, s := range blocked {
		t.Run("blocks "+s, func(t *testing.T) {
			err := CheckPublic(netip.AddrPortFrom(netip.MustParseAddr(s), 80))
			require.ErrorIs(t, err, ErrBlockedAddress)
		})
	}

	allowed := []string{
		"1.1.1.1",
		"93.184.215.14",
		"100.63.255.255", // just below CGNAT
		"100.128.0.0",    // just above CGNAT
		"172.32.0.1",     // just above 172.16/12
		"2606:4700:4700::1111",
		"::ffff:8.8.8.8",
	}
	for _, s := range allowed {
		t.Run("allows "+s, func(t *testing.T) {
			require.NoError(t, CheckPublic(netip.AddrPortFrom(netip.MustParseAddr(s), 443)))
		})
	}
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func get(t *testing.T, c *http.Client, target string) error {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	require.NoError(t, err)

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

func TestNewClient_RefusesLoopback(t *testing.T) {
	srv := okServer(t)

	err := get(t, NewClient(Options{Timeout: 2 * time.Second}), srv.URL)

	require.ErrorIs(t, err, ErrBlockedAddress)
}

// A hostname is no way around the check: it runs on the resolved address.
func TestNewClient_RefusesNameResolvingToLoopback(t *testing.T) {
	srv := okServer(t)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	err = get(t, NewClient(Options{Timeout: 2 * time.Second}), "http://localhost:"+u.Port())

	require.ErrorIs(t, err, ErrBlockedAddress)
}

func TestNewClient_AllowPrivate(t *testing.T) {
	srv := okServer(t)

	err := get(t, NewClient(Options{Timeout: 2 * time.Second, AllowPrivate: true}), srv.URL)

	require.NoError(t, err)
}

// Both test servers live on loopback, so a check that tells them apart by
// port stands in for "the first hop is public, the redirect target is not".
func TestClient_ChecksEveryRedirectHop(t *testing.T) {
	internal := okServer(t)
	internalPort := portOf(t, internal.URL)

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/admin", http.StatusFound)
	}))
	t.Cleanup(public.Close)

	check := func(ap netip.AddrPort) error {
		if ap.Port() == internalPort {
			return fmt.Errorf("%w: %s", ErrBlockedAddress, ap)
		}
		return nil
	}
	c := newClient(Options{Timeout: 2 * time.Second}, check, nil)

	err := get(t, c, public.URL)

	require.ErrorIs(t, err, ErrBlockedAddress)
}

func TestClient_CapsRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Options{Timeout: 2 * time.Second, AllowPrivate: true, MaxRedirects: 3})

	err := get(t, c, srv.URL)

	require.ErrorIs(t, err, ErrTooManyRedirects)
	require.Contains(t, err.Error(), "stopped after 3")
	require.False(t, errors.Is(err, ErrBlockedAddress))
}

func portOf(t *testing.T, rawURL string) uint16 {
	t.Helper()

	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	p, err := strconv.ParseUint(u.Port(), 10, 16)
	require.NoError(t, err)

	return uint16(p)
}
