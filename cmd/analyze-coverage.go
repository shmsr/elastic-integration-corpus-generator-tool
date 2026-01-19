// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/integrations"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// AnalyzeCoverageCmd returns the analyze-coverage command
func AnalyzeCoverageCmd() *cobra.Command {
	var packagePath string
	var dataStream string
	var templatePath string
	var samplePath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "analyze-coverage",
		Short: "Analyze field coverage in benchmark templates or sample events",
		Long: `Analyze how many package fields are covered by a benchmark template
or sample event file.

This helps identify:
- Missing fields that should be added to the template
- Extra fields in the template not defined in the package
- Overall field coverage percentage

Example:
  # Analyze a template
  elastic-integration-corpus-generator-tool analyze-coverage \
    --package-path /path/to/packages/aws \
    --data-stream billing \
    --template ./_dev/benchmark/rally/billing-benchmark/template.ndjson

  # Analyze a sample event
  elastic-integration-corpus-generator-tool analyze-coverage \
    --package-path /path/to/packages/aws \
    --data-stream billing \
    --sample-event ./data_stream/billing/sample_event.json
`,
		RunE: func(c *cobra.Command, args []string) error {
			if packagePath == "" {
				return fmt.Errorf("--package-path is required")
			}
			if dataStream == "" {
				return fmt.Errorf("--data-stream is required")
			}
			if templatePath == "" && samplePath == "" {
				return fmt.Errorf("--template or --sample-event is required")
			}

			fs := afero.NewOsFs()

			// Parse package
			pkg, err := integrations.ParsePackage(fs, packagePath)
			if err != nil {
				return fmt.Errorf("failed to parse package: %w", err)
			}

			ds := pkg.GetDataStream(dataStream)
			if ds == nil {
				return fmt.Errorf("data stream '%s' not found in package", dataStream)
			}

			analyzer := integrations.NewCoverageAnalyzer(fs)

			var report *integrations.CoverageReport

			if templatePath != "" {
				// Make path absolute if relative
				if !filepath.IsAbs(templatePath) {
					templatePath = filepath.Join(packagePath, templatePath)
				}
				report, err = analyzer.AnalyzeTemplate(ds, templatePath)
			} else {
				// Make path absolute if relative
				if !filepath.IsAbs(samplePath) {
					samplePath = filepath.Join(packagePath, samplePath)
				}
				report, err = analyzer.AnalyzeSampleEvent(ds, samplePath)
			}

			if err != nil {
				return fmt.Errorf("analysis failed: %w", err)
			}

			report.PackageName = pkg.Manifest.Name

			if jsonOutput {
				json, err := report.FormatJSON()
				if err != nil {
					return err
				}
				fmt.Println(json)
			} else {
				fmt.Print(report.FormatReport())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&packagePath, "package-path", "p", "", "Path to the integration package (required)")
	cmd.Flags().StringVarP(&dataStream, "data-stream", "d", "", "Data stream name (required)")
	cmd.Flags().StringVarP(&templatePath, "template", "t", "", "Path to template.ndjson to analyze")
	cmd.Flags().StringVarP(&samplePath, "sample-event", "s", "", "Path to sample_event.json to analyze")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}
