// Package reltime formats a past time relative to the current time, short
// enough for a narrow column.
package reltime

import (
	"fmt"
	"time"
)

const day = 24 * time.Hour

// Format says how long before now t was, in whole units rounded down: "just
// now", "5m ago", "2h ago", "yesterday", "3d ago", "2w ago", "4mo ago" or
// "1y ago". A month is 30 days and a year 365. A time after now, from a clock
// that was moved back, is "just now".
func Format(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", d/time.Minute)
	case d < day:
		return fmt.Sprintf("%dh ago", d/time.Hour)
	case d < 2*day:
		return "yesterday"
	case d < 7*day:
		return fmt.Sprintf("%dd ago", d/day)
	case d < 30*day:
		return fmt.Sprintf("%dw ago", d/(7*day))
	case d < 365*day:
		return fmt.Sprintf("%dmo ago", d/(30*day))
	}
	return fmt.Sprintf("%dy ago", d/(365*day))
}
