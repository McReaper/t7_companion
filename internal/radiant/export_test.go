package radiant

import "time"

// SetPollEvery shortens Open's polling for a test, and returns what restores it.
func SetPollEvery(d time.Duration) func() {
	old := pollEvery
	pollEvery = d
	return func() { pollEvery = old }
}

const MaxFrame = maxFrame
