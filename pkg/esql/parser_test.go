// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License
// 2.0; you may not use this file except in compliance with the Elastic License
// 2.0.

package esql

import (
	"strings"
	"testing"
)

func TestParseSimpleQuery(t *testing.T) {
	query := `FROM metrics-mongodb.status-*
| STATS cache_used=AVG(mongodb.status.wired_tiger.cache.used.bytes),
        cache_max=AVG(mongodb.status.wired_tiger.cache.maximum.bytes) BY service.address
| EVAL cache_usage_pct = (cache_used / cache_max) * 100
| WHERE cache_usage_pct > 85`

	q, err := Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	if q == nil {
		t.Fatal("Parsed query is nil")
	}

	config := ExtractAlertConfig(q)

	// Verify index
	if config.Index != "metrics-mongodb.status-*" {
		t.Errorf("Expected index 'metrics-mongodb.status-*', got '%s'", config.Index)
	}

	// Verify data stream
	if config.DataStream != "mongodb.status" {
		t.Errorf("Expected data stream 'mongodb.status', got '%s'", config.DataStream)
	}

	// Verify fields
	if len(config.Fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(config.Fields))
	}

	// Verify conditions
	if len(config.Conditions) == 0 {
		t.Error("Expected at least one condition")
	}

	// Verify eval
	if len(config.Evals) == 0 {
		t.Error("Expected at least one eval")
	}

	t.Logf("Parsed config: %+v", config)
}

func TestParseConnectionQuery(t *testing.T) {
	query := `FROM metrics-mongodb.status-*
| STATS current_conn=AVG(mongodb.status.connections.current),
        available_conn=AVG(mongodb.status.connections.available) BY service.address
| EVAL total_conn = current_conn + available_conn
| WHERE total_conn > 0
| EVAL connection_usage_pct = (current_conn / total_conn) * 100
| WHERE connection_usage_pct > 80`

	q, err := Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	config := ExtractAlertConfig(q)

	// Should have 2 aggregation fields
	if len(config.Fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(config.Fields))
	}

	// Check aliases
	aliases := make(map[string]bool)
	for _, f := range config.Fields {
		aliases[f.Alias] = true
	}

	if !aliases["current_conn"] {
		t.Error("Missing alias 'current_conn'")
	}
	if !aliases["available_conn"] {
		t.Error("Missing alias 'available_conn'")
	}

	t.Logf("Parsed config: %+v", config)
}

func TestParseBacktickFields(t *testing.T) {
	query := `FROM metrics-system.cpu-*
| KEEP ` + "`host.name`" + `, ` + "`system.cpu.total.norm.pct`" + `
| STATS avg_cpu_util = AVG(` + "`system.cpu.total.norm.pct`" + `) BY ` + "`host.name`" + `
| WHERE avg_cpu_util >= 0.85`

	q, err := Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	config := ExtractAlertConfig(q)

	if len(config.Fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(config.Fields))
	}

	// Should extract the condition threshold
	found := false
	for _, cond := range config.Conditions {
		if cond.Field == "avg_cpu_util" && cond.Threshold == 0.85 {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected condition avg_cpu_util >= 0.85, got %+v", config.Conditions)
	}

	t.Logf("Parsed config: %+v", config)
}

func TestParseReplicationLag(t *testing.T) {
	query := `FROM metrics-mongodb.replstatus-*
| STATS replication_lag=MAX(mongodb.replstatus.lag.max) BY mongodb.replstatus.set_name
| WHERE replication_lag > 10`

	q, err := Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query: %v", err)
	}

	config := ExtractAlertConfig(q)

	// Check field
	if len(config.Fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(config.Fields))
	}

	if config.Fields[0].Function != "MAX" {
		t.Errorf("Expected function MAX, got %s", config.Fields[0].Function)
	}

	// Check condition
	if len(config.Conditions) == 0 {
		t.Fatal("Expected at least one condition")
	}

	if config.Conditions[0].Threshold != 10 {
		t.Errorf("Expected threshold 10, got %v", config.Conditions[0].Threshold)
	}

	// Check trigger values were set
	if config.Fields[0].TriggerValue == nil {
		t.Error("TriggerValue should be set")
	}

	t.Logf("Parsed config: %+v", config)
	t.Logf("Field trigger value: %v", config.Fields[0].TriggerValue)
}

func TestRemoveComments(t *testing.T) {
	query := `// Alert when WiredTiger cache utilization exceeds 85%
// Aggregates per instance
FROM metrics-mongodb.status-*
| STATS cache_used=AVG(mongodb.status.wired_tiger.cache.used.bytes)`

	cleaned := removeComments(query)

	if strings.Contains(cleaned, "//") {
		t.Errorf("Comments should be removed: %s", cleaned)
	}

	if !strings.Contains(cleaned, "FROM") {
		t.Errorf("FROM should be preserved: %s", cleaned)
	}

	if !strings.Contains(cleaned, "STATS") {
		t.Errorf("STATS should be preserved: %s", cleaned)
	}
}

func TestExtractDataStream(t *testing.T) {
	tests := []struct {
		index    string
		expected string
	}{
		{"metrics-mongodb.status-*", "mongodb.status"},
		{"metrics-system.cpu-*", "system.cpu"},
		{"logs-aws.cloudwatch-default", "aws.cloudwatch"},
	}

	for _, tt := range tests {
		got := extractDataStream(tt.index)
		if got != tt.expected {
			t.Errorf("extractDataStream(%s) = %s, want %s", tt.index, got, tt.expected)
		}
	}
}

