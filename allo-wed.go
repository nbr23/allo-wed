package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
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

func main() {
	var isAllowed = flag.Bool("is-allowed", false, "Check if call is allowed")
	var getLanguage = flag.Bool("language", false, "Get language")
	var getName = flag.Bool("name", false, "Get contact name")
	var getPhone = flag.Bool("phone", false, "Get phone number if allowed")
	var getNextAllowed = flag.Bool("next-allowed", false, "Get the next time calls are allowed, as HHMM in the caller's timezone")
	var debug = flag.Bool("debug", false, "Print debug information")
	var configPath = flag.String("config", "", "Path to the config file")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		log.Fatal("Usage: program [flags] <extension>")
	}

	if *configPath == "" {
		log.Fatal("a config file path is required")
	}

	ext := args[0]

	data, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatal(err)
	}

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
	if debug {
		fmt.Printf("DEBUG: Extension: %s\n", ext.Ext)
		fmt.Printf("DEBUG: Phone: %s\n", ext.Phone)
		fmt.Printf("DEBUG: Timezone: %s\n", ext.Timezone)
		fmt.Printf("DEBUG: Allowed from: %s\n", ext.AllowedFrom)
		fmt.Printf("DEBUG: Allowed until: %s\n", ext.AllowedUntil)
	}

	if ext.Timezone == "" {
		if debug {
			fmt.Printf("DEBUG: Error parsing timezone: timezone is empty\n")
		}
		return false
	}

	loc, err := time.LoadLocation(ext.Timezone)
	if err != nil {
		if debug {
			fmt.Printf("DEBUG: Error parsing timezone: %v\n", err)
		}
		return false
	}

	now = now.In(loc)
	if debug {
		fmt.Printf("DEBUG: Current time in %s: %s\n", ext.Timezone, now.Format("2006-01-02 15:04:05"))
	}

	from, err := time.Parse("15:04", ext.AllowedFrom)
	if err != nil {
		if debug {
			fmt.Printf("DEBUG: Error parsing allowed_from time: %v\n", err)
		}
		return false
	}

	until, err := time.Parse("15:04", ext.AllowedUntil)
	if err != nil {
		if debug {
			fmt.Printf("DEBUG: Error parsing allowed_until time: %v\n", err)
		}
		return false
	}

	fromMinutes := minuteOfDay(from)
	untilMinutes := minuteOfDay(until)
	nowMinutes := minuteOfDay(now)

	afterFrom := nowMinutes >= fromMinutes
	beforeUntil := nowMinutes < untilMinutes

	if debug {
		fmt.Printf("DEBUG: Allowed from time: %s\n", from.Format("15:04"))
		fmt.Printf("DEBUG: Allowed until time: %s\n", until.Format("15:04"))
		fmt.Printf("DEBUG: Is after from time: %t\n", afterFrom)
		fmt.Printf("DEBUG: Is before until time: %t\n", beforeUntil)
	}

	allowed := afterFrom && beforeUntil
	if fromMinutes > untilMinutes {
		allowed = afterFrom || beforeUntil
	}
	if debug {
		fmt.Printf("DEBUG: Call allowed: %t\n", allowed)
	}

	return allowed
}

func nextAllowedStart(ext Extension, now time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(ext.Timezone)
	if err != nil {
		return time.Time{}, err
	}

	from, err := time.Parse("15:04", ext.AllowedFrom)
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

func nextAllowedForCaller(ext Extension, callerTimezone string, now time.Time) (string, error) {
	if callerTimezone == "" {
		return "", errors.New("caller_timezone is not set")
	}

	callerLoc, err := time.LoadLocation(callerTimezone)
	if err != nil {
		return "", err
	}

	start, err := nextAllowedStart(ext, now)
	if err != nil {
		return "", err
	}

	return roundUpToQuarter(start.In(callerLoc)).Format("1504"), nil
}

func minuteOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
