package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"time"
	_ "time/tzdata"

	"gopkg.in/yaml.v2"
)

type Extension struct {
	Ext          string `yaml:"ext"`
	Phone        string `yaml:"phone"`
	ContactName  string `yaml:"contact_name"`
	Timezone     string `yaml:"timezone"`
	AllowedFrom  string `yaml:"allowed_from"`
	AllowedUntil string `yaml:"allowed_until"`
	Language     string `yaml:"language"`
}

type Config struct {
	CallerTimezone string               `yaml:"caller_timezone"`
	Extensions     map[string]Extension `yaml:"extensions"`
}

const clockLayout = "15:04"

func (e Extension) validate() []error {
	var errs []error

	if e.Ext == "" {
		errs = append(errs, errors.New("ext is empty"))
	}

	if e.Timezone == "" {
		errs = append(errs, errors.New("timezone is empty"))
	} else if _, err := time.LoadLocation(e.Timezone); err != nil {
		errs = append(errs, fmt.Errorf("timezone: %w", err))
	}

	if _, err := time.Parse(clockLayout, e.AllowedFrom); err != nil {
		errs = append(errs, fmt.Errorf("allowed_from: %w", err))
	}

	if _, err := time.Parse(clockLayout, e.AllowedUntil); err != nil {
		errs = append(errs, fmt.Errorf("allowed_until: %w", err))
	}

	return errs
}

func (c Config) validate() error {
	var errs []error

	if c.CallerTimezone != "" {
		if _, err := time.LoadLocation(c.CallerTimezone); err != nil {
			errs = append(errs, fmt.Errorf("caller_timezone: %w", err))
		}
	}

	keyByExt := map[string]string{}
	for _, key := range slices.Sorted(maps.Keys(c.Extensions)) {
		e := c.Extensions[key]
		for _, err := range e.validate() {
			errs = append(errs, fmt.Errorf("extension %q: %w", key, err))
		}

		if e.Ext == "" {
			continue
		}
		if first, ok := keyByExt[e.Ext]; ok {
			errs = append(errs, fmt.Errorf("ext %q is used by extensions %q and %q", e.Ext, first, key))
			continue
		}
		keyByExt[e.Ext] = key
	}

	return errors.Join(errs...)
}

func loadConfig(path string) (Config, error) {
	var config Config

	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}

	if err := yaml.UnmarshalStrict(data, &config); err != nil {
		return config, err
	}

	return config, config.validate()
}

func main() {
	var isAllowed = flag.Bool("is-allowed", false, "Check if call is allowed")
	var getLanguage = flag.Bool("language", false, "Get language")
	var getName = flag.Bool("name", false, "Get contact name")
	var getPhone = flag.Bool("phone", false, "Get phone number if allowed")
	var getNextAllowed = flag.Bool("next-allowed", false, "Get the next time calls are allowed, as HHMM in the caller's timezone")
	var getNextAllowedDay = flag.Bool("next-allowed-day", false, "Get whether the next time calls are allowed is today or tomorrow in the caller's timezone")
	var validate = flag.Bool("validate", false, "Check the config file and exit, without looking up an extension")
	var debug = flag.Bool("debug", false, "Print debug information")
	var configPath = flag.String("config", "", "Path to the config file")
	flag.Parse()

	args := flag.Args()
	wantArgs := 1
	if *validate {
		wantArgs = 0
	}
	hasMode := *isAllowed || *getLanguage || *getName || *getPhone || *getNextAllowed || *getNextAllowedDay || *validate
	if !hasMode || len(args) != wantArgs {
		log.Fatal("Usage: program -config <path> <-is-allowed|-phone|-language|-name|-next-allowed|-next-allowed-day> [-debug] <extension>\n       program -config <path> -validate")
	}

	if *configPath == "" {
		log.Fatal("a config file path is required")
	}

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	if *validate {
		return
	}

	ext := args[0]

	var person Extension
	var found bool
	for _, e := range config.Extensions {
		if e.Ext == ext {
			person = e
			found = true
			break
		}
	}

	if !found {
		log.Fatal("Extension not found")
	}

	if *getLanguage {
		fmt.Print(person.Language)
		return
	}

	if *getName {
		fmt.Print(person.ContactName)
		return
	}

	if *getNextAllowed {
		next, err := nextAllowedForCaller(person, config.CallerTimezone, time.Now())
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(next)
		return
	}

	if *getNextAllowedDay {
		day, err := nextAllowedDayForCaller(person, config.CallerTimezone, time.Now())
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(day)
		return
	}

	allowed := isCallAllowed(person, time.Now(), *debug)

	if *isAllowed && *getPhone {
		if allowed {
			fmt.Print(person.Phone)
		}
		return
	}

	if *isAllowed {
		if allowed {
			fmt.Print("true")
		} else {
			fmt.Print("false")
		}
		return
	}

	if *getPhone {
		fmt.Print(person.Phone)
		return
	}
}

