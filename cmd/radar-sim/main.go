// Command radar-sim is a debugging aid for Radar matching: it scores
// radar_findings against a probe the way the matcher does and prints them
// ranked, with the plain cosine alongside, so you can see why something did
// (or did not) match a topic.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ismd/linktheca/internal/core/db"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/pgvector/pgvector-go"
)

type mode string

const (
	modeQuery      mode = "query"
	modeTopic      mode = "topic"
	modeListTopics mode = "list-topics"
)

type options struct {
	mode           mode
	query          string
	topicID        int64
	limit          int
	threshold      *float32
	subscribedOnly bool
}

func newFlagSet(opts *options, threshold *float64) *flag.FlagSet {
	fs := flag.NewFlagSet("radar-sim", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&opts.query, "q", "", "text to embed via TEI and score against findings")
	fs.Int64Var(&opts.topicID, "topic", 0, "score findings against the stored embedding of this topic")
	fs.Bool("topics", false, "list topics with their id, threshold and embedding state")
	fs.BoolVar(&opts.subscribedOnly, "subscribed", false, "with -topic: only findings from feeds the topic owner is subscribed to")
	fs.IntVar(&opts.limit, "limit", 20, "how many findings to print")
	fs.Float64Var(threshold, "threshold", 0, "draw the cutoff line at this score (default: the topic's cutoff)")

	return fs
}

// usage documents the modes; the flag descriptions come from newFlagSet.
func usage(w io.Writer) {
	fmt.Fprint(w, `radar-sim — rank radar_findings by match score.

Usage:
  radar-sim -q "<text>" [-limit N] [-threshold X]   probe with freshly embedded text
  radar-sim -topic <id> [-subscribed] [-limit N]    probe with a topic's stored embedding
  radar-sim -topics                                 list topics and their ids

Environment:
  LINKTHECA_DB_DSN       required
  LINKTHECA_TEI_URL      default http://localhost:8081 (only used by -q)
  LINKTHECA_TEI_TIMEOUT  default 30s

Flags:
`)

	var (
		opts      options
		threshold float64
	)
	fs := newFlagSet(&opts, &threshold)
	fs.SetOutput(w)
	fs.PrintDefaults()

	fmt.Fprint(w, `
Topics are embedded from their description alone. Findings are embedded as
"Title\n<article text>" when Radar downloaded the page (src "page") and as
"Title\nSummary" from the feed when it did not (src "feed"); the model reads
about the first thousand tokens. A page pruned after the retention period
shows as "feed" even though the finding was embedded from it.

"score" is what the matcher compares with the topic's cutoff: the mean of the
topic's similarity to the finding's text and to its title, minus the same for
an average topic (see radar.Reference). A topic that learned from the user's
marks scores with its moved vector and offset, and its cutoff may sit above
match_threshold (see radar.LearnTopic). "cos" is the plain similarity of the
topic's description and the text, for comparison. Findings embedded before
match vectors existed are left out until the server has filled theirs in.
`)
}

func parseArgs(args []string) (*options, error) {
	var (
		opts      options
		threshold float64
	)
	fs := newFlagSet(&opts, &threshold)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	modes := []struct {
		flag string
		mode mode
	}{
		{"q", modeQuery},
		{"topic", modeTopic},
		{"topics", modeListTopics},
	}
	for _, m := range modes {
		if !given[m.flag] {
			continue
		}
		if opts.mode != "" {
			return nil, errors.New("specify exactly one of -q, -topic, -topics")
		}
		opts.mode = m.mode
	}
	if opts.mode == "" {
		return nil, errors.New("nothing to do: pass -q <text>, -topic <id> or -topics")
	}

	if opts.subscribedOnly && opts.mode != modeTopic {
		return nil, errors.New("-subscribed only makes sense with -topic")
	}

	if opts.limit < 1 {
		return nil, fmt.Errorf("limit must be positive, got %d", opts.limit)
	}

	if given["threshold"] {
		if threshold < 0 || threshold > 1 {
			return nil, fmt.Errorf("threshold must be within [0, 1], got %v", threshold)
		}
		t := float32(threshold)
		opts.threshold = &t
	}

	return &opts, nil
}

// titleWidth caps the printed title so rows stay on one terminal line.
const titleWidth = 70

// scoredFinding is a radar_findings row with its match score against the
// probe and the plain cosine of probe and text.
type scoredFinding struct {
	ID    int64
	Title *string
	URL   string
	Score float32
	Cos   float32
	// HasPage tells whether the downloaded article is stored, i.e. whether
	// the finding was embedded from the page or from the feed's summary.
	HasPage bool
}

