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
	"text/template"

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
	// Start with common ECS fields
	fields := []FieldDefinition{
		{Name: "@timestamp", Type: "date"},
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

	// Add common agent fields
	commonFields := []FieldDefinition{
		{Name: "agent.id", Type: "keyword"},
		{Name: "agent.name", Type: "keyword"},
		{Name: "agent.ephemeral_id", Type: "keyword", Example: "12f376ef-5186-4e8b-a175-70f1140a8f30"},
		{Name: "event.dataset", Type: "keyword"},
		{Name: "event.module", Type: "keyword"},
		{Name: "event.duration", Type: "long"},
		{Name: "metricset.period", Type: "long"},
		{Name: "metricset.name", Type: "keyword"},
	}

	fields = append(fields, commonFields...)

	return fields
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
			// Common agent fields with fixed values
			{Name: "agent.id", Value: "12f376ef-5186-4e8b-a175-70f1140a8f30"},
			{Name: "agent.ephemeral_id", Value: "5fd278ce-2a12-4a09-a125-0c5b39aa69e3"},
			{Name: "agent.name", Value: "host.local"},
			{Name: "event.dataset", Value: fmt.Sprintf("%s.%s", g.config.PackageName, ds.Name)},
			{Name: "event.module", Value: g.config.PackageName},
			{Name: "event.duration", Range: &Range{Min: 1, Max: 1000}},
			{Name: "metricset.period", Value: "60000"},
			{Name: "metricset.name", Value: ds.Name},
		},
	}

	// Add AWS-specific fields if this is an AWS package
	if strings.HasPrefix(g.config.PackageName, "aws") || g.config.PackageName == "aws" {
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
			ConfigFieldDef{Name: "cloud.account.id", Value: "123456789012"},
			ConfigFieldDef{Name: "cloud.account.name", Value: "sample-account"},
			ConfigFieldDef{Name: "cloud.provider", Value: "aws"},
		)
	}

	// Add Kubernetes-specific fields
	if strings.HasPrefix(g.config.PackageName, "kubernetes") || g.config.PackageName == "kubernetes" {
		config.Fields = append(config.Fields,
			ConfigFieldDef{Name: "orchestrator.cluster.name", Value: "sample-cluster"},
			ConfigFieldDef{Name: "kubernetes.namespace", Cardinality: 10, Fuzziness: 0.1},
			ConfigFieldDef{Name: "kubernetes.node.name", Cardinality: 5, Fuzziness: 0.1},
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
	case "keyword", "constant_keyword":
		if f.Dimension {
			cfg.Cardinality = 50
		} else {
			cfg.Cardinality = 100
		}
	case "boolean":
		cfg.Enum = []string{"true", "false"}
	case "ip":
		cfg.Cardinality = 50
	default:
		return nil // Skip unknown types
	}

	return cfg
}

// generateTemplate creates the GoText template file
func (g *BenchmarkGenerator) generateTemplate(ds *DataStream, outputDir string) error {
	tmpl := g.buildTemplate(ds)
	return afero.WriteFile(g.fs, filepath.Join(outputDir, "template.ndjson"), []byte(tmpl), 0644)
}

// TemplateData holds data for template generation
type TemplateData struct {
	PackageName    string
	DataStreamName string
	Fields         []TemplateField
	HasCloud       bool
	HasKubernetes  bool
}

// TemplateField represents a field for template generation
type TemplateField struct {
	Name      string
	Type      string
	IsNumeric bool
	IsDate    bool
	JSONPath  []string
	VarName   string
}

