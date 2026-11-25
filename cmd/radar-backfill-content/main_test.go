package main

import (
	"testing"

	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/stretchr/testify/require"
)

func TestParseArgs(t *testing.T) {
	opts, err := parseArgs(nil)
	require.NoError(t, err)
	require.Equal(t, &options{days: 7}, opts)

	opts, err = parseArgs([]string{"-days", "30", "-match", "-dry-run"})
	require.NoError(t, err)
	require.Equal(t, &options{days: 30, match: true, dryRun: true}, opts)

	_, err = parseArgs([]string{"-days", "0"})
	require.Error(t, err)
}

func TestJobArgs(t *testing.T) {
	quiet := jobArgs([]int64{1, 2}, false)
	require.Equal(t, jobs.FetchFindingContentArgs{FindingID: 1, Reembed: true, SkipMatch: true}, quiet[0].Args)
	require.Equal(t, jobs.FetchFindingContentArgs{FindingID: 2, Reembed: true, SkipMatch: true}, quiet[1].Args)

	matching := jobArgs([]int64{3}, true)
	require.Equal(t, jobs.FetchFindingContentArgs{FindingID: 3, Reembed: true, SkipMatch: false}, matching[0].Args)
}
