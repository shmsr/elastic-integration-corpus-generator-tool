// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package integrations

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

// BenchmarkConfig holds the configuration for benchmark generation
type BenchmarkConfig struct {
	TotalEvents int
	OutputDir   string
	PackageName string
	DataStream  string
}

// BenchmarkGenerator generates benchmark files for a data stream
type BenchmarkGenerator struct {
	fs     afero.Fs
	config BenchmarkConfig
}

// NewBenchmarkGenerator creates a new benchmark generator
func NewBenchmarkGenerator(fs afero.Fs, config BenchmarkConfig) *BenchmarkGenerator {
	return &BenchmarkGenerator{
		fs:     fs,
		config: config,
	}
}

// FieldDefinition represents a field for the benchmark fields.yml
type FieldDefinition struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	Example string `yaml:"example,omitempty"`
}

// ConfigFieldDef represents a field configuration for config.yml
type ConfigFieldDef struct {
	Name        string   `yaml:"name"`
	Cardinality int      `yaml:"cardinality,omitempty"`
	Fuzziness   float64  `yaml:"fuzziness,omitempty"`
	Value       string   `yaml:"value,omitempty"`
	Enum        []string `yaml:"enum,omitempty"`
	Period      string   `yaml:"period,omitempty"`
	Range       *Range   `yaml:"range,omitempty"`
	Counter     bool     `yaml:"counter,omitempty"`
}

// Range represents min/max range for numeric fields
type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

// ConfigFile represents the config.yml structure
type ConfigFile struct {
	Fields []ConfigFieldDef `yaml:"fields"`
}

// RallyConfig represents the rally benchmark configuration file
type RallyConfig struct {
	Description string `yaml:"description"`
	DataStream  struct {
		Name string `yaml:"name"`
	} `yaml:"data_stream"`
	Corpora struct {
		Generator struct {
			TotalEvents int `yaml:"total_events"`
			Template    struct {
				Type string `yaml:"type"`
				Path string `yaml:"path"`
			} `yaml:"template"`
			Config struct {
				Path string `yaml:"path"`
			} `yaml:"config"`
			Fields struct {
				Path string `yaml:"path"`
			} `yaml:"fields"`
		} `yaml:"generator"`
	} `yaml:"corpora"`
}

// Generate creates all benchmark files for a data stream
func (g *BenchmarkGenerator) Generate(ds *DataStream) error {
	benchmarkName := fmt.Sprintf("%s-benchmark", ds.Name)
	benchmarkDir := filepath.Join(g.config.OutputDir, benchmarkName)

	// Create benchmark directory
	if err := g.fs.MkdirAll(benchmarkDir, 0755); err != nil {
		return fmt.Errorf("failed to create benchmark directory: %w", err)
	}

	// Generate files
	if err := g.generateFieldsYAML(ds, benchmarkDir); err != nil {
		return fmt.Errorf("failed to generate fields.yml: %w", err)
	}

	if err := g.generateConfigYAML(ds, benchmarkDir); err != nil {
		return fmt.Errorf("failed to generate config.yml: %w", err)
	}

	if err := g.generateTemplate(ds, benchmarkDir); err != nil {
		return fmt.Errorf("failed to generate template.ndjson: %w", err)
	}

	if err := g.generateRallyConfig(ds, benchmarkName); err != nil {
		return fmt.Errorf("failed to generate rally config: %w", err)
	}

	return nil
}

// generateFieldsYAML creates the fields.yml file
func (g *BenchmarkGenerator) generateFieldsYAML(ds *DataStream, outputDir string) error {
	fields := g.buildFieldDefinitions(ds)

	data, err := yaml.Marshal(fields)
	if err != nil {
		return err
	}

	return afero.WriteFile(g.fs, filepath.Join(outputDir, "fields.yml"), data, 0644)
}

