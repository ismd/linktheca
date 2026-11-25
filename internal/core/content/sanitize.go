package content

import "github.com/microcosm-cc/bluemonday"

// articlePolicy is built once: a bluemonday policy is safe for concurrent
// Sanitize calls but not for concurrent changes.
var articlePolicy = bluemonday.UGCPolicy()

// SanitizeHTML strips everything from article HTML that could run in the
// reader: scripts, event handlers, javascript: links, iframes, forms, styles.
// Readability keeps the markup of the page it parsed, and that page is
// untrusted, so the HTML has to pass through here before the frontend renders
// it. It runs when the HTML is served rather than when it is stored, which
// covers rows saved before it existed and picks up policy changes for free.
func SanitizeHTML(html string) string {
	if html == "" {
		return ""
	}

	return articlePolicy.Sanitize(html)
}
