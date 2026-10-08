package supervisor

import (
	"time"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
)

// restartLimits controls automatic restarts. Production code uses
// defaultRestartLimits; tests use much shorter durations.
type restartLimits struct {
	maxRestarts int           // consecutive restarts before giving up
	baseDelay   time.Duration // wait before the first restart
	maxDelay    time.Duration // upper bound for any wait
	stableAfter time.Duration // a run at least this long resets the count
}

var defaultRestartLimits = restartLimits{
	maxRestarts: 5,
	baseDelay:   time.Second,
	maxDelay:    30 * time.Second,
	stableAfter: 10 * time.Second,
}

// runOutcome describes how one run of a service ended.
type runOutcome struct {
	exitCode      int
	err           error
	stopRequested bool
	duration      time.Duration
}

// failed reports whether the run ended unsuccessfully. A process killed
// by a signal has exit code -1, so it also counts as a failure.
func (o runOutcome) failed() bool {
	return o.exitCode != 0 || o.err != nil
}

// shouldRestart reports whether a service should start again after a
// run ended with outcome o. A service stackrun stopped never restarts.
func shouldRestart(policy config.RestartPolicy, o runOutcome) bool {
	if o.stopRequested {
		return false
	}
	switch policy {
	case config.RestartAlways:
		return true
	case config.RestartOnFailure:
		return o.failed()
	}
	return false
}

// backoffDelay returns the wait before restart attempt n (starting at 1):
// baseDelay doubled for each earlier attempt, never more than maxDelay.
// Doubling in a loop with an early return avoids integer overflow, which
// a formula such as baseDelay << n would hit for large n.
func (l restartLimits) backoffDelay(attempt int) time.Duration {
	delay := l.baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= l.maxDelay {
			return l.maxDelay
		}
	}
	return min(delay, l.maxDelay)
}

// nextAttempt returns the attempt number for the restart that follows a
// run. A run lasting at least stableAfter counts as healthy, so counting
// starts again from 1; otherwise it continues from previous.
func (l restartLimits) nextAttempt(previous int, runDuration time.Duration) int {
	if runDuration >= l.stableAfter {
		return 1
	}
	return previous + 1
}
