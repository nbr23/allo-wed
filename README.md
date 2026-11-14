# Allô-wed

A simple Go utility to check if phone calls to extensions are allowed based on timezone and time restrictions.

This tool is part of the [telefonefix](https://wip.tf/posts/telefonefix-building-babys-first-international-landline/) project.

## Usage

```bash
./allo-wed -config config.yaml [flags] <extension>
```

### Flags

- `-config`: Path to YAML config file (required)
- `-is-allowed`: Check if call is currently allowed (returns true/false)
- `-language`: Get the language for the extension
- `-name`: Get the contact name for the extension
- `-phone`: Get phone number (always returns phone number unless combined with -is-allowed)
- `-next-allowed`: Get the next time calls to the extension are allowed, as `HHMM` in `caller_timezone`, rounded up to the quarter hour. Prints nothing and exits non-zero if `caller_timezone` is not set
- `-debug`: Print debug information about time calculations

### Examples

```bash
# Check if calls to extension 101 are allowed right now
./allo-wed -config config.yaml -is-allowed 101

# Get phone number for extension 102
./allo-wed -config config.yaml -phone 102

# Get phone number for extension 102 only if call is allowed right now
./allo-wed -config config.yaml -is-allowed -phone 102

# Get language for extension 103
./allo-wed -config config.yaml -language 103

# Get contact name for extension 101
./allo-wed -config config.yaml -name 101

# Get the next time calls to extension 101 are allowed, in the caller's timezone (e.g. 1500)
./allo-wed -config config.yaml -next-allowed 101

# Debug time calculations for extension 101
./allo-wed -config config.yaml -debug -is-allowed 101
```

## Config Format

The config file is a YAML file with extension definitions:

```yaml
caller_timezone: "Europe/Paris"

extensions:
  "101":
    ext: "101"
    phone: "+1-555-0101"
    contact_name: "John Doe"
    timezone: "America/New_York"
    allowed_from: "09:00"
    allowed_until: "17:00"
    language: "en"
```

`caller_timezone` is optional and only used by `-next-allowed`: it is the timezone of the person placing the calls, so the next allowed time can be reported in their local time rather than the contact's.

See `config.sample.yaml` for a complete example.
