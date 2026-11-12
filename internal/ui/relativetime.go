package ui

import (
	"fmt"
	"time"
)

// relativeTime names how long ago at happened, short form for dense rows.
//
// A zero time keeps its own wording so callers can pass an optional
// timestamp without inventing a value.
func relativeTime(now, at time.Time) string {
	if at.IsZero() {
		return "not refreshed yet"
	}

	if now.Before(at) {
		now = at
	}
	age := now.Sub(at)

	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	default:
		return "on " + at.Format("2 Jan 2006")
	}
}

// relativeTimeLong names how long ago at happened for the Home hero, where a
// sentence reads better than the row's shorthand.
func relativeTimeLong(now, at time.Time) string {
	if at.IsZero() {
		return ""
	}

	if now.Before(at) {
		now = at
	}
	age := now.Sub(at)

	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return plural(int(age.Minutes()), "minute")
	case age < 24*time.Hour:
		return plural(int(age.Hours()), "hour")
	case age < 7*24*time.Hour:
		return plural(int(age.Hours()/24), "day")
	default:
		return "on " + at.Format("2 January 2006")
	}
}

// plural names one interval with the right noun form.
func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun + " ago"
	}

	return fmt.Sprintf("%d %ss ago", count, noun)
}
