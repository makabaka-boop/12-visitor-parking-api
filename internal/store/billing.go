package store

import "time"

// ComputeFee calculates the temporary-parking fee in integer cents (1元 = 100分)
// for a stay from entry to exit under the given tariff.
//
// Rules (matching the product spec):
//   - freeMinutes is a one-time grace consumed from the start of the stay. If
//     the whole stay fits within the grace, the fee is 0.
//   - Once the grace is exhausted, chargeable time is rounded UP to whole hours
//     ("不足一小时按一小时计算").
//   - The stay is split per natural day (midnight boundary in entry's location);
//     each day's charge is independently capped at dailyCapCents
//     ("跨自然日分别应用每日封顶"). A dailyCapCents of 0 means no cap.
//
// The function is pure and allocation-free so both the memory and PostgreSQL
// stores share identical billing math. All arithmetic uses integers, never
// floats, so monetary results are exact.
//
// It returns (chargedMinutes, amountCents): the billable minutes after the
// grace and the total amount in cents.
func ComputeFee(entry, exit time.Time, freeMinutes int, hourlyRateCents, dailyCapCents int64) (chargedMinutes, amountCents int64) {
	if !exit.After(entry) || hourlyRateCents <= 0 {
		return 0, 0
	}
	loc := entry.Location()
	remainingFree := int64(freeMinutes)
	cur := entry
	for cur.Before(exit) {
		y, m, d := cur.Date()
		dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
		segEnd := dayStart.Add(24 * time.Hour)
		if exit.Before(segEnd) {
			segEnd = exit
		}
		segDur := segEnd.Sub(cur)
		segMinutes := int64(segDur / time.Minute)
		if segDur%time.Minute != 0 {
			segMinutes++ // round sub-minute remainders up
		}
		var billable int64
		if remainingFree >= segMinutes {
			remainingFree -= segMinutes
			billable = 0
		} else {
			billable = segMinutes - remainingFree
			remainingFree = 0
		}
		if billable > 0 {
			hours := (billable + 59) / 60 // ceil to whole hours
			dayAmount := hours * hourlyRateCents
			if dailyCapCents > 0 && dayAmount > dailyCapCents {
				dayAmount = dailyCapCents
			}
			amountCents += dayAmount
			chargedMinutes += billable
		}
		cur = segEnd
	}
	return chargedMinutes, amountCents
}

// MinutesCeil returns the duration between entry and exit rounded up to whole
// minutes, used for the informational DurationMinutes field on a fee.
func MinutesCeil(entry, exit time.Time) int64 {
	d := exit.Sub(entry)
	if d <= 0 {
		return 0
	}
	m := int64(d / time.Minute)
	if d%time.Minute != 0 {
		m++
	}
	return m
}
