// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/integrations"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// ListAlertsCmd returns the list-alerts command
func ListAlertsCmd() *cobra.Command {
	var packagePath string
	var jsonOutput bool
	var showQuery bool

	cmd := &cobra.Command{
		Use:   "list-alerts",
		Short: "List alerting rule templates in an integration package",
		Long: `Display alerting rule templates defined in an integration package's 
kibana/alerting_rule_template directory.

This helps you understand what alerts are available and how to generate
test data that would trigger them.

Example:
  # List all alerting rules in a package
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
`,
		RunE: func(c *cobra.Command, args []string) error {
			if packagePath == "" {
				return fmt.Errorf("--package-path is required")
			}

			fs := afero.NewOsFs()
			parser := integrations.NewAlertingRuleParser(fs)

			rules, err := parser.ParseAlertingRules(packagePath)
			if err != nil {
				return fmt.Errorf("failed to parse alerting rules: %w", err)
			}

			if jsonOutput {
				return outputAlertsAsJSON(rules)
			}

			return outputAlertsAsText(rules, showQuery)
		},
	}

	cmd.Flags().StringVarP(&packagePath, "package-path", "p", "", "Path to the integration package (required)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&showQuery, "show-query", false, "Show the ES|QL queries")

	return cmd
}

func outputAlertsAsText(rules []integrations.AlertingRuleTemplate, showQuery bool) error {
	if len(rules) == 0 {
		fmt.Println("No alerting rule templates found in this package.")
		return nil
	}

	fmt.Printf("📢 Alerting Rule Templates (%d total)\n", len(rules))
	fmt.Println(strings.Repeat("=", 60))

	for _, rule := range rules {
		fmt.Println()
		fmt.Printf("📋 %s\n", rule.Attributes.Name)
		fmt.Printf("   ID: %s\n", rule.ID)
		fmt.Printf("   Type: %s\n", rule.Attributes.RuleTypeID)
		fmt.Printf("   Schedule: %s\n", rule.Attributes.Schedule.Interval)

		if rule.Attributes.Params.ESQLQuery != nil {
			fmt.Printf("   Time Window: %d%s\n",
				rule.Attributes.Params.TimeWindowSize,
				rule.Attributes.Params.TimeWindowUnit)
		}

		if len(rule.Attributes.Tags) > 0 {
			fmt.Printf("   Tags: %s\n", strings.Join(rule.Attributes.Tags, ", "))
		}

		if showQuery && rule.Attributes.Params.ESQLQuery != nil {
			fmt.Println()
			fmt.Println("   ES|QL Query:")
			fmt.Println("   " + strings.Repeat("-", 50))
			for _, line := range strings.Split(rule.Attributes.Params.ESQLQuery.ESQL, "\n") {
				fmt.Printf("   %s\n", line)
			}
		}
	}

	fmt.Println()
	fmt.Println("💡 Tip: Use generate-alert-data to create events that trigger these alerts")

	return nil
}

func outputAlertsAsJSON(rules []integrations.AlertingRuleTemplate) error {
	type alertInfo struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		Type       string   `json:"type"`
		Schedule   string   `json:"schedule"`
		TimeWindow string   `json:"time_window,omitempty"`
		Tags       []string `json:"tags,omitempty"`
		Query      string   `json:"query,omitempty"`
	}

	var alerts []alertInfo
	for _, rule := range rules {
		info := alertInfo{
			ID:       rule.ID,
			Name:     rule.Attributes.Name,
			Type:     rule.Attributes.RuleTypeID,
			Schedule: rule.Attributes.Schedule.Interval,
			Tags:     rule.Attributes.Tags,
		}

		if rule.Attributes.Params.ESQLQuery != nil {
			info.TimeWindow = fmt.Sprintf("%d%s",
				rule.Attributes.Params.TimeWindowSize,
				rule.Attributes.Params.TimeWindowUnit)
			info.Query = rule.Attributes.Params.ESQLQuery.ESQL
		}

		alerts = append(alerts, info)
	}

	output, err := json.MarshalIndent(alerts, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}
