package reloader

import "time"

// Clock is the time source for the core's debounce and backoff timers, kept
// as an injectable seam so the orchestration logic is testable without real
// sleeps.
//
// It is intentionally minimal and may grow (for example with stoppable
// timers) as the reload loop's needs firm up.
type Clock interface {
	// After returns a channel that delivers the current time once d has
	// elapsed.
	After(d time.Duration) <-chan time.Time
}

// systemClock is the real-time Clock selected when Options.Clock is nil.
type systemClock struct{}

func (systemClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
