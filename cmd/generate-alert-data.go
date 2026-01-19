// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/integrations"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// GenerateAlertDataCmd returns the generate-alert-data command
func GenerateAlertDataCmd() *cobra.Command {
	var packagePath string
	var ruleID string
	var outputFile string
	var triggerMode bool
	var numEvents int

	cmd := &cobra.Command{
		Use:   "generate-alert-data",
		Short: "Generate sample events that trigger alerting rules",
		Long: `Generate sample events designed to trigger (or not trigger) alerting rules
defined in an integration package.

This is useful for testing that alerting rules work correctly.

Example:
  # Generate events that trigger a specific alert
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

  # Output to file
  elastic-integration-corpus-generator-tool generate-alert-data \
    --package-path /path/to/packages/mongodb \
    --rule-id mongodb-connection-usage-high \
    --trigger \
    --output ./alert-test-data.ndjson
`,
		RunE: func(c *cobra.Command, args []string) error {
			if packagePath == "" {
				return fmt.Errorf("--package-path is required")
			}
			if ruleID == "" {
				return fmt.Errorf("--rule-id is required (use list-alerts to see available rules)")
			}

			fs := afero.NewOsFs()

			// Parse package to get data streams
			pkg, err := integrations.ParsePackage(fs, packagePath)
			if err != nil {
				return fmt.Errorf("failed to parse package: %w", err)
			}

			// Parse alerting rules
			parser := integrations.NewAlertingRuleParser(fs)
			rules, err := parser.ParseAlertingRules(packagePath)
			if err != nil {
				return fmt.Errorf("failed to parse alerting rules: %w", err)
			}

			// Find the specified rule
			var targetRule *integrations.AlertingRuleTemplate
			for i := range rules {
				if rules[i].ID == ruleID {
					targetRule = &rules[i]
					break
				}
			}

			if targetRule == nil {
				return fmt.Errorf("rule '%s' not found. Use list-alerts to see available rules", ruleID)
			}

			// Extract trigger config
			config, err := parser.ExtractTriggerConfig(*targetRule)
			if err != nil {
				return fmt.Errorf("failed to extract trigger config: %w", err)
			}

			// Find matching data stream
			dsName := extractDataStreamName(config.Index)
			ds := pkg.GetDataStream(dsName)

			// Generate events
			events := generateAlertEvents(config, ds, triggerMode, numEvents)

			// Output
			if outputFile != "" {
				return writeEventsToFile(events, outputFile)
			}

			// Print to stdout
			for _, event := range events {
				data, _ := json.MarshalIndent(event, "", "    ")
				fmt.Println(string(data))
			}

			fmt.Fprintf(os.Stderr, "\n✅ Generated %d event(s) for rule '%s'\n", len(events), targetRule.Attributes.Name)
			if triggerMode {
				fmt.Fprintln(os.Stderr, "⚠️  These events are designed to TRIGGER the alert")
			} else {
				fmt.Fprintln(os.Stderr, "✅ These events should NOT trigger the alert")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&packagePath, "package-path", "p", "", "Path to the integration package (required)")
	cmd.Flags().StringVarP(&ruleID, "rule-id", "r", "", "ID of the alerting rule to generate data for (required)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file path (default: stdout)")
	cmd.Flags().BoolVar(&triggerMode, "trigger", false, "Generate events that will trigger the alert (default: safe events)")
	cmd.Flags().IntVarP(&numEvents, "num-events", "n", 1, "Number of events to generate")

	return cmd
}

func extractDataStreamName(index string) string {
	// Extract data stream from index pattern
	// e.g., metrics-mongodb.status-* -> status
	// e.g., metrics-mongodb.replstatus-* -> replstatus
	parts := strings.Split(index, "-")
	if len(parts) >= 2 {
		dsWithWildcard := parts[1]
		// Remove package prefix (e.g., mongodb.status -> status)
		dsParts := strings.Split(dsWithWildcard, ".")
		if len(dsParts) >= 2 {
			return strings.TrimSuffix(dsParts[1], "*")
		}
		return strings.TrimSuffix(dsWithWildcard, "*")
	}
	return ""
}

func generateAlertEvents(config *integrations.AlertTriggerConfig, ds *integrations.DataStream, trigger bool, numEvents int) []map[string]interface{} {
	var events []map[string]interface{}

	for i := 0; i < numEvents; i++ {
		event := make(map[string]interface{})

		// Add timestamp
		event["@timestamp"] = time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339)

		// Add data_stream metadata
		event["data_stream"] = map[string]interface{}{
			"type":      strings.Split(config.Index, "-")[0],
			"dataset":   config.DataStream,
			"namespace": "default",
		}

		// Add group by field with sample value
		if config.GroupByField != "" {
			setNestedField(event, config.GroupByField, fmt.Sprintf("test-instance-%d", i+1))
		}

		// Add fields with trigger or safe values
		for _, field := range config.Fields {
			var value interface{}
			if trigger {
				value = field.TriggerValue
			} else {
				value = field.SafeValue
			}
			if value != nil {
				setNestedField(event, field.Name, value)
			}
		}

		// Add common ECS fields
		event["ecs"] = map[string]interface{}{"version": "8.11.0"}
		event["agent"] = map[string]interface{}{
			"name":    "alert-test-agent",
			"type":    "metricbeat",
			"version": "8.12.0",
		}
		event["service"] = map[string]interface{}{
			"address": fmt.Sprintf("localhost:%d", 27017+i),
			"type":    strings.Split(config.DataStream, ".")[0],
		}

		// If we have access to data stream fields, add more realistic data
		if ds != nil {
			addFieldsFromDataStream(event, ds, config, trigger)
		}

		events = append(events, event)
	}

	return events
}

func setNestedField(obj map[string]interface{}, path string, value interface{}) {
	parts := strings.Split(path, ".")
	current := obj

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
		} else {
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			current = current[part].(map[string]interface{})
		}
	}
}

