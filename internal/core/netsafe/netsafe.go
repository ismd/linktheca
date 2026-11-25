// Package netsafe builds HTTP clients for fetching URLs that come from
// outside: links in feeds, pages users save, images those pages declare.
// Any of them can point at the server's own network, such as the database,
// TEI, a router admin page or a cloud metadata endpoint at 169.254.169.254,
// and the response would end up in front of the user. The clients built here
// refuse to connect to such addresses.
package netsafe

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned, wrapped, when a request would connect to an
// address outside the public internet.
var ErrBlockedAddress = errors.New("destination address is not public")

// ErrTooManyRedirects is returned, wrapped, when a request is still being
// redirected after Options.MaxRedirects hops.
var ErrTooManyRedirects = errors.New("too many redirects")

// Options configure a client.
type Options struct {
	// Timeout bounds the whole request, redirects and body included.
	Timeout time.Duration

	// MaxRedirects caps how many redirects are followed. Zero means
	// DefaultMaxRedirects.
	MaxRedirects int

	// AllowPrivate turns the address checks off, for instances that read
	// feeds or save pages from their own local network.
	AllowPrivate bool
}

// DefaultMaxRedirects is the redirect cap used when Options leave it unset.
const DefaultMaxRedirects = 5

// NewClient returns a client that only connects to public addresses, unless
// opts.AllowPrivate is set.
//
// The check runs when the connection is dialed, after DNS has resolved the
// host, so it holds for every redirect hop and a name that resolves to a
// private address is caught just like a literal IP. Checking the hostname
// up front instead would be beaten by DNS rebinding.
//
// Proxies from the environment are ignored unless private addresses are
// allowed: through a proxy we would dial the proxy, never see where the
// request really goes, and the check would mean nothing.
func NewClient(opts Options) *http.Client {
	if opts.AllowPrivate {
		return newClient(opts, nil, http.ProxyFromEnvironment)
	}

	return newClient(opts, CheckPublic, nil)
}

// newClient wires a dial-time address check into a client. check may be nil.
func newClient(
	opts Options,
	check func(netip.AddrPort) error,
	proxy func(*http.Request) (*url.URL, error),
) *http.Client {
	maxRedirects := opts.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = DefaultMaxRedirects
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	if check != nil {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return fmt.Errorf("%w: unparsable address %q", ErrBlockedAddress, address)
			}

			return check(ap)
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.Proxy = proxy

	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("%w: stopped after %d", ErrTooManyRedirects, maxRedirects)
			}

			return nil
		},
	}
}

// Ranges the netip predicates below do not cover.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this network"; 0.0.0.0 reaches localhost on Linux
	netip.MustParsePrefix("100.64.0.0/10"),  // carrier-grade NAT, also Tailscale
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, includes broadcast
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64
	netip.MustParsePrefix("fec0::/10"),      // deprecated site-local
}

// CheckPublic returns an error wrapping ErrBlockedAddress unless ap is a
// public unicast address.
func CheckPublic(ap netip.AddrPort) error {
	addr := ap.Addr().Unmap()

	if !addr.IsValid() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
	}

	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
		}
	}

	return nil
}
