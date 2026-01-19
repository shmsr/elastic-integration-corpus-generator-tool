// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package integrations

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTemplateFields(t *testing.T) {
	template := `{{- $timestamp := generate "@timestamp" }}
{{- $cpu := generate "system.cpu.usage" }}
{
    "@timestamp": "{{$timestamp}}",
    "cpu": {{generate "system.cpu.usage"}},
    "memory": {{generate "system.memory.bytes"}}
}`

	fields := extractTemplateFields(template)

	assert.Contains(t, fields, "@timestamp")
	assert.Contains(t, fields, "system.cpu.usage")
	assert.Contains(t, fields, "system.memory.bytes")
	assert.Len(t, fields, 3) // No duplicates
}

func TestFlattenJSON(t *testing.T) {
	data := map[string]interface{}{
		"@timestamp": "2024-01-01",
		"system": map[string]interface{}{
			"cpu": map[string]interface{}{
				"usage": 0.5,
			},
			"memory": 1024,
		},
		"tags": []interface{}{"tag1", "tag2"},
	}

	fields := flattenJSON(data, "")

	assert.Contains(t, fields, "@timestamp")
	assert.Contains(t, fields, "system.cpu.usage")
	assert.Contains(t, fields, "system.memory")
	assert.Contains(t, fields, "tags")
}

func TestIsCommonField(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"@timestamp", true},
		{"agent.id", true},
		{"agent.name", true},
		{"event.dataset", true},
		{"host.name", true},
		{"custom.field", false},
		{"aws.billing.amount", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCommonField(tt.name)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnalyzeTemplate(t *testing.T) {
	fs := afero.NewMemMapFs()

	ds := &DataStream{
		Name: "metrics",
		Fields: []PackageField{
			{Name: "system.cpu.usage", Type: "double"},
			{Name: "system.memory.bytes", Type: "long"},
			{Name: "system.disk.usage", Type: "double"},
		},
	}

	template := `{{- $timestamp := generate "@timestamp" }}
{
    "cpu": {{generate "system.cpu.usage"}},
    "memory": {{generate "system.memory.bytes"}}
}`
	require.NoError(t, afero.WriteFile(fs, "/template.ndjson", []byte(template), 0644))

	analyzer := NewCoverageAnalyzer(fs)
	report, err := analyzer.AnalyzeTemplate(ds, "/template.ndjson")
	require.NoError(t, err)

	assert.Equal(t, 3, report.TotalFields)
	assert.Equal(t, 2, report.CoveredFields)
	assert.Contains(t, report.MissingFields, "system.disk.usage")
	assert.InDelta(t, 66.67, report.CoveragePercent, 0.1)
}

func TestCoverageReportFormat(t *testing.T) {
	report := &CoverageReport{
		DataStreamName:  "test",
		TotalFields:     10,
		CoveredFields:   8,
		MissingFields:   []string{"field.a", "field.b"},
		CoveragePercent: 80.0,
	}

	output := report.FormatReport()

	assert.Contains(t, output, "test")
	assert.Contains(t, output, "80.0%")
	assert.Contains(t, output, "8/10")
	assert.Contains(t, output, "field.a")
	assert.Contains(t, output, "field.b")
}
