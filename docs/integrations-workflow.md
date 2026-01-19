# Integration Workflow Tools

This document describes the tools available for working with Elastic integration packages from the `elastic/integrations` repository.

## Overview

The corpus generator provides several commands specifically designed to help integration developers:

| Command | Description |
|---------|-------------|
| `list-package` | Explore integration packages and their data streams |
| `generate-benchmark` | Auto-generate Rally benchmark files |
| `generate-sample-event` | Generate sample_event.json for a data stream |
| `analyze-coverage` | Analyze field coverage in templates |
| `validate-ecs` | Validate fields against ECS |

---

## list-package

Explore integration packages and discover available data streams.

### Usage

```bash
# List all data streams in a package
elastic-integration-corpus-generator-tool list-package \
  --package-path /path/to/packages/aws

# Show field details for a specific data stream
elastic-integration-corpus-generator-tool list-package \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --fields

# Output as JSON
elastic-integration-corpus-generator-tool list-package \
  --package-path /path/to/packages/kubernetes \
  --json
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--package-path` | `-p` | Path to the integration package (required) |
| `--data-stream` | `-d` | Show details for a specific data stream |
| `--fields` | `-f` | Show field details |
| `--json` | | Output as JSON |

### Example Output

```
📦 Package Information
==================================================
Name:    aws
Title:   AWS
Version: 5.6.1
Owner:   elastic/obs-ds-hosted-services

📊 Data Streams (45 total)
----------------------------------------
  • billing (34 fields)
  • dynamodb (50 fields)
  • ec2_metrics (53 fields)
  ...
```

---

## generate-benchmark

Automatically generate Rally benchmark files from an integration package's field definitions. This saves significant time compared to manually creating benchmark templates.

### Usage

```bash
# Generate benchmark for a specific data stream
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/packages/aws \
  --data-stream dynamodb

# Generate benchmarks for all data streams
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/packages/kubernetes \
  --all

# Always write to the package's _dev/benchmark/rally directory
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/packages/kubernetes \
  --all \
  --output-mode package

# Custom output directory and event count
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --output-dir ./benchmarks \
  --total-events 50000
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--package-path` | `-p` | Path to the integration package (required) | |
| `--data-stream` | `-d` | Data stream name(s) to generate benchmark for | |
| `--output-dir` | `-o` | Output directory | `<package>/_dev/benchmark/rally/` |
| `--output-mode` | | Output mode: `flat` or `package` | `flat` |
| `--all` | `-a` | Generate benchmarks for all data streams | `false` |
| `--total-events` | `-n` | Total events to generate in benchmark | `20000` |

### Generated Files

For each data stream, the command generates:

1. **`<datastream>-benchmark.yml`** - Rally benchmark configuration
2. **`<datastream>-benchmark/fields.yml`** - Field definitions for the generator
3. **`<datastream>-benchmark/config.yml`** - Generator configuration with intelligent defaults
4. **`<datastream>-benchmark/template.ndjson`** - GoText template for event generation

### Intelligent Defaults

The generator automatically:
- Sets appropriate ranges for numeric fields
- Configures cardinality for dimension fields
- Adds AWS-specific fields (regions, account IDs) for AWS packages
- Adds Kubernetes-specific fields for Kubernetes packages
- Includes common ECS fields (agent, event, etc.)

---

## generate-sample-event

Generate a `sample_event.json` file for a data stream. This is useful for documentation
and testing within integration packages.

### Usage

```bash
# Generate from package (prints to stdout)
elastic-integration-corpus-generator-tool generate-sample-event \
  --package-path /path/to/packages/aws \
  --data-stream billing

# Write to a specific file
elastic-integration-corpus-generator-tool generate-sample-event \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --output ./sample_event.json

# Write directly to package's data_stream directory
elastic-integration-corpus-generator-tool generate-sample-event \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --output-mode package

# Generate from existing benchmark files
elastic-integration-corpus-generator-tool generate-sample-event \
  --template ./template.ndjson \
  --fields ./fields.yml \
  --config ./config.yml
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--package-path` | `-p` | Path to the integration package | |
| `--data-stream` | `-d` | Data stream name | |
| `--output` | `-o` | Output file path | stdout |
| `--output-mode` | | `package` writes to `data_stream/<name>/sample_event.json` | |
| `--template` | `-t` | Path to existing template.ndjson | |
| `--fields` | `-f` | Path to existing fields.yml | |
| `--config` | `-c` | Path to existing config.yml | |
| `--pretty` | | Pretty-print JSON output | `true` |

### Example Output

```json
{
    "@timestamp": "2024-01-15T10:30:00.000Z",
    "event": {
        "dataset": "aws.billing",
        "module": "aws"
    },
    "aws": {
        "billing": {
            "amount": 125.50,
            "currency": "USD",
            "service_name": "AmazonEC2"
        }
    },
    "cloud": {
        "provider": "aws",
        "region": "us-east-1",
        "account": {
            "id": "123456789012"
        }
    }
}
```

---

## analyze-coverage

