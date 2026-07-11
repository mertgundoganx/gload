package scheduler

import (
	"strings"
	"testing"
	"time"
)

func TestValidateCron(t *testing.T) {
	valid := []string{
		"* * * * *",
		"0 3 * * *",
		"*/5 * * * *",
		"0 6 * * 1-5",       // weekday range
		"0 9 * * 1,3,5",     // list
		"0 0-6/2 * * *",     // range with step
		"30 2 1,15 * *",     // day list
		"0 6 * * 7",         // 7 = Sunday
	}
	for _, e := range valid {
		if err := ValidateCron(e); err != nil {
			t.Errorf("expected %q valid, got: %v", e, err)
		}
	}
	invalid := []string{
		"* * * *",       // too few fields
		"60 * * * *",    // minute out of range
		"* 24 * * *",    // hour out of range
		"* * 0 * *",     // day-of-month < 1
		"* * * 13 *",    // month > 12
		"* * * * 8",     // weekday > 7
		"5-1 * * * *",   // inverted range
		"*/0 * * * *",   // zero step
		"a * * * *",     // non-numeric
	}
	for _, e := range invalid {
		if err := ValidateCron(e); err == nil {
			t.Errorf("expected %q invalid, got no error", e)
		}
	}
}

func TestCronMatching(t *testing.T) {
	// 2026-01-05 is a Monday.
	monday := time.Date(2026, 1, 5, 6, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 1, 4, 6, 0, 0, 0, time.UTC)

	cases := []struct {
		expr string
		t    time.Time
		want bool
	}{
		{"0 6 * * 1-5", monday, true},  // weekday range matches Monday
		{"0 6 * * 1-5", sunday, false}, // ...but not Sunday
		{"0 6 * * 1,3,5", monday, true},
		{"0 6 * * 0", sunday, true}, // 0 = Sunday
		{"0 6 * * 7", sunday, true}, // 7 = Sunday alias
		{"*/15 * * * *", time.Date(2026, 1, 5, 6, 30, 0, 0, time.UTC), true},
		{"*/15 * * * *", time.Date(2026, 1, 5, 6, 31, 0, 0, time.UTC), false},
		{"0 0-6/2 * * *", time.Date(2026, 1, 5, 4, 0, 0, 0, time.UTC), true},
		{"0 0-6/2 * * *", time.Date(2026, 1, 5, 5, 0, 0, 0, time.UTC), false},
	}
	for _, c := range cases {
		got := matchesCron(strings.Fields(c.expr), c.t)
		if got != c.want {
			t.Errorf("matchesCron(%q, %v) = %v, want %v", c.expr, c.t, got, c.want)
		}
	}
}
