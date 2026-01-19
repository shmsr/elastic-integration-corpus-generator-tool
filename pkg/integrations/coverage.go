// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package integrations

import (
	"bufio"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/afero"
)

// CoverageReport represents the field coverage analysis
type CoverageReport struct {
	PackageName     string
	DataStreamName  string
	TotalFields     int
	CoveredFields   int
	MissingFields   []string
	ExtraFields     []string
	CoveragePercent float64
	FieldDetails    []FieldCoverage
}

// FieldCoverage represents coverage status for a single field
type FieldCoverage struct {
	Name      string
	Type      string
	IsCovered bool
	IsExtra   bool
	Source    string // "package" or "template"
}

// CoverageAnalyzer analyzes field coverage in templates
type CoverageAnalyzer struct {
	fs afero.Fs
}

// NewCoverageAnalyzer creates a new coverage analyzer
func NewCoverageAnalyzer(fs afero.Fs) *CoverageAnalyzer {
	return &CoverageAnalyzer{fs: fs}
}

// AnalyzeTemplate analyzes field coverage in a template against package fields
func (c *CoverageAnalyzer) AnalyzeTemplate(ds *DataStream, templatePath string) (*CoverageReport, error) {
	// Read template file
	templateData, err := afero.ReadFile(c.fs, templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template: %w", err)
	}

	// Extract fields from template
	templateFields := extractTemplateFields(string(templateData))

	// Build package field map
	packageFields := make(map[string]PackageField)
	for _, f := range ds.Fields {
		packageFields[f.Name] = f
	}

	// Calculate coverage
	report := &CoverageReport{
		DataStreamName: ds.Name,
		TotalFields:    len(ds.Fields),
	}

	// Check which package fields are covered
	coveredMap := make(map[string]bool)
	for _, tf := range templateFields {
		if _, exists := packageFields[tf]; exists {
			coveredMap[tf] = true
		} else {
			// Check if it's an ECS or agent field (not "extra")
			if !isCommonField(tf) {
				report.ExtraFields = append(report.ExtraFields, tf)
			}
		}
	}

	// Find missing fields
	for name, f := range packageFields {
		detail := FieldCoverage{
			Name:      name,
			Type:      f.Type,
			IsCovered: coveredMap[name],
			Source:    "package",
		}
		report.FieldDetails = append(report.FieldDetails, detail)

		if coveredMap[name] {
			report.CoveredFields++
		} else {
			report.MissingFields = append(report.MissingFields, name)
		}
	}

	// Add extra fields to details
	for _, name := range report.ExtraFields {
		report.FieldDetails = append(report.FieldDetails, FieldCoverage{
			Name:    name,
			IsExtra: true,
			Source:  "template",
		})
	}

	// Sort for consistent output
	sort.Strings(report.MissingFields)
	sort.Strings(report.ExtraFields)
	sort.Slice(report.FieldDetails, func(i, j int) bool {
		return report.FieldDetails[i].Name < report.FieldDetails[j].Name
	})

	// Calculate percentage
	if report.TotalFields > 0 {
		report.CoveragePercent = float64(report.CoveredFields) / float64(report.TotalFields) * 100
	}

	return report, nil
}

// AnalyzeSampleEvent analyzes a sample_event.json against package fields
func (c *CoverageAnalyzer) AnalyzeSampleEvent(ds *DataStream, samplePath string) (*CoverageReport, error) {
	// Read sample event
	sampleData, err := afero.ReadFile(c.fs, samplePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read sample event: %w", err)
	}

	// Parse JSON
	var event map[string]interface{}
	if err := json.Unmarshal(sampleData, &event); err != nil {
		return nil, fmt.Errorf("failed to parse sample event: %w", err)
	}

	// Flatten JSON to field names
	sampleFields := flattenJSON(event, "")

	// Build package field map
	packageFields := make(map[string]PackageField)
	for _, f := range ds.Fields {
		packageFields[f.Name] = f
	}

	// Calculate coverage
	report := &CoverageReport{
		DataStreamName: ds.Name,
		TotalFields:    len(ds.Fields),
	}

	// Check which package fields are covered
	coveredMap := make(map[string]bool)
	for _, sf := range sampleFields {
		if _, exists := packageFields[sf]; exists {
			coveredMap[sf] = true
		} else if !isCommonField(sf) {
			report.ExtraFields = append(report.ExtraFields, sf)
		}
	}

	// Find missing fields
	for name, f := range packageFields {
		detail := FieldCoverage{
			Name:      name,
			Type:      f.Type,
			IsCovered: coveredMap[name],
			Source:    "package",
		}
		report.FieldDetails = append(report.FieldDetails, detail)

		if coveredMap[name] {
			report.CoveredFields++
		} else {
			report.MissingFields = append(report.MissingFields, name)
		}
	}

	// Sort
	sort.Strings(report.MissingFields)
	sort.Strings(report.ExtraFields)

	// Calculate percentage
	if report.TotalFields > 0 {
		report.CoveragePercent = float64(report.CoveredFields) / float64(report.TotalFields) * 100
	}

	return report, nil
}

