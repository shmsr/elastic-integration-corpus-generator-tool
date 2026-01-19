// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/integrations"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// GenerateBenchmarkCmd returns the generate-benchmark command
func GenerateBenchmarkCmd() *cobra.Command {
	var packagePath string
	var dataStream string
	var outputDir string
	var outputMode string
	var allStreams bool
	var totalEvents int

	cmd := &cobra.Command{
		Use:   "generate-benchmark",
		Short: "Generate rally benchmark files from an integration package",
		Long: `Generate rally benchmark files (fields.yml, config.yml, template.ndjson) 
from an existing integration package's field definitions.

This command reads the package's data stream fields and automatically generates
benchmark configuration files that can be used with Rally for performance testing.

Example:
  # Generate benchmark for a specific data stream
  elastic-integration-corpus-generator-tool generate-benchmark \
    --package-path /path/to/integrations/packages/aws \
    --data-stream billing \
    --output-dir ./_dev/benchmark/rally/

  # Generate benchmarks for all data streams in a package
  elastic-integration-corpus-generator-tool generate-benchmark \
    --package-path /path/to/integrations/packages/kubernetes \
    --all \
    --output-dir ./_dev/benchmark/rally/

  # Always write to the package's _dev/benchmark/rally directory
  elastic-integration-corpus-generator-tool generate-benchmark \
    --package-path /path/to/integrations/packages/kubernetes \
    --all \
    --output-mode package
`,
		RunE: func(c *cobra.Command, args []string) error {
			if packagePath == "" {
				return fmt.Errorf("--package-path is required")
			}

			if !allStreams && dataStream == "" {
				return fmt.Errorf("--data-stream or --all is required")
			}

			fs := afero.NewOsFs()

			// Parse the package
			pkg, err := integrations.ParsePackage(fs, packagePath)
			if err != nil {
				return fmt.Errorf("failed to parse package: %w", err)
			}

			fmt.Printf("📦 Package: %s (%s)\n", pkg.Manifest.Title, pkg.Manifest.Name)
			fmt.Printf("📊 Data streams found: %d\n", len(pkg.DataStreams))

			// Determine output directory
			if outputMode == "" {
				outputMode = "flat"
			}
			switch outputMode {
			case "flat":
				if outputDir == "" {
					outputDir = filepath.Join(packagePath, "_dev", "benchmark", "rally")
				}
			case "package":
				outputDir = filepath.Join(packagePath, "_dev", "benchmark", "rally")
			default:
				return fmt.Errorf("invalid --output-mode '%s' (valid: flat, package)", outputMode)
			}

			// Create output directory
			if err := fs.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Generate benchmarks
			var streamsToGenerate []string
			if allStreams {
				streamsToGenerate = pkg.ListDataStreams()
			} else {
				streamsToGenerate = strings.Split(dataStream, ",")
			}

			generated := 0
			for _, dsName := range streamsToGenerate {
				dsName = strings.TrimSpace(dsName)
				ds := pkg.GetDataStream(dsName)
				if ds == nil {
					fmt.Printf("⚠️  Data stream '%s' not found, skipping\n", dsName)
					continue
				}

				if len(ds.Fields) == 0 {
					fmt.Printf("⚠️  Data stream '%s' has no fields, skipping\n", dsName)
					continue
				}

				config := integrations.BenchmarkConfig{
					TotalEvents: totalEvents,
					OutputDir:   outputDir,
					PackageName: pkg.Manifest.Name,
					DataStream:  dsName,
				}

				generator := integrations.NewBenchmarkGenerator(fs, config)
				if err := generator.Generate(ds); err != nil {
					fmt.Printf("❌ Failed to generate benchmark for '%s': %v\n", dsName, err)
					continue
				}

				generated++
				fmt.Printf("✅ Generated benchmark for '%s' (%d fields)\n", dsName, len(ds.Fields))
			}

			fmt.Printf("\n🎉 Generated %d benchmark(s) in %s\n", generated, outputDir)
			return nil
		},
	}

	cmd.Flags().StringVarP(&packagePath, "package-path", "p", "", "Path to the integration package (required)")
	cmd.Flags().StringVarP(&dataStream, "data-stream", "d", "", "Data stream name to generate benchmark for")
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "Output directory (defaults to <package>/_dev/benchmark/rally/)")
	cmd.Flags().StringVar(&outputMode, "output-mode", "flat", "Output mode: flat or package")
	cmd.Flags().BoolVarP(&allStreams, "all", "a", false, "Generate benchmarks for all data streams")
	cmd.Flags().IntVarP(&totalEvents, "total-events", "n", 20000, "Total events to generate in benchmark")

	return cmd
}
