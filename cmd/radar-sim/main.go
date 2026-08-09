// Command radar-sim is a debugging aid for Radar embeddings: it scores
// radar_findings by cosine similarity against a probe vector and prints them
// ranked, so you can see why something did (or did not) match a topic.
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
	fs.Float64Var(threshold, "threshold", 0, "draw the cutoff line at this similarity (default: the topic's match_threshold)")

	return fs
}

// usage documents the modes; the flag descriptions come from newFlagSet.
func usage(w io.Writer) {
	fmt.Fprint(w, `radar-sim — rank radar_findings by embedding similarity.

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
Topics are embedded as "Name: description" and findings as "Title\nSummary",
so mirror that formatting in -q to reproduce what the matcher sees.
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

// scoredFinding is a radar_findings row with its cosine similarity to the
// probe vector.
type scoredFinding struct {
	ID         int64
	Title      *string
	URL        string
	Similarity float32
}

// renderScores prints rows (already sorted by similarity, descending) and,
// when threshold is set, a cutoff line where the score drops below it.
func renderScores(w io.Writer, rows []scoredFinding, threshold *float32) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "no findings with embeddings — is the embed job running?")
		return
	}

	fmt.Fprintf(w, "%8s  %6s  %s\n", "sim", "id", "title")

	cutDrawn := threshold == nil
	for _, r := range rows {
		if !cutDrawn && r.Similarity < *threshold {
			fmt.Fprintf(w, "%s threshold %.4f %s\n",
				strings.Repeat("─", 8), *threshold, strings.Repeat("─", 40))
			cutDrawn = true
		}
		fmt.Fprintf(w, "%8.4f  %6d  %s\n", r.Similarity, r.ID, displayTitle(r.Title))
	}
}

// renderTopics prints the topic inventory used to pick an id for -topic.
func renderTopics(w io.Writer, topics []topicRow) {
	if len(topics) == 0 {
		fmt.Fprintln(w, "no topics")
		return
	}

	fmt.Fprintf(w, "%6s  %10s  %-30s  %s\n", "id", "threshold", "name", "owner")
	for _, t := range topics {
		notes := ""
		if !t.HasEmbedding {
			notes += "  [no embedding]"
		}
		if !t.IsActive {
			notes += "  [inactive]"
		}
		fmt.Fprintf(w, "%6d  %10.4f  %-30s  %s%s\n",
			t.ID, t.MatchThreshold, displayTitle(&t.Name), t.OwnerEmail, notes)
	}
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
			threshold = &topic.MatchThreshold
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
		vec, err := tei.Embed(ctx, opts.query)
		if err != nil {
			return fmt.Errorf("embed query: %w", err)
		}

		fmt.Fprintf(out, "query %q\n\n", opts.query)

		rows, err := topFindingsByVector(ctx, pool, pgvector.NewVector(vec), opts.limit)
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