// extractTemplateFields extracts field names from a GoText template
func extractTemplateFields(template string) []string {
	// Match patterns like: generate "field.name"
	re := regexp.MustCompile(`generate\s+"([^"]+)"`)
	matches := re.FindAllStringSubmatch(template, -1)

	seen := make(map[string]bool)
	fields := make([]string, 0, len(matches))

	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			fields = append(fields, match[1])
			seen[match[1]] = true
		}
	}

	// Add literal fields from JSON keys (e.g., event.dataset: "value")
	for _, name := range extractLiteralTemplateFields(template) {
		if !seen[name] {
			fields = append(fields, name)
			seen[name] = true
		}
	}

	return fields
}

func extractLiteralTemplateFields(template string) []string {
	scanner := bufio.NewScanner(strings.NewReader(template))
	keyRe := regexp.MustCompile(`"([^"]+)"\s*:`)

	fields := make(map[string]bool)
	var stack []string

	for scanner.Scan() {
		line := stripTemplateTags(scanner.Text())
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := keyRe.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			key := line[match[2]:match[3]]
			after := strings.TrimSpace(line[match[1]:])

			fullKey := key
			if len(stack) > 0 {
				fullKey = strings.Join(append(stack, key), ".")
			}

			if strings.HasPrefix(after, "{") {
				stack = append(stack, key)
			} else {
				fields[fullKey] = true
			}
		}

		closeCount := strings.Count(line, "}")
		for i := 0; i < closeCount && len(stack) > 0; i++ {
			stack = stack[:len(stack)-1]
		}
	}

	result := make([]string, 0, len(fields))
	for name := range fields {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func stripTemplateTags(line string) string {
	for {
		start := strings.Index(line, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(line[start+2:], "}}")
		if end == -1 {
			break
		}
		endIdx := start + 2 + end + 2
		line = line[:start] + line[endIdx:]
	}
	return line
}

// flattenJSON flattens a nested JSON object into dot-notation field names
func flattenJSON(data map[string]interface{}, prefix string) []string {
	var fields []string

	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			// Recurse into nested objects
			nested := flattenJSON(v, fullKey)
			fields = append(fields, nested...)
		case []interface{}:
			// For arrays, check if first element is an object
			if len(v) > 0 {
				if obj, ok := v[0].(map[string]interface{}); ok {
					nested := flattenJSON(obj, fullKey)
					fields = append(fields, nested...)
				} else {
					fields = append(fields, fullKey)
				}
			} else {
				fields = append(fields, fullKey)
			}
		default:
			fields = append(fields, fullKey)
		}
	}

	return fields
}

// isCommonField checks if a field is a common ECS/agent field
func isCommonField(name string) bool {
	commonPrefixes := []string{
		"@timestamp",
		"agent.",
		"ecs.",
		"event.",
		"host.",
		"metricset.",
		"service.",
		"data_stream.",
		"elastic_agent.",
		"input.",
	}

	for _, prefix := range commonPrefixes {
		if name == prefix || strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// FormatReport formats a coverage report as a string
func (r *CoverageReport) FormatReport() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 Field Coverage Report: %s\n", r.DataStreamName))
	sb.WriteString(strings.Repeat("=", 50) + "\n\n")

	sb.WriteString(fmt.Sprintf("Coverage: %.1f%% (%d/%d fields)\n\n",
		r.CoveragePercent, r.CoveredFields, r.TotalFields))

	if len(r.MissingFields) > 0 {
		sb.WriteString("❌ Missing Fields:\n")
		for _, f := range r.MissingFields {
			sb.WriteString(fmt.Sprintf("   - %s\n", f))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("✅ All package fields are covered!\n\n")
	}

	if len(r.ExtraFields) > 0 {
		sb.WriteString("⚠️  Extra Fields (not in package definition):\n")
		for _, f := range r.ExtraFields {
			sb.WriteString(fmt.Sprintf("   - %s\n", f))
		}
	}

	return sb.String()
}

// FormatJSON returns the report as JSON
func (r *CoverageReport) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
