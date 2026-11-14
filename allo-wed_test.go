package main

import (
	"testing"
	"time"
)

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestIsCallAllowed(t *testing.T) {
	newYork := mustLoadLocation(t, "America/New_York")

	window := func(tz, from, until string) Extension {
		return Extension{Ext: "101", Timezone: tz, AllowedFrom: from, AllowedUntil: until}
	}
	day := window("America/New_York", "09:00", "17:00")
	night := window("America/New_York", "22:00", "02:00")

	at := func(month time.Month, d, hour, minute int) time.Time {
		return time.Date(2026, month, d, hour, minute, 0, 0, newYork)
	}

	tests := []struct {
		name string
		ext  Extension
		now  time.Time
		want bool
	}{
		{"inside window", day, at(time.January, 15, 12, 0), true},
		{"before window", day, at(time.January, 15, 8, 59), false},
		{"after window", day, at(time.January, 15, 17, 1), false},
		{"from is inclusive", day, at(time.January, 15, 9, 0), true},
		{"until is exclusive", day, at(time.January, 15, 17, 0), false},
		{"last minute of window", day, at(time.January, 15, 16, 59), true},

		{"utc clock inside window", day, time.Date(2026, time.January, 15, 14, 30, 0, 0, time.UTC), true},
		{"utc clock before window", day, time.Date(2026, time.January, 15, 13, 30, 0, 0, time.UTC), false},

		{"midnight wrap before midnight", night, at(time.January, 15, 23, 0), true},
		{"midnight wrap after midnight", night, at(time.January, 15, 1, 0), true},
		{"midnight wrap from is inclusive", night, at(time.January, 15, 22, 0), true},
		{"midnight wrap until is exclusive", night, at(time.January, 15, 2, 0), false},
		{"midnight wrap midday", night, at(time.January, 15, 12, 0), false},

		{"spring forward after from", day, at(time.March, 8, 9, 30), true},
		{"spring forward after until", day, at(time.March, 8, 17, 30), false},
		{"fall back before from", day, at(time.November, 1, 8, 30), false},
		{"fall back before until", day, at(time.November, 1, 16, 30), true},

		{"empty timezone", window("", "00:00", "23:59"), at(time.January, 15, 12, 0), false},
		{"unknown timezone", window("Mars/Olympus_Mons", "00:00", "23:59"), at(time.January, 15, 12, 0), false},
		{"malformed allowed_from", window("America/New_York", "9am", "17:00"), at(time.January, 15, 12, 0), false},
		{"malformed allowed_until", window("America/New_York", "09:00", "25:00"), at(time.January, 15, 12, 0), false},
		{"empty window", window("America/New_York", "09:00", "09:00"), at(time.January, 15, 9, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCallAllowed(tt.ext, tt.now, false); got != tt.want {
				t.Errorf("isCallAllowed(%s-%s %q, %s) = %t, want %t",
					tt.ext.AllowedFrom, tt.ext.AllowedUntil, tt.ext.Timezone, tt.now.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}