Analyze how many package fields are covered by a benchmark template or sample event.
Coverage counts both generated fields (`{{generate "field.name"}}`) and literal
fields defined directly in the template (for example `event.dataset: "aws.billing"`).

### Usage

```bash
# Analyze a template
elastic-integration-corpus-generator-tool analyze-coverage \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --template ./_dev/benchmark/rally/billing-benchmark/template.ndjson

# Analyze a sample event
elastic-integration-corpus-generator-tool analyze-coverage \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --sample-event ./data_stream/billing/sample_event.json

# JSON output
elastic-integration-corpus-generator-tool analyze-coverage \
  --package-path /path/to/packages/aws \
  --data-stream billing \
  --template ./template.ndjson \
  --json
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--package-path` | `-p` | Path to the integration package (required) |
| `--data-stream` | `-d` | Data stream name (required) |
| `--template` | `-t` | Path to template.ndjson to analyze |
| `--sample-event` | `-s` | Path to sample_event.json to analyze |
| `--json` | | Output as JSON |

### Example Output

```
📊 Field Coverage Report: billing
==================================================

Coverage: 35.3% (12/34 fields)

❌ Missing Fields:
   - aws.billing.AmortizedCost.unit
   - aws.billing.BlendedCost.unit
   - aws.billing.end_date
   ...

⚠️  Extra Fields (not in package definition):
   - cloud.account.name
   - cloud.region
```

---

## validate-ecs

Validate that package field definitions follow ECS naming conventions and type definitions.

### Usage

```bash
# Validate a specific data stream
elastic-integration-corpus-generator-tool validate-ecs \
  --package-path /path/to/packages/aws \
  --data-stream billing

# Validate all data streams
elastic-integration-corpus-generator-tool validate-ecs \
  --package-path /path/to/packages/kubernetes \
  --all

# JSON output
elastic-integration-corpus-generator-tool validate-ecs \
  --package-path /path/to/packages/aws \
  --all \
  --json
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--package-path` | `-p` | Path to the integration package (required) | |
| `--data-stream` | `-d` | Data stream name(s) to validate (comma-separated) | |
| `--all` | `-a` | Validate all data streams | `false` |
| `--json` | | Output as JSON | `false` |
| `--ecs-version` | | ECS version to validate against | `8.11.0` |

### What It Checks

1. **ECS Field Types**: Validates that ECS fields use compatible types
2. **Naming Conventions**: Checks that custom fields use snake_case
3. **Namespacing**: Ensures custom fields are properly namespaced (e.g., `aws.billing.amount` not just `amount`)

### Example Output

```
🔍 ECS Validation Report: billing
==================================================

Total Fields: 34
  - ECS Fields: 7
  - Custom Fields: 27

❌ Found 15 issue(s):

  • Field 'aws.billing.EstimatedCharges' should use snake_case (found: 'EstimatedCharges')
  • Field 'aws.billing.ServiceName' should use snake_case (found: 'ServiceName')
  ...
```

---

## Complete Workflow Example

Here's a typical workflow for adding benchmarks to a new data stream:

```bash
# 1. Explore the package
elastic-integration-corpus-generator-tool list-package \
  --package-path ~/integrations/packages/aws

# 2. Check field details for a data stream
elastic-integration-corpus-generator-tool list-package \
  --package-path ~/integrations/packages/aws \
  --data-stream dynamodb \
  --fields

# 3. Validate ECS compliance
elastic-integration-corpus-generator-tool validate-ecs \
  --package-path ~/integrations/packages/aws \
  --data-stream dynamodb

# 4. Generate benchmark files
elastic-integration-corpus-generator-tool generate-benchmark \
  --package-path ~/integrations/packages/aws \
  --data-stream dynamodb

# 5. Review and customize the generated files
# Edit config.yml to fine-tune cardinality, ranges, and enums

# 6. Test the generated benchmark
elastic-integration-corpus-generator-tool local-template \
  --template ~/integrations/packages/aws/_dev/benchmark/rally/dynamodb-benchmark/template.ndjson \
  --fields-definition ~/integrations/packages/aws/_dev/benchmark/rally/dynamodb-benchmark/fields.yml \
  --config-file ~/integrations/packages/aws/_dev/benchmark/rally/dynamodb-benchmark/config.yml \
  --tot-events 100

# 7. Analyze coverage of the generated template
elastic-integration-corpus-generator-tool analyze-coverage \
  --package-path ~/integrations/packages/aws \
  --data-stream dynamodb \
  --template ./_dev/benchmark/rally/dynamodb-benchmark/template.ndjson
```

---

## Tips for obs-infraobs-integrations Packages

For packages owned by `@elastic/obs-infraobs-integrations`:

### AWS Packages

The generator automatically includes:
- AWS regions with appropriate cardinality
- Cloud account IDs and names
- AWS-specific service fields

Customize the `config.yml` to add:
- Specific service names as enums
- Realistic metric ranges based on the service

### Kubernetes Packages

The generator automatically includes:
- Cluster and namespace configurations
- Node and pod name patterns
- Container-specific fields

Customize the `config.yml` to add:
- Specific container images as enums
- Realistic resource limits and requests