// buildFieldDefinitions converts package fields to benchmark field definitions
func (g *BenchmarkGenerator) buildFieldDefinitions(ds *DataStream) []FieldDefinition {
	// Start with common ECS fields that are always needed
	fields := []FieldDefinition{
		{Name: "@timestamp", Type: "date"},
		// Data stream fields
		{Name: "data_stream.type", Type: "keyword"},
		{Name: "data_stream.dataset", Type: "keyword"},
		{Name: "data_stream.namespace", Type: "keyword"},
		// Host fields
		{Name: "host.name", Type: "keyword"},
		{Name: "host.hostname", Type: "keyword"},
		{Name: "host.ip", Type: "ip"},
	}

	// Detect what ECS field groups are needed based on package fields
	hasCloud := g.hasFieldPrefix(ds.Fields, "cloud.")
	hasContainer := g.hasFieldPrefix(ds.Fields, "container.")
	hasLog := g.hasFieldPrefix(ds.Fields, "log.")
	hasInput := g.hasFieldPrefix(ds.Fields, "input.")

	// Add cloud fields if package uses them or is a cloud package
	if hasCloud || g.isCloudPackage() {
		fields = append(fields,
			FieldDefinition{Name: "cloud.provider", Type: "keyword"},
			FieldDefinition{Name: "cloud.region", Type: "keyword"},
			FieldDefinition{Name: "cloud.availability_zone", Type: "keyword"},
			FieldDefinition{Name: "cloud.account.id", Type: "keyword"},
			FieldDefinition{Name: "cloud.account.name", Type: "keyword"},
			FieldDefinition{Name: "cloud.instance.id", Type: "keyword"},
			FieldDefinition{Name: "cloud.instance.name", Type: "keyword"},
		)
	}

	// Add container fields if needed
	if hasContainer || g.isContainerPackage() {
		fields = append(fields,
			FieldDefinition{Name: "container.id", Type: "keyword"},
			FieldDefinition{Name: "container.name", Type: "keyword"},
			FieldDefinition{Name: "container.image.name", Type: "keyword"},
			FieldDefinition{Name: "container.image.tag", Type: "keyword"},
		)
	}

	// Add log fields if this appears to be a log data stream
	if hasLog || strings.Contains(ds.Name, "log") {
		fields = append(fields,
			FieldDefinition{Name: "log.level", Type: "keyword"},
			FieldDefinition{Name: "log.file.path", Type: "keyword"},
			FieldDefinition{Name: "log.offset", Type: "long"},
		)
	}

	// Add input fields if needed
	if hasInput {
		fields = append(fields,
			FieldDefinition{Name: "input.type", Type: "keyword"},
		)
	}

	// Add fields from the data stream
	for _, f := range ds.Fields {
		// Map package field types to generator types
		genType := mapFieldType(f.Type)
		if genType == "" {
			continue
		}

		example := ""
		if f.Example != nil {
			example = fmt.Sprintf("%v", f.Example)
		}

		fields = append(fields, FieldDefinition{
			Name:    f.Name,
			Type:    genType,
			Example: example,
		})
	}

	// Add common agent/event fields at the end
	commonFields := []FieldDefinition{
		{Name: "agent.id", Type: "keyword"},
		{Name: "agent.name", Type: "keyword"},
		{Name: "agent.type", Type: "keyword"},
		{Name: "agent.version", Type: "keyword"},
		{Name: "agent.ephemeral_id", Type: "keyword", Example: "12f376ef-5186-4e8b-a175-70f1140a8f30"},
		{Name: "ecs.version", Type: "keyword"},
		{Name: "event.dataset", Type: "keyword"},
		{Name: "event.module", Type: "keyword"},
		{Name: "event.duration", Type: "long"},
		{Name: "metricset.period", Type: "long"},
		{Name: "metricset.name", Type: "keyword"},
		{Name: "service.type", Type: "keyword"},
		{Name: "service.address", Type: "keyword"},
	}

	fields = append(fields, commonFields...)

	return fields
}

