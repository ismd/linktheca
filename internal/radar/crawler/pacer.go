package crawler

import (
	"sync"
	"time"
)

// heldSlotTTL is how long a slot stays reserved for a job that has not come
// back for it, after which it is forgotten.
const heldSlotTTL = time.Hour

// HostPacer keeps requests to one host at least an interval apart, so a feed
// that brings fifty new links to the same site does not hit it fifty times at
// once.
//
// It hands out time slots instead of blocking. A job that has to wait is told
// how long, snoozes, and finds its slot still held for it when it comes back,
// so it waits once rather than competing again with every other job for that
// host. Slots live in memory, which is enough for a single process; after a
// restart a job just reserves a new one.
type HostPacer struct {
	interval time.Duration
	now      func() time.Time

	mu   sync.Mutex
	next map[string]time.Time // host → start of the next free slot
	held map[int64]time.Time  // job ID → slot reserved for it
}

// NewHostPacer returns a pacer allowing one request per host per interval.
// A zero interval turns pacing off.
func NewHostPacer(interval time.Duration) *HostPacer {
	return &HostPacer{
		interval: interval,
		now:      time.Now,
		next:     map[string]time.Time{},
		held:     map[int64]time.Time{},
	}
}

// Wait returns how long job has to wait before it may send a request to
// host. Zero means it may go now; otherwise a slot is reserved for the job
// and it should come back after the returned delay.
func (p *HostPacer) Wait(host string, jobID int64) time.Duration {
	if p.interval <= 0 {
		return 0
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()

	if slot, ok := p.held[jobID]; ok {
		if wait := slot.Sub(now); wait > 0 {
			return wait
		}

		delete(p.held, jobID)

		return 0
	}

	slot := p.next[host]
	if slot.Before(now) {
		slot = now
	}
	p.next[host] = slot.Add(p.interval)

	wait := slot.Sub(now)
	if wait > 0 {
		p.held[jobID] = slot
	}

	p.forget(now)

	return wait
}

// forget drops hosts whose next slot has passed and slots whose jobs never
// came back, so the maps do not grow without bound.
func (p *HostPacer) forget(now time.Time) {
	for host, slot := range p.next {
		if slot.Before(now) {
			delete(p.next, host)
		}
	}

	for job, slot := range p.held {
		if now.Sub(slot) > heldSlotTTL {
			delete(p.held, job)
		}
	}
}
