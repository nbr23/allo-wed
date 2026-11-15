package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestConfigValidate(t *testing.T) {
	valid := Extension{Ext: "101", Phone: "+15555555555", Timezone: "America/New_York", AllowedFrom: "09:00", AllowedUntil: "17:00"}
	with := func(mutate func(*Extension)) Extension {
		e := valid
		mutate(&e)
		return e
	}
	single := func(e Extension) Config {
		return Config{Extensions: map[string]Extension{"101": e}}
	}

	tests := []struct {
		name   string
		config Config
		want   []string
	}{
		{"valid", single(valid), nil},
		{"no extensions", Config{}, nil},
		{"valid caller timezone", Config{CallerTimezone: "Europe/Paris", Extensions: map[string]Extension{"101": valid}}, nil},
		{"midnight wrap window", single(with(func(e *Extension) { e.AllowedFrom, e.AllowedUntil = "22:00", "02:00" })), nil},

		{"unknown caller timezone", Config{CallerTimezone: "Mars/Olympus_Mons"}, []string{"caller_timezone: unknown time zone Mars/Olympus_Mons"}},
		{"empty ext", single(with(func(e *Extension) { e.Ext = "" })), []string{`extension "101": ext is empty`}},
		{"empty timezone", single(with(func(e *Extension) { e.Timezone = "" })), []string{`extension "101": timezone is empty`}},
		{"unknown timezone", single(with(func(e *Extension) { e.Timezone = "Mars/Olympus_Mons" })), []string{`extension "101": timezone: unknown time zone Mars/Olympus_Mons`}},
		{"malformed allowed_from", single(with(func(e *Extension) { e.AllowedFrom = "9am" })), []string{`extension "101": allowed_from: `}},
		{"empty allowed_from", single(with(func(e *Extension) { e.AllowedFrom = "" })), []string{`extension "101": allowed_from: `}},
		{"malformed allowed_until", single(with(func(e *Extension) { e.AllowedUntil = "25:00" })), []string{`extension "101": allowed_until: `}},
		{
			"duplicate ext",
			Config{Extensions: map[string]Extension{"grandma": valid, "grandpa": valid}},
			[]string{`ext "101" is used by extensions "grandma" and "grandpa"`},
		},
		{
			"every problem is reported",
			Config{
				CallerTimezone: "Nowhere/Land",
				Extensions: map[string]Extension{
					"101": with(func(e *Extension) { e.Timezone = "" }),
					"102": with(func(e *Extension) { e.AllowedFrom, e.AllowedUntil = "morning", "evening" }),
				},
			},
			[]string{
				"caller_timezone: unknown time zone Nowhere/Land",
				`extension "101": timezone is empty`,
				`extension "102": allowed_from: `,
				`extension "102": allowed_until: `,
				`ext "101" is used by extensions "101" and "102"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate() = nil, want errors containing %q", tt.want)
			}

			lines := strings.Split(err.Error(), "\n")
			if len(lines) != len(tt.want) {
				t.Fatalf("validate() reported %d problems, want %d:\n%v", len(lines), len(tt.want), err)
			}
			for i, want := range tt.want {
				if !strings.HasPrefix(lines[i], want) {
					t.Errorf("problem %d = %q, want prefix %q", i, lines[i], want)
				}
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	entry := `    ext: "%s"
    phone: "+15555555555"
    timezone: "Europe/Paris"
    allowed_from: "09:00"
    allowed_until: "17:00"
`
	block := func(key, ext string) string {
		return `  "` + key + `":` + "\n" + strings.Replace(entry, "%s", ext, 1)
	}

	t.Run("sample config", func(t *testing.T) {
		config, err := loadConfig("config.sample.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if len(config.Extensions) == 0 {
			t.Fatal("sample config has no extensions")
		}
	})

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"valid", "extensions:\n" + block("101", "101") + block("102", "102"), ""},
		{"duplicate key", "extensions:\n" + block("101", "101") + block("101", "102"), `key "101" already set in map`},
		{"unknown field", "extensions:\n" + block("101", "101") + "    allowed_untill: \"18:00\"\n", "field allowed_untill not found"},
		{"invalid entry", "extensions:\n" + block("101", "101") + block("102", "101"), `ext "101" is used by extensions "101" and "102"`},
		{"not yaml", "extensions: [", "yaml: "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(write(t, tt.content))
			if tt.want == "" {
				if err != nil {
					t.Fatalf("loadConfig() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("loadConfig() = %v, want error containing %q", err, tt.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		if _, err := loadConfig(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
			t.Fatal("loadConfig() = nil, want error")
		}
	})
}

func TestNextAllowedStart(t *testing.T) {
	newYork := mustLoadLocation(t, "America/New_York")

	from := func(tz, allowedFrom string) Extension {
		return Extension{Ext: "101", Timezone: tz, AllowedFrom: allowedFrom, AllowedUntil: "17:00"}
	}
	morning := from("America/New_York", "09:00")

	at := func(month time.Month, d, hour, minute int) time.Time {
		return time.Date(2026, month, d, hour, minute, 0, 0, newYork)
	}

	tests := []struct {
		name string
		ext  Extension
		now  time.Time
		want time.Time
	}{
		{"before from is today", morning, at(time.January, 15, 8, 0), at(time.January, 15, 9, 0)},
		{"exactly at from is tomorrow", morning, at(time.January, 15, 9, 0), at(time.January, 16, 9, 0)},
		{"inside window is tomorrow", morning, at(time.January, 15, 12, 0), at(time.January, 16, 9, 0)},
		{"after window is tomorrow", morning, at(time.January, 15, 20, 0), at(time.January, 16, 9, 0)},
		{"month end rolls over", morning, at(time.January, 31, 12, 0), at(time.February, 1, 9, 0)},
		{"year end rolls over", morning, at(time.December, 31, 12, 0), time.Date(2027, time.January, 1, 9, 0, 0, 0, newYork)},
		{"utc clock before from", morning, time.Date(2026, time.January, 15, 13, 30, 0, 0, time.UTC), at(time.January, 15, 9, 0)},
		{"utc clock on the next utc day", morning, time.Date(2026, time.January, 16, 2, 0, 0, 0, time.UTC), at(time.January, 16, 9, 0)},
		{"evening before spring forward", morning, at(time.March, 7, 12, 0), at(time.March, 8, 9, 0)},
		{"evening before fall back", morning, at(time.October, 31, 12, 0), at(time.November, 1, 9, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextAllowedStart(tt.ext, tt.now)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("nextAllowedStart(from %s, %s) = %s, want %s",
					tt.ext.AllowedFrom, tt.now.Format(time.RFC3339), got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
			if got.Location().String() != tt.ext.Timezone {
				t.Errorf("nextAllowedStart() location = %s, want %s", got.Location(), tt.ext.Timezone)
			}
		})
	}

	failures := []struct {
		name string
		ext  Extension
	}{
		{"unknown timezone", from("Mars/Olympus_Mons", "09:00")},
		{"malformed allowed_from", from("America/New_York", "9am")},
	}

	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := nextAllowedStart(tt.ext, at(time.January, 15, 12, 0)); err == nil {
				t.Errorf("nextAllowedStart() = %s, want error", got.Format(time.RFC3339))
			}
		})
	}
}

func TestRoundUpToQuarter(t *testing.T) {
	kathmandu := mustLoadLocation(t, "Asia/Kathmandu")

	utc := func(d, hour, minute, second int) time.Time {
		return time.Date(2026, time.January, d, hour, minute, second, 0, time.UTC)
	}

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"on the hour", utc(15, 9, 0, 0), utc(15, 9, 0, 0)},
		{"on a quarter", utc(15, 9, 45, 0), utc(15, 9, 45, 0)},
		{"one minute past", utc(15, 9, 1, 0), utc(15, 9, 15, 0)},
		{"one minute short", utc(15, 9, 14, 0), utc(15, 9, 15, 0)},
		{"rolls to the next hour", utc(15, 9, 46, 0), utc(15, 10, 0, 0)},
		{"rolls to the next day", utc(15, 23, 50, 0), utc(16, 0, 0, 0)},
		{"seconds are dropped", utc(15, 9, 0, 30), utc(15, 9, 0, 0)},
		{
			"quarters follow the local clock",
			time.Date(2026, time.January, 15, 9, 20, 0, 0, kathmandu),
			time.Date(2026, time.January, 15, 9, 30, 0, 0, kathmandu),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roundUpToQuarter(tt.in); !got.Equal(tt.want) {
				t.Errorf("roundUpToQuarter(%s) = %s, want %s",
					tt.in.Format(time.RFC3339), got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
		})
	}
}

func TestNextAllowedForCaller(t *testing.T) {
	newYork := mustLoadLocation(t, "America/New_York")

	from := func(tz, allowedFrom string) Extension {
		return Extension{Ext: "101", Timezone: tz, AllowedFrom: allowedFrom, AllowedUntil: "17:00"}
	}
	morning := from("America/New_York", "09:00")
	winterEvening := time.Date(2026, time.January, 15, 20, 0, 0, 0, newYork)

	tests := []struct {
		name   string
		ext    Extension
		caller string
		now    time.Time
		want   string
	}{
		{"converted to the caller's timezone", morning, "Europe/Paris", winterEvening, "1500"},
		{"same timezone", morning, "America/New_York", winterEvening, "0900"},
		{"only the contact is on dst", morning, "Europe/Paris", time.Date(2026, time.March, 10, 20, 0, 0, 0, newYork), "1400"},
		{"rounded up to the quarter", from("America/New_York", "09:10"), "Europe/Paris", winterEvening, "1515"},
		{"rounded past midnight", from("America/New_York", "17:50"), "Europe/Paris", winterEvening, "0000"},
		{"45 minute offset contact", from("Asia/Kathmandu", "09:00"), "Europe/Paris", winterEvening, "0415"},
		{"45 minute offset caller", morning, "Asia/Kathmandu", winterEvening, "1945"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextAllowedForCaller(tt.ext, tt.caller, tt.now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("nextAllowedForCaller(from %s %q, %q) = %q, want %q",
					tt.ext.AllowedFrom, tt.ext.Timezone, tt.caller, got, tt.want)
			}
		})
	}

	failures := []struct {
		name   string
		ext    Extension
		caller string
	}{
		{"caller timezone not set", morning, ""},
		{"unknown caller timezone", morning, "Mars/Olympus_Mons"},
		{"unknown contact timezone", from("Mars/Olympus_Mons", "09:00"), "Europe/Paris"},
		{"malformed allowed_from", from("America/New_York", "9am"), "Europe/Paris"},
	}

	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := nextAllowedForCaller(tt.ext, tt.caller, winterEvening); err == nil {
				t.Errorf("nextAllowedForCaller() = %q, want error", got)
			}
		})
	}
}

func TestNextAllowedDayForCaller(t *testing.T) {
	newYork := mustLoadLocation(t, "America/New_York")

	from := func(tz, allowedFrom string) Extension {
		return Extension{Ext: "101", Timezone: tz, AllowedFrom: allowedFrom, AllowedUntil: "20:00"}
	}
	brazil := from("America/Sao_Paulo", "09:00")
	at := func(hour, minute int) time.Time {
		return time.Date(2026, time.November, 15, hour, minute, 0, 0, newYork)
	}

	tests := []struct {
		name   string
		ext    Extension
		caller string
		now    time.Time
		want   string
	}{
		{"evening call", brazil, "America/New_York", at(21, 0), "tomorrow"},
		{"early morning call", brazil, "America/New_York", at(6, 0), "today"},
		{"after midnight call", brazil, "America/New_York", at(1, 0), "today"},
		{"rounded past midnight", from("America/New_York", "23:50"), "America/New_York", at(23, 55), "tomorrow"},
		{"contact already past midnight", brazil, "America/New_York", at(23, 0), "tomorrow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextAllowedDayForCaller(tt.ext, tt.caller, tt.now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("nextAllowedDayForCaller(from %s %q, %q, %s) = %q, want %q",
					tt.ext.AllowedFrom, tt.ext.Timezone, tt.caller, tt.now.Format(clockLayout), got, tt.want)
			}
		})
	}

	if got, err := nextAllowedDayForCaller(brazil, "", at(21, 0)); err == nil {
		t.Errorf("nextAllowedDayForCaller() without caller timezone = %q, want error", got)
	}
}
