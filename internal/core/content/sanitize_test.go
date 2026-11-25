package content_test

import (
	"testing"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/stretchr/testify/require"
)

func TestSanitizeHTMLStripsActiveContent(t *testing.T) {
	tests := map[string]struct {
		in      string
		gone    string
		survive string
	}{
		"script":           {in: `<p>a</p><script>alert(1)</script>`, gone: "script", survive: "<p>a</p>"},
		"event handler":    {in: `<img src="https://x.example/a.png" onerror="alert(1)">`, gone: "onerror", survive: `src="https://x.example/a.png"`},
		"javascript link":  {in: `<a href="javascript:alert(1)">x</a>`, gone: "javascript:", survive: "x"},
		"iframe":           {in: `<p>a</p><iframe src="https://x.example"></iframe>`, gone: "iframe", survive: "<p>a</p>"},
		"inline style":     {in: `<p style="position:fixed;inset:0">a</p>`, gone: "style", survive: "<p>a</p>"},
		"style element":    {in: `<style>body{display:none}</style><p>a</p>`, gone: "display", survive: "<p>a</p>"},
		"form":             {in: `<form action="https://x.example"><input name="p"></form><p>a</p>`, gone: "form", survive: "<p>a</p>"},
		"data uri image":   {in: `<img src="data:image/svg+xml,<svg onload=alert(1)>">`, gone: "data:", survive: ""},
		"svg with handler": {in: `<svg onload="alert(1)"><circle/></svg><p>a</p>`, gone: "onload", survive: "<p>a</p>"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := content.SanitizeHTML(tt.in)

			require.NotContains(t, got, tt.gone)
			require.Contains(t, got, tt.survive)
		})
	}
}

// What readability produces for an ordinary article has to come through, or
// the reader turns into a wall of text.
func TestSanitizeHTMLKeepsArticleMarkup(t *testing.T) {
	in := `<div><h2>Heading</h2>` +
		`<p>Text with <em>emphasis</em>, <strong>bold</strong> and <a href="https://example.com/x">a link</a>.</p>` +
		`<figure><img src="https://example.com/a.png" alt="Alt"><figcaption>Caption</figcaption></figure>` +
		`<pre><code>fmt.Println("hi")</code></pre>` +
		`<blockquote>Quote</blockquote>` +
		`<ul><li>one</li></ul>` +
		`<table><tbody><tr><td>cell</td></tr></tbody></table></div>`

	got := content.SanitizeHTML(in)

	for _, want := range []string{
		"<h2>Heading</h2>",
		"<em>emphasis</em>",
		"<strong>bold</strong>",
		`href="https://example.com/x"`,
		`<img src="https://example.com/a.png" alt="Alt">`,
		"<figcaption>Caption</figcaption>",
		"<pre><code>",
		"<blockquote>Quote</blockquote>",
		"<li>one</li>",
		"<td>cell</td>",
	} {
		require.Contains(t, got, want)
	}
}

func TestSanitizeHTMLEmpty(t *testing.T) {
	require.Empty(t, content.SanitizeHTML(""))
}
