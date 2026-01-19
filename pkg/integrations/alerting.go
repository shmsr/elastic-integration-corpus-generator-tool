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

	// Match patterns like: field > 85, field < 15, field >= 80, `field` >= 0.85
	whereRegex := regexp.MustCompile("(?:`([^`]+)`|(\\w+))\\s*(>|<|>=|<=|==|!=)\\s*([\\d.]+)")
	matches := whereRegex.FindAllStringSubmatch(esql, -1)

	for _, match := range matches {
		if len(match) >= 5 {
			// Field is in group 1 (backtick-quoted) or group 2 (unquoted)
			field := match[1]
			if field == "" {
				field = match[2]
			}
			threshold, _ := strconv.ParseFloat(match[4], 64)
			conditions = append(conditions, AlertCondition{
				Field:     field,
				Operator:  match[3],
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

	// Extract fields from STATS clause - handle both regular and backtick-quoted field names
	// e.g., AVG(mongodb.status.wired_tiger.cache.used.bytes) or AVG(`system.cpu.total.norm.pct`)
	// Also handle formulas like AVG(host.cpu.usage*100)
	statsRegex := regexp.MustCompile("(\\w+)\\s*=\\s*(AVG|MAX|MIN|SUM|COUNT)\\((?:`([^`]+)`|([\\w\\.]+(?:\\*\\d+)?))\\)")
	matches := statsRegex.FindAllStringSubmatch(esql, -1)

	// Build alias to field mapping
	aliasToField := make(map[string]string)
	fieldToAlias := make(map[string]string)
	for _, m := range matches {
		if len(m) >= 5 {
			alias := m[1]
			// Field name is in group 3 (backtick-quoted) or group 4 (unquoted)
			fieldName := m[3]
			if fieldName == "" {
				fieldName = m[4]
			}
			// Remove any formula parts (e.g., *100 from host.cpu.usage*100)
			if idx := strings.Index(fieldName, "*"); idx > 0 {
				fieldName = fieldName[:idx]
			}
			if fieldName != "" {
				aliasToField[alias] = fieldName
				fieldToAlias[fieldName] = alias
			}
		}
	}

	// Detect ratio-based alerts (e.g., field_pct = (a / b) * 100)
	isRatioAlert := strings.Contains(esql, "* 100") || strings.Contains(esql, "*100")

	// Map conditions to aliases
	aliasCondition := make(map[string]AlertCondition)
	for _, cond := range conditions {
		aliasCondition[cond.Field] = cond
	}

	// For ratio alerts, identify numerator field (the one being divided)
	var numeratorAlias string
	var denominatorAlias string
	// Track if denominator is a sum (like total_conn = current_conn + available_conn)
	sumComponents := make(map[string]bool) // aliases that are part of a sum forming the denominator

	if isRatioAlert {
		// Look for division pattern in EVAL statements: (field1 / field2) * 100
		// Be specific to avoid matching comments
		divRegex := regexp.MustCompile(`EVAL\s+\w+\s*=\s*\(?\s*(\w+)\s*/\s*(\w+)\s*\)?`)
		if divMatch := divRegex.FindStringSubmatch(esql); len(divMatch) >= 3 {
			numeratorAlias = divMatch[1]
			denominatorAlias = divMatch[2]

			// Check if denominator is a computed sum (EVAL x = a + b)
			sumRegex := regexp.MustCompile(`EVAL\s+` + denominatorAlias + `\s*=\s*(\w+)\s*\+\s*(\w+)`)
			if sumMatch := sumRegex.FindStringSubmatch(esql); len(sumMatch) >= 3 {
				sumComponents[sumMatch[1]] = true
				sumComponents[sumMatch[2]] = true
			}
		}
	}

	// Find the percent threshold for ratio alerts
	var percentThreshold float64 = 80.0
	for _, cond := range conditions {
		if strings.Contains(cond.Field, "pct") || strings.Contains(cond.Field, "percent") ||
			strings.Contains(cond.Field, "ratio") || strings.Contains(cond.Field, "usage") {
			percentThreshold = cond.Threshold
			break
		}
	}

	for _, match := range matches {
		if len(match) >= 5 {
			alias := match[1]
			// Field name is in group 3 (backtick-quoted) or group 4 (unquoted)
			fieldName := match[3]
			if fieldName == "" {
				fieldName = match[4]
			}
			// Remove any formula parts (e.g., *100 from host.cpu.usage*100)
			if idx := strings.Index(fieldName, "*"); idx > 0 {
				fieldName = fieldName[:idx]
			}
			if fieldName == "" || seenFields[fieldName] {
				continue
			}
			seenFields[fieldName] = true

			field := AlertField{
				Name: fieldName,
			}

			// Check if this alias has a meaningful direct condition (not just validity checks like > 0)
			// Note: Threshold > 0.001 to exclude == 0 checks but allow decimals like 0.85
			if cond, hasDirectCond := aliasCondition[alias]; hasDirectCond && cond.Threshold > 0.001 {
				// Direct aggregation with condition (e.g., AVG(field) >= threshold)
				switch cond.Operator {
				case ">":
					field.TriggerValue = cond.Threshold + (cond.Threshold * 0.2)
					field.SafeValue = cond.Threshold * 0.5
				case ">=":
					field.TriggerValue = cond.Threshold + (cond.Threshold * 0.1)
					field.SafeValue = cond.Threshold * 0.5
				case "<":
					field.TriggerValue = cond.Threshold * 0.5
					field.SafeValue = cond.Threshold * 2
				case "<=":
					field.TriggerValue = cond.Threshold * 0.9
					field.SafeValue = cond.Threshold * 2
				}
			} else if isRatioAlert {
				// Handle ratio-based alerts
				if alias == numeratorAlias || (sumComponents[alias] && alias == numeratorAlias) {
					// This is the numerator field - set high for trigger
					field.TriggerValue = (percentThreshold + 10) * 1000.0 // e.g., 95% * 1000 = 95000
					field.SafeValue = (percentThreshold - 40) * 1000.0    // e.g., 45% * 1000 = 45000
				} else if alias == denominatorAlias {
					// Denominator field - always set to 100000 as the base for percentage calculation
					field.TriggerValue = 100000.0
					field.SafeValue = 100000.0
				} else if sumComponents[alias] {
					// This alias is part of a sum that forms the denominator
					// e.g., for current/(current+available), available is a sum component
					// For high trigger: low value helps numerator dominate the ratio
					// For safe: high value makes numerator percentage low
					if strings.Contains(alias, "available") || strings.Contains(fieldName, "available") ||
						alias != numeratorAlias {
						field.TriggerValue = 10000.0 // Low = high ratio for numerator
						field.SafeValue = 150000.0   // High = low ratio for numerator
					} else {
						field.TriggerValue = 90000.0
						field.SafeValue = 40000.0
					}
				} else {
					// Other fields not in the ratio formula
					if strings.Contains(alias, "available") || strings.Contains(fieldName, "available") {
						field.TriggerValue = 10000.0
						field.SafeValue = 150000.0
					} else {
						field.TriggerValue = 100.0
						field.SafeValue = 10.0
					}
				}
			} else {
				// Try to match field to any condition
				matched := false
				for _, cond := range conditions {
					if strings.Contains(esql, fieldName) && strings.Contains(esql, cond.Field) {
						switch cond.Operator {
						case ">":
							field.TriggerValue = cond.Threshold + (cond.Threshold * 0.2)
							field.SafeValue = cond.Threshold * 0.5
						case ">=":
							field.TriggerValue = cond.Threshold + (cond.Threshold * 0.1)
							field.SafeValue = cond.Threshold * 0.5
						case "<":
							field.TriggerValue = cond.Threshold * 0.5
							field.SafeValue = cond.Threshold * 2
						case "<=":
							field.TriggerValue = cond.Threshold * 0.9
							field.SafeValue = cond.Threshold * 2
						}
						matched = true
						break
					}
				}

				// Default values if no condition matched
				if !matched {
					field.TriggerValue = 100.0
					field.SafeValue = 10.0
				}
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
