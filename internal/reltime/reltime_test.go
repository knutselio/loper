package reltime_test

import (
	"testing"
	"time"

	"github.com/knutselio/loper/internal/reltime"
)

var now = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

// check fails the test unless a time ago before now reads as want.
func check(t *testing.T, ago time.Duration, want string) {
	t.Helper()
	if got := reltime.Format(now.Add(-ago), now); got != want {
		t.Errorf("%v ago reads %q, want %q", ago, got, want)
	}
}

func TestUnderAMinuteAgoIsJustNow(t *testing.T) {
	check(t, 0, "just now")
	check(t, 59*time.Second, "just now")
}

func TestATimeInTheFutureIsJustNow(t *testing.T) {
	check(t, -time.Hour, "just now")
}

func TestFromAMinuteToAnHourIsWholeMinutes(t *testing.T) {
	check(t, time.Minute, "1m ago")
	check(t, 59*time.Minute+59*time.Second, "59m ago")
}

func TestFromAnHourToADayIsWholeHours(t *testing.T) {
	check(t, time.Hour, "1h ago")
	check(t, 2*time.Hour+59*time.Minute, "2h ago")
	check(t, 23*time.Hour+59*time.Minute, "23h ago")
}

func TestFromOneToTwoDaysIsYesterday(t *testing.T) {
	check(t, 24*time.Hour, "yesterday")
	check(t, 47*time.Hour+59*time.Minute, "yesterday")
}

func TestFromTwoDaysToAWeekIsWholeDays(t *testing.T) {
	check(t, 48*time.Hour, "2d ago")
	check(t, 7*24*time.Hour-time.Minute, "6d ago")
}

func TestFromAWeekTo30DaysIsWholeWeeks(t *testing.T) {
	check(t, 7*24*time.Hour, "1w ago")
	check(t, 30*24*time.Hour-time.Minute, "4w ago")
}

func TestFrom30DaysToAYearIsWhole30DayMonths(t *testing.T) {
	check(t, 30*24*time.Hour, "1mo ago")
	check(t, 365*24*time.Hour-time.Minute, "12mo ago")
}

func TestFromAYearOnIsWhole365DayYears(t *testing.T) {
	check(t, 365*24*time.Hour, "1y ago")
	check(t, 3*365*24*time.Hour, "3y ago")
}