// renderScores prints rows (already sorted by score, descending) and, when
// threshold is set, a cutoff line where the score drops below it.
func renderScores(w io.Writer, rows []scoredFinding, threshold *float32) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "no findings with embeddings — is the embed job running?")
		return
	}

	fmt.Fprintf(w, "%8s  %6s  %6s  %4s  %s\n", "score", "cos", "id", "src", "title")

	cutDrawn := threshold == nil
	for _, r := range rows {
		if !cutDrawn && r.Score < *threshold {
			fmt.Fprintf(w, "%s threshold %.4f %s\n",
				strings.Repeat("─", 8), *threshold, strings.Repeat("─", 40))
			cutDrawn = true
		}
		fmt.Fprintf(w, "%8.4f  %6.4f  %6d  %4s  %s\n",
			r.Score, r.Cos, r.ID, textSource(r.HasPage), displayTitle(r.Title))
	}
}

// renderTopics prints the topic inventory used to pick an id for -topic.
func renderTopics(w io.Writer, topics []topicRow) {
	if len(topics) == 0 {
		fmt.Fprintln(w, "no topics")
		return
	}

	fmt.Fprintf(w, "%6s  %10s  %7s  %-30s  %s\n", "id", "threshold", "cutoff", "name", "owner")
	for _, t := range topics {
		notes := ""
		if !t.HasEmbedding {
			notes += "  [no embedding]"
		}
		if !t.IsActive {
			notes += "  [inactive]"
		}
		fmt.Fprintf(w, "%6d  %10.4f  %7.4f  %-30s  %s%s\n",
			t.ID, t.MatchThreshold, t.MatchCutoff, displayTitle(&t.Name), t.OwnerEmail, notes)
	}
}

func textSource(hasPage bool) string {
	if hasPage {
		return "page"
	}

	return "feed"
}

func displayTitle(t *string) string {
	if t == nil || strings.TrimSpace(*t) == "" {
		return "(no title)"
	}

	s := strings.Join(strings.Fields(*t), " ")

	runes := []rune(s)
	if len(runes) > titleWidth {
		return string(runes[:titleWidth-1]) + "…"
	}

	return s
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "radar-sim:", err)
		os.Exit(1)
	}
}

// run reads its connection settings straight from the environment instead of
// config.Load(): this tool only reads the database and TEI, and demanding a
// JWT secret to print similarity scores would be pure friction.
func run(ctx context.Context, args []string, out io.Writer) error {
	opts, err := parseArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		usage(out)
		return nil
	}
	if err != nil {
		return err
	}

	dsn := os.Getenv("LINKTHECA_DB_DSN")
	if dsn == "" {
		return errors.New("LINKTHECA_DB_DSN is not set")
	}

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch opts.mode {
	case modeListTopics:
		topics, err := listTopics(ctx, pool)
		if err != nil {
			return err
		}
		renderTopics(out, topics)
		return nil

	case modeTopic:
		topic, err := loadTopic(ctx, pool, opts.topicID)
		if err != nil {
			return err
		}

		threshold := opts.threshold
		if threshold == nil {
			threshold = &topic.MatchCutoff
		}

		scope := "all findings"
		if opts.subscribedOnly {
			scope = "findings from subscribed feeds"
		}
		fmt.Fprintf(out, "topic %d %q (owner %s) vs %s\n\n", topic.ID, topic.Name, topic.OwnerEmail, scope)

		rows, err := topFindingsByTopic(ctx, pool, opts.topicID, opts.limit, opts.subscribedOnly)
		if err != nil {
			return err
		}
		renderScores(out, rows, threshold)
		return nil

	case modeQuery:
		tei := embeddings.NewTEIClient(teiURL(), teiTimeout())
		vec, err := tei.Embed(ctx, radar.TopicEmbedText(opts.query))
		if err != nil {
			return fmt.Errorf("embed query: %w", err)
		}
		matchVec, err := radar.NewReference(tei).TopicMatchVector(ctx, vec)
		if err != nil {
			return fmt.Errorf("match vector of query: %w", err)
		}

		fmt.Fprintf(out, "query %q\n\n", opts.query)

		rows, err := topFindingsByVector(ctx, pool, pgvector.NewVector(vec), pgvector.NewVector(matchVec), opts.limit)
		if err != nil {
			return err
		}
		renderScores(out, rows, opts.threshold)
		return nil
	}

	return fmt.Errorf("unknown mode %q", opts.mode)
}

func teiURL() string {
	if url := os.Getenv("LINKTHECA_TEI_URL"); url != "" {
		return url
	}
	return "http://localhost:8081"
}

func teiTimeout() time.Duration {
	if raw := os.Getenv("LINKTHECA_TEI_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			return d
		}
	}
	return 30 * time.Second
}
