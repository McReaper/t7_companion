package radiant

import "time"

// SetPollEvery shortens Open's polling for a test, and returns what restores it.
func SetPollEvery(d time.Duration) func() {
	old := pollEvery
	pollEvery = d
	return func() { pollEvery = old }
}

// SetMaxFrame lowers the frame cap for a test, and returns what restores it.
func SetMaxFrame(n uint32) func() {
	old := maxFrame
	maxFrame = n
	return func() { maxFrame = old }
}
