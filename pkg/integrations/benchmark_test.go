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

func TestBenchmarkGenerator(t *testing.T) {
	fs := afero.NewMemMapFs()

	ds := &DataStream{
		Name: "metrics",
		Fields: []PackageField{
			{Name: "testpkg.cpu", Type: "double"},
			{Name: "testpkg.memory", Type: "long"},
			{Name: "testpkg.status", Type: "keyword"},
			{Name: "testpkg.enabled", Type: "boolean"},
		},
	}

	config := BenchmarkConfig{
		TotalEvents: 10000,
		OutputDir:   "/tmp/benchmark",
		PackageName: "testpkg",
		DataStream:  "metrics",
	}

	generator := NewBenchmarkGenerator(fs, config)
	err := generator.Generate(ds)
	require.NoError(t, err)

	// Verify files were created
	exists, err := afero.Exists(fs, "/tmp/benchmark/metrics-benchmark/fields.yml")
	require.NoError(t, err)
	assert.True(t, exists, "fields.yml should exist")

	exists, err = afero.Exists(fs, "/tmp/benchmark/metrics-benchmark/config.yml")
	require.NoError(t, err)
	assert.True(t, exists, "config.yml should exist")

	exists, err = afero.Exists(fs, "/tmp/benchmark/metrics-benchmark/template.ndjson")
	require.NoError(t, err)
	assert.True(t, exists, "template.ndjson should exist")

	exists, err = afero.Exists(fs, "/tmp/benchmark/metrics-benchmark.yml")
	require.NoError(t, err)
	assert.True(t, exists, "rally config should exist")
}

func TestMapFieldType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"keyword", "keyword"},
		{"text", "keyword"},
		{"long", "long"},
		{"integer", "integer"},
		{"double", "double"},
		{"float", "float"},
		{"date", "date"},
		{"boolean", "boolean"},
		{"ip", "ip"},
		{"geo_point", "geo_point"},
		{"unknown_type", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapFieldType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildFieldDefinitions(t *testing.T) {
	ds := &DataStream{
		Name: "metrics",
		Fields: []PackageField{
			{Name: "test.field1", Type: "keyword"},
			{Name: "test.field2", Type: "long"},
		},
	}

	config := BenchmarkConfig{
		PackageName: "test",
		DataStream:  "metrics",
	}

	generator := NewBenchmarkGenerator(afero.NewMemMapFs(), config)
	fields := generator.buildFieldDefinitions(ds)

	// Should include @timestamp plus data stream fields plus common fields
	assert.True(t, len(fields) > 2, "Should have more than just data stream fields")

	// Check for @timestamp
	hasTimestamp := false
	for _, f := range fields {
		if f.Name == "@timestamp" {
			hasTimestamp = true
			break
		}
	}
	assert.True(t, hasTimestamp, "Should include @timestamp")
}
