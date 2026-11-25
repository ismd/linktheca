package jobs

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/stretchr/testify/require"
)

func strp(s string) *string { return &s }

func TestEmbedText(t *testing.T) {
	tests := map[string]struct {
		f    radar.FindingForEmbed
		want string
	}{
		"title and page text": {
			f:    radar.FindingForEmbed{Title: strp(" Rust 2.0 "), Summary: strp("Feed summary"), Text: strp(" Article body \n")},
			want: "Rust 2.0\nArticle body",
		},
		"no page: title and summary": {
			f:    radar.FindingForEmbed{Title: strp("Rust 2.0"), Summary: strp(" Feed summary ")},
			want: "Rust 2.0\nFeed summary",
		},
		"blank page text falls back to summary": {
			f:    radar.FindingForEmbed{Title: strp("Rust 2.0"), Summary: strp("Feed summary"), Text: strp("  \n ")},
			want: "Rust 2.0\nFeed summary",
		},
		"title only": {
			f:    radar.FindingForEmbed{Title: strp("Rust 2.0")},
			want: "Rust 2.0",
		},
		"page text without title": {
			f:    radar.FindingForEmbed{Text: strp("Article body")},
			want: "Article body",
		},
		"nothing": {
			f:    radar.FindingForEmbed{},
			want: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, embedText(&tt.f))
		})
	}
}

func TestEmbedText_CapsLongArticles(t *testing.T) {
	body := strings.Repeat("ж", maxEmbedTextRunes+500)

	got := embedText(&radar.FindingForEmbed{Title: strp("T"), Text: &body})

	require.Equal(t, 2+maxEmbedTextRunes, utf8.RuneCountInString(got))
	require.True(t, utf8.ValidString(got))
}