func addFieldsFromDataStream(event map[string]interface{}, ds *integrations.DataStream, config *integrations.AlertTriggerConfig, trigger bool) {
	// Add fields from data stream that are relevant to the alert
	for _, field := range ds.Fields {
		// Skip if we already set this field
		if containsField(config.Fields, field.Name) {
			continue
		}

		// Check if this field is referenced in the alert
		esql := ""
		if config.Index != "" {
			esql = config.Index // Used as proxy to check if field is relevant
		}

		// Add common monitoring fields
		if strings.Contains(field.Name, "connection") ||
			strings.Contains(field.Name, "cache") ||
			strings.Contains(field.Name, "memory") ||
			strings.Contains(field.Name, "cpu") ||
			strings.Contains(field.Name, "error") ||
			strings.Contains(field.Name, "lag") {

			var value interface{}
			switch field.Type {
			case "long", "integer":
				if trigger {
					value = 95000 // High value
				} else {
					value = 1000 // Low value
				}
			case "double", "float":
				if trigger {
					value = 95.5
				} else {
					value = 10.5
				}
			case "boolean":
				value = trigger
			default:
				value = "test-value"
			}
			setNestedField(event, field.Name, value)
		}
		_ = esql // silence unused warning
	}
}

func containsField(fields []integrations.AlertField, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func writeEventsToFile(events []map[string]interface{}, outputFile string) error {
	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		f.WriteString(string(data) + "\n")
	}

	fmt.Printf("✅ Generated %d event(s) to %s\n", len(events), outputFile)
	return nil
}

// parseThresholdFromQuery extracts threshold values from ES|QL WHERE clauses
func parseThresholdFromQuery(esql string) map[string]float64 {
	thresholds := make(map[string]float64)

	// Match patterns like: field > 85, field_pct > 80
	re := regexp.MustCompile(`(\w+)\s*[><=]+\s*([\d.]+)`)
	matches := re.FindAllStringSubmatch(esql, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			if val, err := strconv.ParseFloat(match[2], 64); err == nil {
				thresholds[match[1]] = val
			}
		}
	}

	return thresholds
}