// hasFieldPrefix checks if any field starts with the given prefix
func (g *BenchmarkGenerator) hasFieldPrefix(fields []PackageField, prefix string) bool {
	for _, f := range fields {
		if strings.HasPrefix(f.Name, prefix) {
			return true
		}
	}
	return false
}

// isCloudPackage returns true if this is a cloud provider package
func (g *BenchmarkGenerator) isCloudPackage() bool {
	cloudPrefixes := []string{"aws", "azure", "gcp", "google_cloud"}
	name := strings.ToLower(g.config.PackageName)
	for _, prefix := range cloudPrefixes {
		if strings.HasPrefix(name, prefix) || name == prefix {
			return true
		}
	}
	return false
}

// isContainerPackage returns true if this is a container/kubernetes package
func (g *BenchmarkGenerator) isContainerPackage() bool {
	containerKeywords := []string{"kubernetes", "docker", "container", "k8s", "ecs", "fargate"}
	name := strings.ToLower(g.config.PackageName)
	for _, kw := range containerKeywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// detectDataNamespaces finds the primary data namespaces from the fields
func (g *BenchmarkGenerator) detectDataNamespaces(fields []PackageField, ecsNamespaces map[string]bool) []string {
	namespaceCount := make(map[string]int)

	for _, f := range fields {
		parts := strings.Split(f.Name, ".")
		if len(parts) > 0 {
			ns := parts[0]
			// Skip ECS namespaces
			if !ecsNamespaces[ns] {
				namespaceCount[ns]++
			}
		}
	}

	// Sort namespaces by count (most fields first)
	type nsCount struct {
		ns    string
		count int
	}
	var sorted []nsCount
	for ns, count := range namespaceCount {
		sorted = append(sorted, nsCount{ns, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	// Return namespace names
	var result []string
	for _, nc := range sorted {
		result = append(result, nc.ns)
	}
	return result
}

// mapFieldType maps Elasticsearch field types to generator types
func mapFieldType(esType string) string {
	typeMap := map[string]string{
		"keyword":          "keyword",
		"text":             "keyword",
		"long":             "long",
		"integer":          "integer",
		"short":            "short",
		"byte":             "byte",
		"double":           "double",
		"float":            "float",
		"half_float":       "float",
		"scaled_float":     "float",
		"date":             "date",
		"boolean":          "boolean",
		"ip":               "ip",
		"geo_point":        "geo_point",
		"constant_keyword": "keyword",
		"flattened":        "object",
		"object":           "object",
		"wildcard":         "keyword",
		"match_only_text":  "keyword",
	}

	if mapped, ok := typeMap[esType]; ok {
		return mapped
	}
	return ""
}

// generateConfigYAML creates the config.yml file with intelligent defaults
func (g *BenchmarkGenerator) generateConfigYAML(ds *DataStream, outputDir string) error {
	config := g.buildConfigFields(ds)

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return afero.WriteFile(g.fs, filepath.Join(outputDir, "config.yml"), data, 0644)
}

// buildConfigFields generates intelligent config for each field
func (g *BenchmarkGenerator) buildConfigFields(ds *DataStream) ConfigFile {
	config := ConfigFile{
		Fields: []ConfigFieldDef{
			// Timestamp with 1 hour period
			{Name: "@timestamp", Period: "60m"},
			// Data stream fields
			{Name: "data_stream.type", Value: "metrics"},
			{Name: "data_stream.dataset", Value: fmt.Sprintf("%s.%s", g.config.PackageName, ds.Name)},
			{Name: "data_stream.namespace", Value: "default"},
			// Host fields
			{Name: "host.name", Value: "host.local"},
			{Name: "host.hostname", Value: "host.local"},
			{Name: "host.ip", Cardinality: 10},
			// Common agent fields with fixed values
			{Name: "agent.id", Value: "12f376ef-5186-4e8b-a175-70f1140a8f30"},
			{Name: "agent.ephemeral_id", Value: "5fd278ce-2a12-4a09-a125-0c5b39aa69e3"},
			{Name: "agent.name", Value: "host.local"},
			{Name: "agent.type", Value: "metricbeat"},
			{Name: "agent.version", Value: "8.12.0"},
			{Name: "ecs.version", Value: "8.11.0"},
			{Name: "event.dataset", Value: fmt.Sprintf("%s.%s", g.config.PackageName, ds.Name)},
			{Name: "event.module", Value: g.config.PackageName},
			{Name: "event.duration", Range: &Range{Min: 1, Max: 1000}},
			{Name: "metricset.period", Value: "60000"},
			{Name: "metricset.name", Value: ds.Name},
			{Name: "service.type", Value: g.config.PackageName},
			{Name: "service.address", Value: "localhost:9200"},
		},
	}

	// Detect what's needed from package fields
	hasCloud := g.hasFieldPrefix(ds.Fields, "cloud.") || g.isCloudPackage()
	hasContainer := g.hasFieldPrefix(ds.Fields, "container.") || g.isContainerPackage()
	hasLog := g.hasFieldPrefix(ds.Fields, "log.") || strings.Contains(ds.Name, "log")
	hasInput := g.hasFieldPrefix(ds.Fields, "input.")

	// Add cloud-specific fields
	if hasCloud {
		if g.isCloudPackage() && strings.HasPrefix(strings.ToLower(g.config.PackageName), "aws") {
			config.Fields = append(config.Fields,
				ConfigFieldDef{
					Name: "cloud.region",
					Enum: []string{
						"us-east-1", "us-east-2", "us-west-1", "us-west-2",
						"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1",
						"ap-northeast-1", "ap-northeast-2", "ap-southeast-1", "ap-southeast-2",
					},
					Cardinality: 12,
				},
				ConfigFieldDef{Name: "cloud.provider", Value: "aws"},
			)
		} else if g.isCloudPackage() && strings.HasPrefix(strings.ToLower(g.config.PackageName), "azure") {
			config.Fields = append(config.Fields,
				ConfigFieldDef{
					Name: "cloud.region",
					Enum: []string{
						"eastus", "eastus2", "westus", "westus2", "centralus",
						"northeurope", "westeurope", "uksouth", "ukwest",
					},
					Cardinality: 9,
				},
				ConfigFieldDef{Name: "cloud.provider", Value: "azure"},
			)
		} else if g.isCloudPackage() && strings.HasPrefix(strings.ToLower(g.config.PackageName), "gcp") {
			config.Fields = append(config.Fields,
				ConfigFieldDef{
					Name: "cloud.region",
					Enum: []string{
						"us-central1", "us-east1", "us-west1", "europe-west1",
						"asia-east1", "asia-southeast1",
					},
					Cardinality: 6,
				},
				ConfigFieldDef{Name: "cloud.provider", Value: "gcp"},
			)
		} else {
			config.Fields = append(config.Fields,
				ConfigFieldDef{Name: "cloud.provider", Cardinality: 5},
				ConfigFieldDef{Name: "cloud.region", Cardinality: 10},
			)
		}
		config.Fields = append(config.Fields,
			ConfigFieldDef{Name: "cloud.account.id", Value: "123456789012"},
			ConfigFieldDef{Name: "cloud.account.name", Value: "sample-account"},
			ConfigFieldDef{Name: "cloud.availability_zone", Cardinality: 3},
			ConfigFieldDef{Name: "cloud.instance.id", Cardinality: 20},
			ConfigFieldDef{Name: "cloud.instance.name", Cardinality: 20},
		)
	}

	// Add container-specific fields
	if hasContainer {
		config.Fields = append(config.Fields,
			ConfigFieldDef{Name: "container.id", Cardinality: 50},
			ConfigFieldDef{Name: "container.name", Cardinality: 20},
			ConfigFieldDef{Name: "container.image.name", Cardinality: 10},
			ConfigFieldDef{Name: "container.image.tag", Enum: []string{"latest", "v1.0", "v1.1", "v2.0"}},
		)
	}

	// Add Kubernetes-specific fields
	if g.isContainerPackage() && strings.Contains(strings.ToLower(g.config.PackageName), "kubernetes") {
		config.Fields = append(config.Fields,
			ConfigFieldDef{Name: "orchestrator.cluster.name", Value: "sample-cluster"},
			ConfigFieldDef{Name: "kubernetes.namespace", Cardinality: 10},
			ConfigFieldDef{Name: "kubernetes.node.name", Cardinality: 5},
			ConfigFieldDef{Name: "kubernetes.pod.name", Cardinality: 50},
			ConfigFieldDef{Name: "kubernetes.pod.uid", Cardinality: 50},
		)
	}

	// Add log fields
	if hasLog {
		config.Fields = append(config.Fields,
			ConfigFieldDef{Name: "log.level", Enum: []string{"debug", "info", "warn", "error", "fatal"}},
			ConfigFieldDef{Name: "log.file.path", Value: "/var/log/app.log"},
			ConfigFieldDef{Name: "log.offset", Range: &Range{Min: 0, Max: 1000000}},
		)
	}

	// Add input fields
	if hasInput {
		config.Fields = append(config.Fields,
			ConfigFieldDef{Name: "input.type", Value: "metrics"},
		)
	}

	// Generate config for each data stream field
	for _, f := range ds.Fields {
		cfg := g.generateFieldConfig(f)
		if cfg != nil {
			config.Fields = append(config.Fields, *cfg)
		}
	}

	return config
}

// generateFieldConfig creates config for a single field based on its type and properties
func (g *BenchmarkGenerator) generateFieldConfig(f PackageField) *ConfigFieldDef {
	cfg := &ConfigFieldDef{Name: f.Name}

	switch f.Type {
	case "long", "integer", "short", "byte":
		if f.MetricType == "counter" {
			cfg.Counter = true
		} else {
			cfg.Range = &Range{Min: 0, Max: 10000}
			cfg.Cardinality = 100
			cfg.Fuzziness = 0.2
		}
	case "double", "float", "half_float", "scaled_float":
		cfg.Range = &Range{Min: 0, Max: 100}
		cfg.Cardinality = 100
		cfg.Fuzziness = 0.2
	case "keyword", "constant_keyword", "text", "wildcard", "match_only_text":
		if f.Dimension {
			cfg.Cardinality = 50
		} else {
			cfg.Cardinality = 100
		}
	case "boolean":
		cfg.Enum = []string{"true", "false"}
	case "ip":
		cfg.Cardinality = 50
	case "date":
		cfg.Period = "60m"
	default:
		return nil // Skip unknown types
	}

	return cfg
}

// jsonNode represents a node in the JSON tree structure
type jsonNode struct {
	children map[string]*jsonNode
	field    *PackageField // nil for intermediate nodes
	isLeaf   bool
}

// generateTemplate creates the GoText template file
func (g *BenchmarkGenerator) generateTemplate(ds *DataStream, outputDir string) error {
	tmpl := g.buildTemplate(ds)
	return afero.WriteFile(g.fs, filepath.Join(outputDir, "template.ndjson"), []byte(tmpl), 0644)
}

// buildTemplate generates a GoText template with proper nested JSON structure
func (g *BenchmarkGenerator) buildTemplate(ds *DataStream) string {
	var buf bytes.Buffer

	// Build a tree structure from field names
	root := &jsonNode{children: make(map[string]*jsonNode)}

	for i := range ds.Fields {
		f := &ds.Fields[i]
		parts := strings.Split(f.Name, ".")
		insertField(root, parts, f)
	}

	// Detect what's needed
	hasCloud := g.hasFieldPrefix(ds.Fields, "cloud.") || g.isCloudPackage()
	hasContainer := g.hasFieldPrefix(ds.Fields, "container.") || g.isContainerPackage()
	hasLog := g.hasFieldPrefix(ds.Fields, "log.") || strings.Contains(ds.Name, "log")

	// Find primary namespaces from actual fields (excluding ECS fields)
	ecsNamespaces := map[string]bool{
		"@timestamp": true, "agent": true, "cloud": true, "container": true,
		"data_stream": true, "ecs": true, "error": true, "event": true,
		"file": true, "host": true, "input": true, "log": true, "message": true,
		"metricset": true, "orchestrator": true, "process": true, "related": true,
		"service": true, "source": true, "tags": true, "url": true, "user": true,
	}
	dataNamespaces := g.detectDataNamespaces(ds.Fields, ecsNamespaces)

	// Generate variable declarations
	buf.WriteString(`{{- $timestamp := generate "@timestamp" }}
`)

	if hasCloud {
		buf.WriteString(`{{- $cloudRegion := generate "cloud.region" }}
{{- $cloudAccountId := generate "cloud.account.id" }}
`)
	}

	// Start JSON object
	buf.WriteString(`{
    "@timestamp": "{{$timestamp.Format "2006-01-02T15:04:05.999999Z07:00"}}",
    "data_stream": {
        "type": "{{generate "data_stream.type"}}",
        "dataset": "{{generate "data_stream.dataset"}}",
        "namespace": "{{generate "data_stream.namespace"}}"
    },
    "host": {
        "name": "{{generate "host.name"}}",
        "hostname": "{{generate "host.hostname"}}",
        "ip": ["{{generate "host.ip"}}"]
    },
`)

	// Generate cloud section if needed
	if hasCloud {
		buf.WriteString(`    "cloud": {
        "provider": "{{generate "cloud.provider"}}",
        "region": "{{$cloudRegion}}",
        "availability_zone": "{{generate "cloud.availability_zone"}}",
        "account": {
            "id": "{{$cloudAccountId}}",
            "name": "{{generate "cloud.account.name"}}"
        },
        "instance": {
            "id": "{{generate "cloud.instance.id"}}",
            "name": "{{generate "cloud.instance.name"}}"
        }
    },
`)
	}

	// Generate container section if needed
	if hasContainer {
		buf.WriteString(`    "container": {
        "id": "{{generate "container.id"}}",
        "name": "{{generate "container.name"}}",
        "image": {
            "name": "{{generate "container.image.name"}}",
            "tag": "{{generate "container.image.tag"}}"
        }
    },
`)
	}

	// Generate event section
	buf.WriteString(fmt.Sprintf(`    "event": {
        "dataset": "%s.%s",
        "module": "%s",
        "duration": {{generate "event.duration"}}
    },
    "metricset": {
        "name": "%s",
        "period": {{generate "metricset.period"}}
    },
    "ecs": {
        "version": "{{generate "ecs.version"}}"
    },
`, g.config.PackageName, ds.Name, g.config.PackageName, ds.Name))

	// Generate log section if needed
	if hasLog {
		buf.WriteString(`    "log": {
        "level": "{{generate "log.level"}}",
        "file": {
            "path": "{{generate "log.file.path"}}"
        },
        "offset": {{generate "log.offset"}}
    },
`)
	}

	// Generate main data sections based on detected namespaces
	// This handles cases where package name differs from field namespace (e.g., aws_mq vs aws.amazonmq)
	for _, ns := range dataNamespaces {
		if node, ok := root.children[ns]; ok {
			// Always add comma since service/agent sections follow
			g.writeJSONNode(&buf, ns, node, "    ", true)
		}
	}

	// Generate service section
	buf.WriteString(fmt.Sprintf(`    "service": {
        "type": "%s",
        "address": "{{generate "service.address"}}"
    },
`, g.config.PackageName))

	// Generate agent section
	buf.WriteString(`    "agent": {
        "id": "{{generate "agent.id"}}",
        "name": "{{generate "agent.name"}}",
        "type": "{{generate "agent.type"}}",
        "version": "{{generate "agent.version"}}",
        "ephemeral_id": "{{generate "agent.ephemeral_id"}}"
    }
}
`)

	return buf.String()
}

// insertField inserts a field into the JSON tree structure
func insertField(node *jsonNode, parts []string, field *PackageField) {
	if len(parts) == 0 {
		return
	}

	key := parts[0]
	if _, exists := node.children[key]; !exists {
		node.children[key] = &jsonNode{children: make(map[string]*jsonNode)}
	}

	if len(parts) == 1 {
		node.children[key].field = field
		node.children[key].isLeaf = true
	} else {
		insertField(node.children[key], parts[1:], field)
	}
}

// writeJSONNode recursively writes a JSON node to the buffer
func (g *BenchmarkGenerator) writeJSONNode(buf *bytes.Buffer, name string, node *jsonNode, indent string, hasMore bool) {
	// Get sorted keys for consistent output
	keys := make([]string, 0, len(node.children))
	for k := range node.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// If this is a leaf node with a field
	if node.isLeaf && node.field != nil {
		g.writeFieldValue(buf, name, node.field, indent, hasMore)
		return
	}

	// If this node has children, write as nested object
	if len(keys) > 0 {
		buf.WriteString(fmt.Sprintf(`%s"%s": {
`, indent, name))

		for i, key := range keys {
			child := node.children[key]
			isLast := i == len(keys)-1
			g.writeJSONNode(buf, key, child, indent+"    ", !isLast)
		}

		comma := ""
		if hasMore {
			comma = ","
		}
		buf.WriteString(fmt.Sprintf(`%s}%s
`, indent, comma))
	}
}

// writeFieldValue writes a single field value
func (g *BenchmarkGenerator) writeFieldValue(buf *bytes.Buffer, name string, field *PackageField, indent string, hasMore bool) {
	comma := ""
	if hasMore {
		comma = ","
	}

	fullName := field.Name

	switch field.Type {
	case "long", "integer", "short", "byte", "double", "float", "half_float", "scaled_float":
		buf.WriteString(fmt.Sprintf(`%s"%s": {{generate "%s"}}%s
`, indent, name, fullName, comma))
	case "boolean":
		buf.WriteString(fmt.Sprintf(`%s"%s": {{generate "%s"}}%s
`, indent, name, fullName, comma))
	default:
		buf.WriteString(fmt.Sprintf(`%s"%s": "{{generate "%s"}}"%s
`, indent, name, fullName, comma))
	}
}

// generateRallyConfig creates the rally benchmark configuration file
func (g *BenchmarkGenerator) generateRallyConfig(ds *DataStream, benchmarkName string) error {
	config := RallyConfig{
		Description: fmt.Sprintf("Benchmark %d %s.%s events ingested",
			g.config.TotalEvents, g.config.PackageName, ds.Name),
	}
	config.DataStream.Name = ds.Name
	config.Corpora.Generator.TotalEvents = g.config.TotalEvents
	config.Corpora.Generator.Template.Type = "gotext"
	config.Corpora.Generator.Template.Path = fmt.Sprintf("./%s/template.ndjson", benchmarkName)
	config.Corpora.Generator.Config.Path = fmt.Sprintf("./%s/config.yml", benchmarkName)
	config.Corpora.Generator.Fields.Path = fmt.Sprintf("./%s/fields.yml", benchmarkName)

	// Custom marshal to add --- header
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	content := "---\n" + string(data)
	return afero.WriteFile(g.fs, filepath.Join(g.config.OutputDir, benchmarkName+".yml"), []byte(content), 0644)
}
