package content

import "testing"

// SetMaxPageBytes lowers the page size cap for one test, so a test page past
// it does not have to be megabytes long.
func SetMaxPageBytes(t *testing.T, n int64) {
	old := maxPageBytes
	maxPageBytes = n
	t.Cleanup(func() { maxPageBytes = old })
}
