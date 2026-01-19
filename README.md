# elastic-integration-corpus-generator-tool

Command line tool for generating events corpus dynamically for Elastic integrations.

## Quick Start

```bash
# Install
go install github.com/elastic/elastic-integration-corpus-generator-tool@latest

# Generate events from a local template
elastic-integration-corpus-generator-tool local-template \
  --template ./template.ndjson \
  --fields-definition ./fields.yml \
  --config-file ./config.yml \
  --tot-events 1000

# Generate benchmark for an integration package
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/integrations/packages/aws \
  --data-stream billing
```

## Features

### Core Generation
- **`generate`** - Generate corpus from package registry
- **`generate-with-template`** - Generate corpus with remote template
- **`local-template`** - Generate corpus from local template files

### Integration Workflow Tools
- **`list-package`** - Explore integration packages and data streams
- **`generate-benchmark`** - Auto-generate Rally benchmark files from package fields
- **`generate-sample-event`** - Generate sample_event.json for data streams
- **`analyze-coverage`** - Analyze field coverage in templates
- **`validate-ecs`** - Validate fields against Elastic Common Schema

### Alerting Tools
- **`list-alerts`** - List alerting rule templates in a package
- **`generate-alert-data`** - Generate events that trigger alerting rules

## Documentation

| Document | Description |
|----------|-------------|
| [CLI Help](./docs/cli-help.md) | Complete command reference |
| [Usage Guide](./docs/usage.md) | Common use cases and examples |
| [Writing Templates](./docs/writing-templates.md) | Template creation guide |
| [Integration Workflow](./docs/integrations-workflow.md) | Tools for integration developers |
| [Performance](./docs/performances.md) | Optimization tips |
| [Fields Configuration](./docs/fields-configuration.md) | Configuration options |

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for development setup and guidelines.

## Maintainers

[Observability Integrations Team](https://github.com/orgs/elastic/teams/obs-infraobs-integrations)

## License

Elastic License 2.0
