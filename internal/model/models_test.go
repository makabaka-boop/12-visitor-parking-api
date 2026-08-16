package model

import (
	"testing"
	"time"
)

func TestExtensibleAuthRejectsExpiredActiveAuthorization(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

	if ExtensibleAuth(AuthStatusActive, now.Add(-time.Second), now) {
		t.Fatal("expired active authorization must not be extensible")
	}
	if ExtensibleAuth(AuthStatusActive, now, now) {
		t.Fatal("authorization ending at the current time must not be extensible")
	}
	if !ExtensibleAuth(AuthStatusActive, now.Add(time.Second), now) {
		t.Fatal("active authorization before its end time should be extensible")
	}
}
