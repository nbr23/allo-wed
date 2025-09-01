# Allô-wed

A simple Go utility to check if phone calls to extensions are allowed based on timezone and time restrictions.

## Usage

```bash
./allo-wed -config config.yaml [flags] <extension>
```

### Flags

- `-config`: Path to YAML config file (required)
- `-is-allowed`: Check if call is currently allowed (returns true/false)
- `-language`: Get the language for the extension
- `-phone`: Get phone number (always returns phone number unless combined with -is-allowed)
- `-playback`: Get playback name for specified playback type
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

# Get playback name for "not-available" type
./allo-wed -config config.yaml -playback not-available 101

# Debug time calculations for extension 101
./allo-wed -config config.yaml -debug -is-allowed 101
```

## Config Format

The config file is a YAML file with extension definitions. Each extension can optionally include audio playback definitions:

```yaml
extensions:
  "101":
    ext: "101"
    phone: "+1-555-0101"
    timezone: "America/New_York"
    allowed_from: "09:00"
    allowed_until: "17:00"
    language: "en"
    audio_playbacks:
      not-available: "not-available"
      busy: "busy"
      offline: "offline"
```

See `config.sample.yaml` for a complete example.
