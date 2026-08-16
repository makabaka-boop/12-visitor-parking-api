package model

import (
	"testing"
	"time"
)

func TestExtensibleAuth(t *testing.T) {
	base := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	end := base.Add(5 * time.Hour) // 15:00

	cases := []struct {
		name   string
		status string
		end    time.Time
		now    time.Time
		want   bool
	}{
		// pending authorizations: extensible only while inside their window
		{"pending within window", AuthStatusPending, end, base.Add(1 * time.Hour), true},
		{"pending at end time", AuthStatusPending, end, end, false},
		{"pending past end time", AuthStatusPending, end, base.Add(6 * time.Hour), false},

		// active authorizations: extensible only while their validity window
		// has not yet passed. An active authorization whose end time has been
		// reached is effectively expired, even though its stored status has not
		// been derived to "expired" (derivation only applies to pending).
		{"active within window", AuthStatusActive, end, base.Add(1 * time.Hour), true},
		{"active at end time", AuthStatusActive, end, end, false},
		{"active past end time", AuthStatusActive, end, base.Add(6 * time.Hour), false},

		// terminal / derived statuses are never extensible
		{"completed", AuthStatusCompleted, end, base, false},
		{"cancelled", AuthStatusCancelled, end, base, false},
		{"expired", AuthStatusExpired, end, base, false},
		{"empty status", "", end, base, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtensibleAuth(c.status, c.end, c.now); got != c.want {
				t.Fatalf("ExtensibleAuth(%s, end=%v, now=%v) = %v, want %v", c.status, c.end, c.now, got, c.want)
			}
		})
	}
}

// TestExtensibleAuth_ActivePastEndTimeNotExtensible is the targeted reproduction
// for the bug where an active authorization whose end time has passed was still
// treated as extensible, contradicting the rule that expired authorizations may
// not be extended.
func TestExtensibleAuth_ActivePastEndTimeNotExtensible(t *testing.T) {
	base := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	end := base.Add(5 * time.Hour) // 15:00
	now := base.Add(6 * time.Hour) // 16:00, past the authorization's end time

	if ExtensibleAuth(AuthStatusActive, end, now) {
		t.Fatal("active authorization past its end time must not be extensible (rule: expired authorizations cannot be extended)")
	}
}
