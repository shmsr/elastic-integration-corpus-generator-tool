// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package integrations

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

// AlertingRuleTemplate represents a Kibana alerting rule template
type AlertingRuleTemplate struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Attributes AlertingRuleAttributes `json:"attributes"`
}

// AlertingRuleAttributes contains the rule configuration
type AlertingRuleAttributes struct {
	Name       string           `json:"name"`
	Tags       []string         `json:"tags"`
	RuleTypeID string           `json:"ruleTypeId"`
	Schedule   AlertingSchedule `json:"schedule"`
	Params     AlertingParams   `json:"params"`
}

// AlertingSchedule defines the rule execution schedule
type AlertingSchedule struct {
	Interval string `json:"interval"`
}

// AlertingParams contains the ES|QL query and other params
type AlertingParams struct {
	SearchType     string     `json:"searchType"`
	TimeWindowSize int        `json:"timeWindowSize"`
	TimeWindowUnit string     `json:"timeWindowUnit"`
	ESQLQuery      *ESQLQuery `json:"esqlQuery,omitempty"`
	GroupBy        string     `json:"groupBy"`
	TermSize       int        `json:"termSize"`
	TimeField      string     `json:"timeField"`
}

// ESQLQuery contains the ES|QL query string
type ESQLQuery struct {
	ESQL string `json:"esql"`
}

// AlertCondition represents an extracted alert condition
type AlertCondition struct {
	Field     string
	Operator  string
	Threshold float64
	Unit      string
}

// AlertTriggerConfig contains configuration for generating trigger events
type AlertTriggerConfig struct {
	RuleName     string
	RuleID       string
	DataStream   string
	Index        string
	Fields       []AlertField
	Conditions   []AlertCondition
	GroupByField string
}

// AlertField represents a field needed for the alert
type AlertField struct {
	Name         string
	TriggerValue interface{}
	SafeValue    interface{}
}

// AlertingRuleParser parses alerting rule templates
type AlertingRuleParser struct {
	fs afero.Fs
}

// NewAlertingRuleParser creates a new alerting rule parser
func NewAlertingRuleParser(fs afero.Fs) *AlertingRuleParser {
	return &AlertingRuleParser{fs: fs}
}

// ParseAlertingRules parses all alerting rule templates in a package
func (p *AlertingRuleParser) ParseAlertingRules(packagePath string) ([]AlertingRuleTemplate, error) {
	alertDir := filepath.Join(packagePath, "kibana", "alerting_rule_template")

	exists, err := afero.DirExists(p.fs, alertDir)
	if err != nil || !exists {
		return nil, nil // No alerting rules is valid
	}

	entries, err := afero.ReadDir(p.fs, alertDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read alerting_rule_template directory: %w", err)
	}

	var rules []AlertingRuleTemplate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		rulePath := filepath.Join(alertDir, entry.Name())
		data, err := afero.ReadFile(p.fs, rulePath)
		if err != nil {
			continue
		}

		var rule AlertingRuleTemplate
		if err := json.Unmarshal(data, &rule); err != nil {
			continue
		}

		rules = append(rules, rule)
	}

	// Sort by ID for consistent output
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})

	return rules, nil
}

// ExtractTriggerConfig analyzes a rule and extracts configuration for generating trigger events
func (p *AlertingRuleParser) ExtractTriggerConfig(rule AlertingRuleTemplate) (*AlertTriggerConfig, error) {
	if rule.Attributes.Params.ESQLQuery == nil {
		return nil, fmt.Errorf("rule %s has no ES|QL query", rule.ID)
	}

	esql := rule.Attributes.Params.ESQLQuery.ESQL
	config := &AlertTriggerConfig{
		RuleName: rule.Attributes.Name,
		RuleID:   rule.ID,
	}

	// Extract index/data stream from FROM clause
	fromRegex := regexp.MustCompile(`FROM\s+([\w\-\*\.]+)`)
	if match := fromRegex.FindStringSubmatch(esql); len(match) > 1 {
		config.Index = match[1]
		// Extract data stream from index pattern (e.g., metrics-mongodb.status-* -> mongodb.status)
		parts := strings.Split(match[1], "-")
		if len(parts) >= 2 {
			config.DataStream = strings.TrimSuffix(parts[1], "*")
		}
	}

	// Extract GROUP BY field
	groupByRegex := regexp.MustCompile(`BY\s+([\w\.]+)`)
	if match := groupByRegex.FindStringSubmatch(esql); len(match) > 1 {
		config.GroupByField = match[1]
	}

	// Extract conditions from WHERE clauses
	config.Conditions = extractConditions(esql)

	// Extract fields from STATS clause
	config.Fields = extractFieldsFromESQL(esql, config.Conditions)

	return config, nil
}

