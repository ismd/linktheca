package crawler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestPacer(interval time.Duration) (*HostPacer, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	p := NewHostPacer(interval)
	p.now = clock.now

	return p, clock
}

func TestHostPacer_SpacesRequestsToOneHost(t *testing.T) {
	p, _ := newTestPacer(2 * time.Second)

	require.Zero(t, p.Wait("a.example", 1))
	require.Equal(t, 2*time.Second, p.Wait("a.example", 2))
	require.Equal(t, 4*time.Second, p.Wait("a.example", 3))
}

func TestHostPacer_HostsAreIndependent(t *testing.T) {
	p, _ := newTestPacer(2 * time.Second)

	require.Zero(t, p.Wait("a.example", 1))
	require.Zero(t, p.Wait("b.example", 2))
}

// A job that was told to wait gets its own slot when it comes back, instead
// of queueing behind everyone who asked in the meantime.
func TestHostPacer_HoldsTheSlotForTheWaitingJob(t *testing.T) {
	p, clock := newTestPacer(2 * time.Second)

	require.Zero(t, p.Wait("a.example", 1))
	require.Equal(t, 2*time.Second, p.Wait("a.example", 2))
	require.Equal(t, 4*time.Second, p.Wait("a.example", 3))

	clock.advance(2 * time.Second)
	require.Zero(t, p.Wait("a.example", 2), "job 2 comes back for its slot")

	clock.advance(time.Second)
	require.Equal(t, time.Second, p.Wait("a.example", 3), "job 3 came back early")

	clock.advance(time.Second)
	require.Zero(t, p.Wait("a.example", 3))

	require.Equal(t, 2*time.Second, p.Wait("a.example", 4), "new jobs queue after the held slots")
}

func TestHostPacer_FreeAfterInterval(t *testing.T) {
	p, clock := newTestPacer(2 * time.Second)

	require.Zero(t, p.Wait("a.example", 1))
	clock.advance(5 * time.Second)
	require.Zero(t, p.Wait("a.example", 2))
}

func TestHostPacer_ForgetsAbandonedSlots(t *testing.T) {
	p, clock := newTestPacer(2 * time.Second)

	require.Zero(t, p.Wait("a.example", 1))
	require.Positive(t, p.Wait("a.example", 2)) // job 2 never comes back

	clock.advance(2 * time.Hour)
	p.Wait("b.example", 3)

	require.NotContains(t, p.held, int64(2))
	require.NotContains(t, p.next, "a.example")
}

func TestHostPacer_ZeroIntervalDisablesPacing(t *testing.T) {
	p, _ := newTestPacer(0)

	require.Zero(t, p.Wait("a.example", 1))
	require.Zero(t, p.Wait("a.example", 2))
}
