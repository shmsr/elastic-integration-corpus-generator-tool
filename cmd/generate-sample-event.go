// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/genlib"
	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/genlib/config"
	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/genlib/fields"
	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/integrations"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// GenerateSampleEventCmd returns the generate-sample-event command
func GenerateSampleEventCmd() *cobra.Command {
	var packagePath string
	var dataStream string
	var outputFile string
	var outputMode string
	var templatePath string
	var fieldsPath string
	var configPath string
	var prettyPrint bool

	cmd := &cobra.Command{
		Use:   "generate-sample-event",
		Short: "Generate a sample_event.json for a data stream",
		Long: `Generate a sample_event.json file from a benchmark template or package fields.

This command creates a realistic sample event that can be used for documentation
and testing in integration packages.

Example:
  # Generate from package (auto-creates temporary benchmark files)
  elastic-integration-corpus-generator-tool generate-sample-event \
    --package-path /path/to/packages/aws \
    --data-stream billing

  # Generate and write to package directory
  elastic-integration-corpus-generator-tool generate-sample-event \
    --package-path /path/to/packages/aws \
    --data-stream billing \
    --output-mode package

  # Generate from existing benchmark files
  elastic-integration-corpus-generator-tool generate-sample-event \
    --template ./template.ndjson \
    --fields ./fields.yml \
    --config ./config.yml

  # Output to specific file
  elastic-integration-corpus-generator-tool generate-sample-event \
    --package-path /path/to/packages/aws \
    --data-stream billing \
    --output ./sample_event.json
`,
		RunE: func(c *cobra.Command, args []string) error {
			fs := afero.NewOsFs()

			// Determine mode: from package or from existing files
			if templatePath != "" && fieldsPath != "" {
				// Use existing benchmark files
				return generateFromFiles(fs, templatePath, fieldsPath, configPath, outputFile, prettyPrint)
			}

			if packagePath == "" {
				return fmt.Errorf("--package-path is required (or use --template and --fields)")
			}
			if dataStream == "" {
				return fmt.Errorf("--data-stream is required")
			}

			return generateFromPackage(fs, packagePath, dataStream, outputFile, outputMode, prettyPrint)
		},
	}

	cmd.Flags().StringVarP(&packagePath, "package-path", "p", "", "Path to the integration package")
	cmd.Flags().StringVarP(&dataStream, "data-stream", "d", "", "Data stream name")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file path (default: stdout)")
	cmd.Flags().StringVar(&outputMode, "output-mode", "", "Output mode: 'package' writes to data_stream/<name>/sample_event.json")
	cmd.Flags().StringVarP(&templatePath, "template", "t", "", "Path to existing template.ndjson")
	cmd.Flags().StringVarP(&fieldsPath, "fields", "f", "", "Path to existing fields.yml")
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to existing config.yml")
	cmd.Flags().BoolVar(&prettyPrint, "pretty", true, "Pretty-print JSON output")

	return cmd
}

func generateFromPackage(fs afero.Fs, packagePath, dataStreamName, outputFile, outputMode string, prettyPrint bool) error {
	// Parse package
	pkg, err := integrations.ParsePackage(fs, packagePath)
	if err != nil {
		return fmt.Errorf("failed to parse package: %w", err)
	}

	ds := pkg.GetDataStream(dataStreamName)
	if ds == nil {
		return fmt.Errorf("data stream '%s' not found", dataStreamName)
	}

	if len(ds.Fields) == 0 {
		return fmt.Errorf("data stream '%s' has no fields", dataStreamName)
	}

	// Create temporary benchmark files
	tmpDir, err := os.MkdirTemp("", "sample-event-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	benchConfig := integrations.BenchmarkConfig{
		TotalEvents: 1,
		OutputDir:   tmpDir,
		PackageName: pkg.Manifest.Name,
		DataStream:  dataStreamName,
	}

	generator := integrations.NewBenchmarkGenerator(fs, benchConfig)
	if err := generator.Generate(ds); err != nil {
		return fmt.Errorf("failed to generate benchmark files: %w", err)
	}

	benchmarkDir := filepath.Join(tmpDir, dataStreamName+"-benchmark")
	templateFile := filepath.Join(benchmarkDir, "template.ndjson")
	fieldsFile := filepath.Join(benchmarkDir, "fields.yml")
	configFile := filepath.Join(benchmarkDir, "config.yml")

	// Determine output path
	if outputMode == "package" {
		outputFile = filepath.Join(packagePath, "data_stream", dataStreamName, "sample_event.json")
	}

	return generateFromFiles(fs, templateFile, fieldsFile, configFile, outputFile, prettyPrint)
}

func generateFromFiles(fs afero.Fs, templatePath, fieldsPath, configPath, outputFile string, prettyPrint bool) error {
	// Load template
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	// Load fields
	flds, err := fields.LoadFieldsWithTemplate(nil, fieldsPath)
	if err != nil {
		return fmt.Errorf("failed to load fields: %w", err)
	}

	// Load config (optional)
	var cfg config.Config
	if configPath != "" {
		cfg, err = config.LoadConfig(fs, configPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Initialize generator
	genlib.InitGeneratorTimeNow(time.Now())
	genlib.InitGeneratorRandSeed(42) // Fixed seed for reproducible output

	evgen, err := genlib.NewGeneratorWithTextTemplate(templateData, cfg, flds, 1, 42)
	if err != nil {
		return fmt.Errorf("failed to create generator: %w", err)
	}
	defer evgen.Close()

	// Generate single event
	var buf bytes.Buffer
	if err := evgen.Emit(&buf); err != nil {
		return fmt.Errorf("failed to generate event: %w", err)
	}

	// Parse and optionally pretty-print
	eventJSON := buf.Bytes()
	if prettyPrint {
		var parsed map[string]interface{}
		if err := json.Unmarshal(eventJSON, &parsed); err != nil {
			return fmt.Errorf("generated event is not valid JSON: %w", err)
		}
		eventJSON, err = json.MarshalIndent(parsed, "", "    ")
		if err != nil {
			return fmt.Errorf("failed to format JSON: %w", err)
		}
	}

	// Output
	if outputFile == "" {
		fmt.Println(string(eventJSON))
		return nil
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(outputFile, append(eventJSON, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	// Show relative path if possible
	displayPath := outputFile
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, outputFile); err == nil && !strings.HasPrefix(rel, "..") {
			displayPath = rel
		}
	}

	fmt.Printf("✅ Generated sample event: %s\n", displayPath)
	return nil
}
