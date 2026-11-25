package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ismd/linktheca/internal/core/netsafe"
	"github.com/jimsmart/grobotstxt"
)

// robotsAgent is the product token robots.txt groups are matched against.
const robotsAgent = "Linktheca"

// maxRobotsBytes is how much of a robots.txt is read. RFC 9309 asks crawlers
// to parse at least 500 KiB.
const maxRobotsBytes = 500 << 10

// Robots answers whether Radar may fetch a page, following the site's
// robots.txt. Each site's file is fetched once and kept for a day.
type Robots struct {
	client *http.Client
	ttl    time.Duration
	now    func() time.Time

	mu    sync.Mutex
	sites map[string]robotsRules
}

type robotsRules struct {
	body     string // robots.txt contents; empty allows everything
	disallow bool   // the whole site is off limits
	expires  time.Time
}

// NewRobots returns a Robots fetching robots.txt files with client.
func NewRobots(client *http.Client) *Robots {
	return &Robots{
		client: client,
		ttl:    24 * time.Hour,
		now:    time.Now,
		sites:  map[string]robotsRules{},
	}
}

// Allowed reports whether rawURL may be fetched. An error means the site's
// robots.txt could not be read right now, a 5xx or a network failure. RFC 9309
// treats that as "disallow everything", so the caller should not fetch the
// page now but may try again later.
func (r *Robots) Allowed(ctx context.Context, rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false, nil
	}

	site := u.Scheme + "://" + u.Host

	rules, ok := r.cached(site)
	if !ok {
		rules, err = r.fetch(ctx, site)
		if err != nil {
			return false, err
		}
		r.store(site, rules)
	}

	if rules.disallow {
		return false, nil
	}
	if rules.body == "" {
		return true, nil
	}

	return grobotstxt.AgentAllowed(rules.body, robotsAgent, rawURL), nil
}

func (r *Robots) cached(site string) (robotsRules, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rules, ok := r.sites[site]
	if !ok || r.now().After(rules.expires) {
		return robotsRules{}, false
	}

	return rules, true
}

func (r *Robots) store(site string, rules robotsRules) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	for k, v := range r.sites {
		if now.After(v.expires) {
			delete(r.sites, k)
		}
	}

	rules.expires = now.Add(r.ttl)
	r.sites[site] = rules
}

// fetch downloads and interprets a site's robots.txt the way RFC 9309 says:
// a missing file (any 4xx) allows everything, a server error means "try again
// later".
func (r *Robots) fetch(ctx context.Context, site string) (robotsRules, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, site+"/robots.txt", nil)
	if err != nil {
		return robotsRules{}, fmt.Errorf("robots.txt: %w", err)
	}
	req.Header.Set("User-Agent", netsafe.UserAgent)

	resp, err := r.client.Do(req)
	if errors.Is(err, netsafe.ErrTooManyRedirects) {
		// RFC 9309 lets a crawler treat a robots.txt lost in redirects as
		// unavailable, which allows everything.
		return robotsRules{}, nil
	}
	if err != nil {
		return robotsRules{}, fmt.Errorf("robots.txt: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxRobotsBytes))
		if err != nil {
			return robotsRules{}, fmt.Errorf("robots.txt: read: %w", err)
		}

		return robotsRules{body: string(body)}, nil

	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return robotsRules{}, fmt.Errorf("robots.txt: status %d", resp.StatusCode)

	case resp.StatusCode >= 400:
		return robotsRules{}, nil

	default:
		// A redirect the client refused to follow, or something stranger.
		return robotsRules{disallow: true}, nil
	}
}
