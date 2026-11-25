package radar

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestDeriveTopicName(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want string
	}{
		{"short description is used whole", "Rust async runtimes", "Rust async runtimes"},
		{"cut at colon", "Open-weight LLM releases: Llama, Qwen, Mistral", "Open-weight LLM releases"},
		{"cut at sentence end", "Passkeys and WebAuthn. Browser support, spec changes", "Passkeys and WebAuthn"},
		{"cut at semicolon", "Postgres releases; extensions too", "Postgres releases"},
		{"cut at spaced dash", "Home Assistant — new integrations", "Home Assistant"},
		{"cut at spaced hyphen", "Home Assistant - new integrations", "Home Assistant"},
		{"dots inside words survive", "Node.js and Deno v2.1 releases", "Node.js and Deno v2.1 releases"},
		{"urls survive", "News from https://go.dev blog", "News from https://go.dev blog"},
		{"hyphenated words survive", "Open-source self-hosted tools", "Open-source self-hosted tools"},
		{"first line only", "Self-hosting\nDocker, Proxmox, homelab gear", "Self-hosting"},
		{"cyrillic", "Новости Rust: релизы, RFC и экосистема", "Новости Rust"},
		{"leading punctuation is not a break", ": weird start of a description", ": weird start of a description"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, deriveTopicName(tc.desc))
		})
	}
}

func TestDeriveTopicName_TruncatesAtWordBoundary(t *testing.T) {
	desc := "Новые релизы и бенчмарки открытых языковых моделей вроде Llama Qwen Mistral DeepSeek"

	got := deriveTopicName(desc)

	require.True(t, strings.HasSuffix(got, "…"), got)
	require.LessOrEqual(t, utf8.RuneCountInString(got), maxDerivedNameRunes+1)

	head := strings.TrimSuffix(got, "…")
	require.True(t, strings.HasPrefix(desc, head+" "),
		"the name must be a word-aligned prefix of the description: %q", got)
}

func TestDeriveTopicName_LongWordWithoutSpaces(t *testing.T) {
	got := deriveTopicName(strings.Repeat("ж", 100))

	require.Equal(t, strings.Repeat("ж", maxDerivedNameRunes)+"…", got)
}
