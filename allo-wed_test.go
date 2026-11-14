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