// buildTemplate generates a GoText template
func (g *BenchmarkGenerator) buildTemplate(ds *DataStream) string {
	// Organize fields by their JSON path for proper nesting
	fieldsByPrefix := make(map[string][]PackageField)

	for _, f := range ds.Fields {
		parts := strings.Split(f.Name, ".")
		if len(parts) > 1 {
			prefix := parts[0]
			fieldsByPrefix[prefix] = append(fieldsByPrefix[prefix], f)
		} else {
			fieldsByPrefix["_root"] = append(fieldsByPrefix["_root"], f)
		}
	}

	var buf bytes.Buffer

	// Generate variable declarations
	buf.WriteString(`{{- $timestamp := generate "@timestamp" }}
`)

	// Check for cloud fields
	hasCloud := len(fieldsByPrefix["cloud"]) > 0 || strings.HasPrefix(g.config.PackageName, "aws")
	if hasCloud {
		buf.WriteString(`{{- $cloudRegion := generate "cloud.region" }}
{{- $cloudAccountId := generate "cloud.account.id" }}
`)
	}

	// Start JSON object
	buf.WriteString(`{
    "@timestamp": "{{$timestamp.Format "2006-01-02T15:04:05.999999Z07:00"}}",
`)

	// Generate cloud section if needed
	if hasCloud {
		buf.WriteString(`    "cloud": {
        "provider": "{{generate "cloud.provider"}}",
        "region": "{{$cloudRegion}}",
        "account": {
            "id": "{{$cloudAccountId}}",
            "name": "{{generate "cloud.account.name"}}"
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
        "version": "8.11.0"
    },
`, g.config.PackageName, ds.Name, g.config.PackageName, ds.Name))

	// Generate main data section based on package name
	mainSection := g.config.PackageName
	if mainSection == "aws" {
		// For AWS, use aws.<datastream> structure
		buf.WriteString(fmt.Sprintf(`    "aws": {
        "%s": {
`, ds.Name))
		g.writeFieldsSection(&buf, fieldsByPrefix[g.config.PackageName], "            ", true)
		buf.WriteString(`        }
    },
`)
	} else if mainSection == "kubernetes" {
		// For Kubernetes
		buf.WriteString(fmt.Sprintf(`    "kubernetes": {
        "%s": {
`, ds.Name))
		g.writeFieldsSection(&buf, fieldsByPrefix[mainSection], "            ", true)
		buf.WriteString(`        }
    },
`)
	} else {
		// Generic section
		if fields, ok := fieldsByPrefix[mainSection]; ok && len(fields) > 0 {
			buf.WriteString(fmt.Sprintf(`    "%s": {
`, mainSection))
			g.writeFieldsSection(&buf, fields, "        ", true)
			buf.WriteString(`    },
`)
		}
	}

	// Generate service section
	buf.WriteString(fmt.Sprintf(`    "service": {
        "type": "%s"
    },
`, g.config.PackageName))

	// Generate agent section
	buf.WriteString(`    "agent": {
        "id": "{{generate "agent.id"}}",
        "name": "{{generate "agent.name"}}",
        "type": "metricbeat",
        "version": "8.0.0",
        "ephemeral_id": "{{generate "agent.ephemeral_id"}}"
    }
}
`)

	return buf.String()
}

// writeFieldsSection writes fields as JSON
func (g *BenchmarkGenerator) writeFieldsSection(buf *bytes.Buffer, fields []PackageField, indent string, isLast bool) {
	if len(fields) == 0 {
		return
	}

	// Sort fields for consistent output
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})

	for i, f := range fields {
		// Get the short name (after last dot)
		parts := strings.Split(f.Name, ".")
		shortName := parts[len(parts)-1]

		isLastField := i == len(fields)-1

		switch f.Type {
		case "long", "integer", "short", "byte", "double", "float", "half_float", "scaled_float":
			g.writeNumericField(buf, f.Name, shortName, indent, !isLastField)
		case "boolean":
			g.writeBooleanField(buf, f.Name, shortName, indent, !isLastField)
		default:
			g.writeStringField(buf, f.Name, shortName, indent, !isLastField)
		}
	}
}

func (g *BenchmarkGenerator) writeNumericField(buf *bytes.Buffer, fullName, shortName, indent string, hasMore bool) {
	comma := ""
	if hasMore {
		comma = ","
	}
	buf.WriteString(fmt.Sprintf(`%s"%s": {{generate "%s"}}%s
`, indent, shortName, fullName, comma))
}

func (g *BenchmarkGenerator) writeBooleanField(buf *bytes.Buffer, fullName, shortName, indent string, hasMore bool) {
	comma := ""
	if hasMore {
		comma = ","
	}
	buf.WriteString(fmt.Sprintf(`%s"%s": {{generate "%s"}}%s
`, indent, shortName, fullName, comma))
}

func (g *BenchmarkGenerator) writeStringField(buf *bytes.Buffer, fullName, shortName, indent string, hasMore bool) {
	comma := ""
	if hasMore {
		comma = ","
	}
	buf.WriteString(fmt.Sprintf(`%s"%s": "{{generate "%s"}}"%s
`, indent, shortName, fullName, comma))
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

// GenerateTemplateFull creates a complete template with proper JSON structure
func GenerateTemplateFull(pkg *Package, ds *DataStream) (string, error) {
	tmpl := `{{- $timestamp := generate "@timestamp" -}}
{
    "@timestamp": "{{$timestamp.Format "2006-01-02T15:04:05.999999Z07:00"}}",
{{- range $i, $section := .Sections }}
    "{{ $section.Name }}": {
{{- range $j, $field := $section.Fields }}
        "{{ $field.ShortName }}": {{ $field.Generator }}{{ if not $field.IsLast }},{{ end }}
{{- end }}
    }{{ if not $section.IsLast }},{{ end }}
{{- end }}
}
`

	t, err := template.New("benchmark").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, struct {
		Sections []interface{}
	}{})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