func isCallAllowed(ext Extension, now time.Time, debug bool) bool {
	debugf := func(format string, args ...any) {
		if debug {
			fmt.Fprintf(os.Stderr, "DEBUG: "+format+"\n", args...)
		}
	}

	debugf("Extension: %s", ext.Ext)
	debugf("Phone: %s", ext.Phone)
	debugf("Timezone: %s", ext.Timezone)
	debugf("Allowed from: %s", ext.AllowedFrom)
	debugf("Allowed until: %s", ext.AllowedUntil)

	if ext.Timezone == "" {
		debugf("Error parsing timezone: timezone is empty")
		return false
	}

	loc, err := time.LoadLocation(ext.Timezone)
	if err != nil {
		debugf("Error parsing timezone: %v", err)
		return false
	}

	now = now.In(loc)
	debugf("Current time in %s: %s", ext.Timezone, now.Format("2006-01-02 15:04:05"))

	from, err := time.Parse(clockLayout, ext.AllowedFrom)
	if err != nil {
		debugf("Error parsing allowed_from time: %v", err)
		return false
	}

	until, err := time.Parse(clockLayout, ext.AllowedUntil)
	if err != nil {
		debugf("Error parsing allowed_until time: %v", err)
		return false
	}

	fromMinutes := minuteOfDay(from)
	untilMinutes := minuteOfDay(until)
	nowMinutes := minuteOfDay(now)

	afterFrom := nowMinutes >= fromMinutes
	beforeUntil := nowMinutes < untilMinutes

	debugf("Allowed from time: %s", from.Format(clockLayout))
	debugf("Allowed until time: %s", until.Format(clockLayout))
	debugf("Is after from time: %t", afterFrom)
	debugf("Is before until time: %t", beforeUntil)

	allowed := afterFrom && beforeUntil
	if fromMinutes > untilMinutes {
		allowed = afterFrom || beforeUntil
	}
	debugf("Call allowed: %t", allowed)

	return allowed
}

func nextAllowedStart(ext Extension, now time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(ext.Timezone)
	if err != nil {
		return time.Time{}, err
	}

	from, err := time.Parse(clockLayout, ext.AllowedFrom)
	if err != nil {
		return time.Time{}, err
	}

	now = now.In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), from.Hour(), from.Minute(), 0, 0, loc)
	if !start.After(now) {
		start = time.Date(now.Year(), now.Month(), now.Day()+1, from.Hour(), from.Minute(), 0, 0, loc)
	}

	return start, nil
}

func roundUpToQuarter(t time.Time) time.Time {
	t = t.Truncate(time.Minute)
	if rem := t.Minute() % 15; rem != 0 {
		t = t.Add(time.Duration(15-rem) * time.Minute)
	}
	return t
}

func nextAllowedInCallerTZ(ext Extension, callerTimezone string, now time.Time) (time.Time, error) {
	if callerTimezone == "" {
		return time.Time{}, errors.New("caller_timezone is not set")
	}

	callerLoc, err := time.LoadLocation(callerTimezone)
	if err != nil {
		return time.Time{}, err
	}

	start, err := nextAllowedStart(ext, now)
	if err != nil {
		return time.Time{}, err
	}

	return roundUpToQuarter(start.In(callerLoc)), nil
}

func nextAllowedForCaller(ext Extension, callerTimezone string, now time.Time) (string, error) {
	next, err := nextAllowedInCallerTZ(ext, callerTimezone, now)
	if err != nil {
		return "", err
	}

	return next.Format("1504"), nil
}

func nextAllowedDayForCaller(ext Extension, callerTimezone string, now time.Time) (string, error) {
	next, err := nextAllowedInCallerTZ(ext, callerTimezone, now)
	if err != nil {
		return "", err
	}

	now = now.In(next.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	nextDay := time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, time.UTC)
	if nextDay.After(today) {
		return "tomorrow", nil
	}

	return "today", nil
}

func minuteOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
