package store

import (
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
func TestComputeFee_WithinFreePeriod(t *testing.T) {
	// 20 min stay, 30 min free -> 0
	entry := ts("2026-08-16T10:00:00Z")
	exit := ts("2026-08-16T10:20:00Z")
	charged, amount := ComputeFee(entry, exit, 30, 500, 4000)
	if charged != 0 || amount != 0 {
		t.Fatalf("free stay: charged=%d amount=%d, want 0,0", charged, amount)
	}
}
func TestComputeFee_JustOverFreeRoundsUp(t *testing.T) {
	// 31 min stay, 30 min free -> 1 billable min -> 1 hour -> 500
	entry := ts("2026-08-16T10:00:00Z")
	exit := ts("2026-08-16T10:31:00Z")
	charged, amount := ComputeFee(entry, exit, 30, 500, 4000)
	if charged != 1 {
		t.Fatalf("charged=%d want 1", charged)
	}
	if amount != 500 {
		t.Fatalf("amount=%d want 500", amount)
	}
}
func TestComputeFee_PartialHourRoundsUp(t *testing.T) {
	// 2h stay, 30 min free -> 90 billable -> ceil 2h -> 1000
	entry := ts("2026-08-16T10:00:00Z")
	exit := ts("2026-08-16T12:00:00Z")
	charged, amount := ComputeFee(entry, exit, 30, 500, 4000)
	if charged != 90 {
		t.Fatalf("charged=%d want 90", charged)
	}
	if amount != 1000 {
		t.Fatalf("amount=%d want 1000", amount)
	}
}
func TestComputeFee_DailyCapSingleDay(t *testing.T) {
	// 6h stay, no free, 500/h -> 3000, cap 2000 -> 2000
	entry := ts("2026-08-16T10:00:00Z")
	exit := ts("2026-08-16T16:00:00Z")
	charged, amount := ComputeFee(entry, exit, 0, 500, 2000)
	if charged != 360 {
		t.Fatalf("charged=%d want 360", charged)
	}
	if amount != 2000 {
		t.Fatalf("amount=%d want 2000 (capped)", amount)
	}
}
func TestComputeFee_CrossDaySeparateCap(t *testing.T) {
	// entry 18:00 day1, exit 06:00 day2 = 12h total.
	// day1: 6h -> 6*500=3000 -> capped 2000
	// day2: 6h -> 6*500=3000 -> capped 2000
	// total 4000
	entry := ts("2026-08-16T18:00:00Z")
	exit := ts("2026-08-17T06:00:00Z")
	charged, amount := ComputeFee(entry, exit, 0, 500, 2000)
	if charged != 720 {
		t.Fatalf("charged=%d want 720", charged)
	}
	if amount != 4000 {
		t.Fatalf("amount=%d want 4000 (two capped days)", amount)
	}
}
func TestComputeFee_CrossDayNoCapHit(t *testing.T) {
	// entry 22:00 day1, exit 02:00 day2 = 4h total.
	// day1: 2h -> 1000; day2: 2h -> 1000; total 2000, no cap hit
	entry := ts("2026-08-16T22:00:00Z")
	exit := ts("2026-08-17T02:00:00Z")
	charged, amount := ComputeFee(entry, exit, 0, 500, 2000)
	if charged != 240 {
		t.Fatalf("charged=%d want 240", charged)
	}
	if amount != 2000 {
		t.Fatalf("amount=%d want 2000", amount)
	}
}
func TestComputeFee_FreeCarriesAcrossDay(t *testing.T) {
	// entry 23:00 day1, exit 01:30 day2 = 150 min. free 90.
	// day1 seg 60 min, all free, remaining free 30.
	// day2 seg 90 min, 30 free -> 60 billable -> 1h -> 500.
	entry := ts("2026-08-16T23:00:00Z")
	exit := ts("2026-08-17T01:30:00Z")
	charged, amount := ComputeFee(entry, exit, 90, 500, 4000)
	if charged != 60 {
		t.Fatalf("charged=%d want 60", charged)
	}
	if amount != 500 {
		t.Fatalf("amount=%d want 500", amount)
	}
}
func TestComputeFee_NoCapWhenZero(t *testing.T) {
	// daily cap 0 means no cap; 6h * 500 = 3000
	entry := ts("2026-08-16T10:00:00Z")
	exit := ts("2026-08-16T16:00:00Z")
	_, amount := ComputeFee(entry, exit, 0, 500, 0)
	if amount != 3000 {
		t.Fatalf("amount=%d want 3000 (no cap)", amount)
	}
}
func TestComputeFee_InvalidInputs(t *testing.T) {
	entry := ts("2026-08-16T10:00:00Z")
	// exit not after entry
	if _, amount := ComputeFee(entry, entry, 0, 500, 0); amount != 0 {
		t.Fatalf("zero duration should be free")
	}
	// zero rate
	if _, amount := ComputeFee(entry, entry.Add(time.Hour), 0, 0, 0); amount != 0 {
		t.Fatalf("zero rate should be free")
	}
}
func TestMinutesCeil(t *testing.T) {
	entry := ts("2026-08-16T10:00:00Z")
	if m := MinutesCeil(entry, entry.Add(90*time.Second)); m != 2 {
		t.Fatalf("90s -> %d want 2", m)
	}
	if m := MinutesCeil(entry, entry.Add(time.Hour)); m != 60 {
		t.Fatalf("1h -> %d want 60", m)
	}
	if m := MinutesCeil(entry, entry); m != 0 {
		t.Fatalf("0 -> %d want 0", m)
	}
}
