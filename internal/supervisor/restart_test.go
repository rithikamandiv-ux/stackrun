package supervisor

import (
	"errors"
	"testing"
	"time"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
	"github.com/rithikamandiv-ux/stackrun/internal/events"
)

func TestShouldRestart(t *testing.T) {
	success := runOutcome{exitCode: 0}
	failure := runOutcome{exitCode: 1}
	crashed := runOutcome{exitCode: events.ExitCodeSignal}
	waitErr := runOutcome{exitCode: 0, err: errors.New("wait failed")}
	stopped := runOutcome{exitCode: events.ExitCodeSignal, stopRequested: true}

	tests := []struct {
		name    string
		policy  config.RestartPolicy
		outcome runOutcome
		want    bool
	}{
		{"never after failure", config.RestartNever, failure, false},
		{"never after success", config.RestartNever, success, false},
		{"on-failure after failure", config.RestartOnFailure, failure, true},
		{"on-failure after crash", config.RestartOnFailure, crashed, true},
		{"on-failure after wait error", config.RestartOnFailure, waitErr, true},
		{"on-failure after success", config.RestartOnFailure, success, false},
		{"always after success", config.RestartAlways, success, true},
		{"always after failure", config.RestartAlways, failure, true},
		{"always after requested stop", config.RestartAlways, stopped, false},
		{"on-failure after requested stop", config.RestartOnFailure, stopped, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRestart(tt.policy, tt.outcome); got != tt.want {
				t.Errorf("shouldRestart(%q, %+v) = %v, want %v", tt.policy, tt.outcome, got, tt.want)
			}
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	limits := defaultRestartLimits

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 30 * time.Second}, // 32s capped
		{7, 30 * time.Second},
		{1000, 30 * time.Second}, // no overflow
	}

	for _, tt := range tests {
		if got := limits.backoffDelay(tt.attempt); got != tt.want {
			t.Errorf("backoffDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestNextAttempt(t *testing.T) {
	limits := defaultRestartLimits

	tests := []struct {
		name     string
		previous int
		duration time.Duration
		want     int
	}{
		{"first quick failure", 0, time.Second, 1},
		{"consecutive quick failure", 3, time.Second, 4},
		{"stable run resets", 4, time.Minute, 1},
		{"exactly the stable period resets", 4, limits.stableAfter, 1},
		{"just under the stable period continues", 4, limits.stableAfter - time.Millisecond, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := limits.nextAttempt(tt.previous, tt.duration); got != tt.want {
				t.Errorf("nextAttempt(%d, %v) = %d, want %d", tt.previous, tt.duration, got, tt.want)
			}
		})
	}
}
