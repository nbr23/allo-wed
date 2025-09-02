package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Extension struct {
	Ext         string `yaml:"ext"`
	Phone       string `yaml:"phone"`
	ContactName string `yaml:"contact_name"`
	Timezone    string `yaml:"timezone"`
	AllowedFrom string `yaml:"allowed_from"`
	AllowedUntil string `yaml:"allowed_until"`
	Language    string `yaml:"language"`
}

type Config struct {
	Extensions map[string]Extension `yaml:"extensions"`
}

func main() {
	var isAllowed = flag.Bool("is-allowed", false, "Check if call is allowed")
	var getLanguage = flag.Bool("language", false, "Get language")
	var getName = flag.Bool("name", false, "Get contact name")
	var getPhone = flag.Bool("phone", false, "Get phone number if allowed")
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


	allowed := isCallAllowed(person, *debug)

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

func isCallAllowed(ext Extension, debug bool) bool {
	if debug {
		fmt.Printf("DEBUG: Extension: %s\n", ext.Ext)
		fmt.Printf("DEBUG: Phone: %s\n", ext.Phone)
		fmt.Printf("DEBUG: Timezone: %s\n", ext.Timezone)
		fmt.Printf("DEBUG: Allowed from: %s\n", ext.AllowedFrom)
		fmt.Printf("DEBUG: Allowed until: %s\n", ext.AllowedUntil)
	}

	loc, err := time.LoadLocation(ext.Timezone)
	if err != nil {
		if debug {
			fmt.Printf("DEBUG: Error parsing timezone: %v\n", err)
		}
		return false
	}

	now := time.Now().In(loc)
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

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	fromTime := today.Add(time.Duration(from.Hour())*time.Hour + time.Duration(from.Minute())*time.Minute)
	untilTime := today.Add(time.Duration(until.Hour())*time.Hour + time.Duration(until.Minute())*time.Minute)

	if debug {
		fmt.Printf("DEBUG: Allowed from time: %s\n", fromTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("DEBUG: Allowed until time: %s\n", untilTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("DEBUG: Is after from time: %t\n", now.After(fromTime))
		fmt.Printf("DEBUG: Is before until time: %t\n", now.Before(untilTime))
	}

	allowed := now.After(fromTime) && now.Before(untilTime)
	if debug {
		fmt.Printf("DEBUG: Call allowed: %t\n", allowed)
	}

	return allowed
}