// extractConditions parses WHERE clauses to find thresholds
func extractConditions(esql string) []AlertCondition {
	var conditions []AlertCondition

	// Match patterns like: field > 85, field < 15, field >= 80
	whereRegex := regexp.MustCompile(`(\w+)\s*(>|<|>=|<=|==|!=)\s*([\d.]+)`)
	matches := whereRegex.FindAllStringSubmatch(esql, -1)

	for _, match := range matches {
		if len(match) >= 4 {
			threshold, _ := strconv.ParseFloat(match[3], 64)
			conditions = append(conditions, AlertCondition{
				Field:     match[1],
				Operator:  match[2],
				Threshold: threshold,
			})
		}
	}

	return conditions
}

// extractFieldsFromESQL identifies fields used in the query
func extractFieldsFromESQL(esql string, conditions []AlertCondition) []AlertField {
	var fields []AlertField
	seenFields := make(map[string]bool)

	// Extract fields from STATS clause (e.g., AVG(mongodb.status.wired_tiger.cache.used.bytes))
	statsRegex := regexp.MustCompile(`(AVG|MAX|MIN|SUM|COUNT)\(([\w\.]+)\)`)
	matches := statsRegex.FindAllStringSubmatch(esql, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			fieldName := match[2]
			if seenFields[fieldName] {
				continue
			}
			seenFields[fieldName] = true

			field := AlertField{
				Name: fieldName,
			}

			// Determine trigger values based on conditions
			for _, cond := range conditions {
				// Try to match field to condition
				if strings.Contains(esql, fieldName) && strings.Contains(esql, cond.Field) {
					// Generate a value that would trigger the alert
					switch cond.Operator {
					case ">":
						field.TriggerValue = cond.Threshold + (cond.Threshold * 0.2) // 20% above threshold
						field.SafeValue = cond.Threshold * 0.5                       // 50% of threshold
					case ">=":
						field.TriggerValue = cond.Threshold
						field.SafeValue = cond.Threshold * 0.5
					case "<":
						field.TriggerValue = cond.Threshold * 0.5 // 50% of threshold
						field.SafeValue = cond.Threshold * 2      // 2x threshold
					case "<=":
						field.TriggerValue = cond.Threshold
						field.SafeValue = cond.Threshold * 2
					}
					break
				}
			}

			// Default values if no condition matched
			if field.TriggerValue == nil {
				field.TriggerValue = 100.0
				field.SafeValue = 10.0
			}

			fields = append(fields, field)
		}
	}

	return fields
}

// FormatRuleSummary returns a human-readable summary of the rule
func (p *AlertingRuleParser) FormatRuleSummary(rule AlertingRuleTemplate) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📋 %s\n", rule.Attributes.Name))
	sb.WriteString(fmt.Sprintf("   ID: %s\n", rule.ID))
	sb.WriteString(fmt.Sprintf("   Type: %s\n", rule.Attributes.RuleTypeID))
	sb.WriteString(fmt.Sprintf("   Schedule: %s\n", rule.Attributes.Schedule.Interval))

	if rule.Attributes.Params.ESQLQuery != nil {
		sb.WriteString(fmt.Sprintf("   Time Window: %d%s\n",
			rule.Attributes.Params.TimeWindowSize,
			rule.Attributes.Params.TimeWindowUnit))

		// Extract index from query
		fromRegex := regexp.MustCompile(`FROM\s+([\w\-\*\.]+)`)
		if match := fromRegex.FindStringSubmatch(rule.Attributes.Params.ESQLQuery.ESQL); len(match) > 1 {
			sb.WriteString(fmt.Sprintf("   Index: %s\n", match[1]))
		}
	}

	if len(rule.Attributes.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("   Tags: %s\n", strings.Join(rule.Attributes.Tags, ", ")))
	}

	return sb.String()
}

// ListAlertingRules returns a formatted list of all alerting rules in a package
func (p *AlertingRuleParser) ListAlertingRules(packagePath string) (string, error) {
	rules, err := p.ParseAlertingRules(packagePath)
	if err != nil {
		return "", err
	}

	if len(rules) == 0 {
		return "No alerting rule templates found in this package.\n", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📢 Alerting Rule Templates (%d total)\n", len(rules)))
	sb.WriteString(strings.Repeat("=", 50) + "\n\n")

	for _, rule := range rules {
		sb.WriteString(p.FormatRuleSummary(rule))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
