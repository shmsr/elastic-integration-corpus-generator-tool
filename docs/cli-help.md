# CLI Reference

Complete command reference for the elastic-integration-corpus-generator-tool.

## Global Usage

```bash
elastic-integration-corpus-generator-tool [command] [flags]
```

Run `elastic-integration-corpus-generator-tool --help` for available commands.

Run `elastic-integration-corpus-generator-tool <command> --help` for command-specific help.

---

## Commands Overview

### Core Generation Commands

| Command | Description |
|---------|-------------|
| `generate` | Generate corpus from Elastic package registry |
| `generate-with-template` | Generate corpus with custom template and fields |
| `local-template` | Generate corpus from local asset templates |

### Integration Workflow Commands

| Command | Description |
|---------|-------------|
| `list-package` | Explore integration packages and data streams |
| `generate-benchmark` | Auto-generate Rally benchmark files |
| `generate-sample-event` | Generate sample_event.json for data streams |
| `analyze-coverage` | Analyze field coverage in templates |
| `validate-ecs` | Validate fields against ECS |

### Alerting Commands

| Command | Description |
|---------|-------------|
| `list-alerts` | List alerting rule templates in a package |
| `generate-alert-data` | Generate events that trigger alerting rules |

### Utility Commands

| Command | Description |
|---------|-------------|
| `version` | Display version information |
| `completion` | Generate shell completion scripts |

---

## Core Commands

### generate

Generate corpus from the Elastic package registry.

```bash
elastic-integration-corpus-generator-tool generate \
  --package <name> \
  --dataset <name> \
  --version <version> \
  --tot-events 1000
```

### generate-with-template

Generate corpus using custom template and fields files.

```bash
elastic-integration-corpus-generator-tool generate-with-template \
  ./template.ndjson \
  ./fields.yml \
  --config-file ./config.yml \
  --template-type gotext \
  --tot-events 1000
```

### local-template

Generate corpus from templates in the assets/templates directory.

```bash
elastic-integration-corpus-generator-tool local-template \
  <package> <dataset> \
  --config-file ./config.yml \
  --tot-events 1000
```

---

## Integration Workflow Commands

### list-package

Explore integration packages and discover data streams.

```bash
# List all data streams
elastic-integration-corpus-generator-tool list-package \
  --package-path /path/to/packages/aws

# Show field details
elastic-integration-corpus-generator-tool list-package \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --fields

# JSON output
elastic-integration-corpus-generator-tool list-package \
  --package-path /path/to/packages/aws \
  --json
```

### generate-benchmark

Auto-generate Rally benchmark files from package fields.

```bash
# Single data stream
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/packages/aws \
  --data-stream billing

# All data streams
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/packages/aws \
  --all

# Write to package directory
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/packages/aws \
  --all \
  --output-mode package
```

### generate-sample-event

Generate sample_event.json for integration packages.

```bash
# Print to stdout
elastic-integration-corpus-generator-tool generate-sample-event \
  --package-path /path/to/packages/aws \
  --data-stream billing

# Write to file
elastic-integration-corpus-generator-tool generate-sample-event \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --output ./sample_event.json

# Write to package directory
elastic-integration-corpus-generator-tool generate-sample-event \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --output-mode package

# From existing benchmark files
elastic-integration-corpus-generator-tool generate-sample-event \
  --template ./template.ndjson \
  --fields ./fields.yml \
  --config ./config.yml
```

### analyze-coverage

Analyze field coverage in benchmark templates.

```bash
# Analyze template
elastic-integration-corpus-generator-tool analyze-coverage \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --template ./template.ndjson

# Analyze sample event
elastic-integration-corpus-generator-tool analyze-coverage \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --sample-event ./sample_event.json
```

### validate-ecs

Validate package fields against Elastic Common Schema.

```bash
# Validate single data stream
elastic-integration-corpus-generator-tool validate-ecs \
  --package-path /path/to/packages/aws \
  --data-stream billing

# Validate all data streams
elastic-integration-corpus-generator-tool validate-ecs \
  --package-path /path/to/packages/aws \
  --all
```

---

## Alerting Commands

### list-alerts

List alerting rule templates in a package.

```bash
# List all alerting rules
elastic-integration-corpus-generator-tool list-alerts \
  --package-path /path/to/packages/mongodb

# Show the ES|QL queries
elastic-integration-corpus-generator-tool list-alerts \
  --package-path /path/to/packages/mongodb \
  --show-query

# JSON output
elastic-integration-corpus-generator-tool list-alerts \
  --package-path /path/to/packages/aws \
  --json
```

### generate-alert-data

Generate events that trigger (or don't trigger) alerting rules.

```bash
# Generate events that TRIGGER the alert
elastic-integration-corpus-generator-tool generate-alert-data \
  --package-path /path/to/packages/mongodb \
  --rule-id mongodb-cache-usage-high \
  --trigger

# Generate safe events (won't trigger)
elastic-integration-corpus-generator-tool generate-alert-data \
  --package-path /path/to/packages/mongodb \
  --rule-id mongodb-cache-usage-high

# Generate multiple events to file
elastic-integration-corpus-generator-tool generate-alert-data \
  --package-path /path/to/packages/aws \
  --rule-id aws-ec2-high-cpu-utilization \
  --trigger \
  --num-events 10 \
  --output ./alert-test-data.ndjson
```

---

## Output Location

Generated corpus files are saved to:

- **macOS**: `~/Library/Application Support/elastic-integration-corpus-generator-tool/corpora/`
- **Linux**: `~/.local/share/elastic-integration-corpus-generator-tool/corpora/`

---

## Need Help?

- Open an issue in this repository
- Check [Integration Workflow Guide](./integrations-workflow.md) for detailed documentation
- See [Usage Guide](./usage.md) for common use cases
