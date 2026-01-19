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
| `list-alerts` | List alerting rule templates in a package |
| `generate-alert-data` | Generate events that trigger alerting rules |

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

---

## list-alerts

List alerting rule templates defined in an integration package's `kibana/alerting_rule_template` directory.

### Usage

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

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--package-path` | `-p` | Path to the integration package (required) |
| `--show-query` | | Show the ES|QL queries |
| `--json` | | Output as JSON |

### Example Output

```
📢 Alerting Rule Templates (6 total)
============================================================

📋 [MongoDB] WiredTiger cache pressure
   ID: mongodb-cache-usage-high
   Type: .es-query
   Schedule: 1m
   Time Window: 5m
   Tags: MongoDB

📋 [MongoDB Availability] High connection usage
   ID: mongodb-connection-usage-high
   Type: .es-query
   Schedule: 1m
   Time Window: 5m
   Tags: MongoDB
```

---

## generate-alert-data

Generate sample events designed to trigger (or not trigger) alerting rules.

This is useful for testing that alerting rules work correctly.

### Usage

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

# Generate multiple events
elastic-integration-corpus-generator-tool generate-alert-data \
  --package-path /path/to/packages/aws \
  --rule-id aws-ec2-high-cpu-utilization \
  --trigger \
  --num-events 10

# Output to file (NDJSON format)
elastic-integration-corpus-generator-tool generate-alert-data \
  --package-path /path/to/packages/mongodb \
  --rule-id mongodb-connection-usage-high \
  --trigger \
  --output ./alert-test-data.ndjson
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--package-path` | `-p` | Path to the integration package (required) | |
| `--rule-id` | `-r` | ID of the alerting rule (required) | |
| `--trigger` | | Generate events that will trigger the alert | `false` |
| `--num-events` | `-n` | Number of events to generate | `1` |
| `--output` | `-o` | Output file path | stdout |

### How It Works

1. Parses the alerting rule's ES|QL query
2. Extracts threshold conditions (e.g., `> 85`, `< 15`)
3. Identifies the relevant data stream and fields
4. Generates events with values above/below thresholds

### Example

For the MongoDB cache alert with condition `cache_usage_pct > 85`:

**Trigger mode (`--trigger`)**: Generates events with ~102% cache usage (above 85%)  
**Safe mode (default)**: Generates events with ~42.5% cache usage (below 85%)

### Packages with Alerting Rules

Several infraobs-owned packages include alerting rule templates:

| Package | Alert Rules |
|---------|-------------|
| `mongodb` | 6 rules (cache, connections, replication) |
| `mysql` | 3 rules (galera, replication lag, slow queries) |
| `microsoft_sqlserver` | 3 rules (lock waits, memory, log space) |
| `aws` | 8 rules (EC2, Lambda, SNS, SQS) |
| `azure_openai` | 3 rules (latency, utilization, errors) |
| `azure_ai_foundry` | 3 rules (latency, availability, utilization) |
| `aws_bedrock_agentcore` | 15 rules (various agent errors) |
