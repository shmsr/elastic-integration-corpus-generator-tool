// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"fmt"
	"strings"

	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/integrations"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// ValidateECSCmd returns the validate-ecs command
func ValidateECSCmd() *cobra.Command {
	var packagePath string
	var dataStream string
	var allStreams bool
	var jsonOutput bool
	var ecsVersion string

	cmd := &cobra.Command{
		Use:   "validate-ecs",
		Short: "Validate package fields against the Elastic Common Schema (ECS)",
		Long: `Validate that package field definitions follow ECS naming conventions
and type definitions.

This command checks:
- Field name matches ECS where applicable
- Field types are compatible with ECS definitions
- Custom fields follow snake_case naming conventions
- Custom fields are properly namespaced

Example:
  # Validate a specific data stream
  elastic-integration-corpus-generator-tool validate-ecs \
    --package-path /path/to/packages/aws \
    --data-stream billing

  # Validate all data streams in a package
  elastic-integration-corpus-generator-tool validate-ecs \
    --package-path /path/to/packages/kubernetes \
    --all
`,
		RunE: func(c *cobra.Command, args []string) error {
			if packagePath == "" {
				return fmt.Errorf("--package-path is required")
			}
			if !allStreams && dataStream == "" {
				return fmt.Errorf("--data-stream or --all is required")
			}

			fs := afero.NewOsFs()

			// Parse package
			pkg, err := integrations.ParsePackage(fs, packagePath)
			if err != nil {
				return fmt.Errorf("failed to parse package: %w", err)
			}

			fmt.Printf("📦 Package: %s (%s)\n\n", pkg.Manifest.Title, pkg.Manifest.Name)

			validator := integrations.NewECSValidator(ecsVersion)

			// Determine which streams to validate
			var streamsToValidate []string
			if allStreams {
				streamsToValidate = pkg.ListDataStreams()
			} else {
				streamsToValidate = strings.Split(dataStream, ",")
			}

			allValid := true
			for _, dsName := range streamsToValidate {
				dsName = strings.TrimSpace(dsName)
				ds := pkg.GetDataStream(dsName)
				if ds == nil {
					fmt.Printf("⚠️  Data stream '%s' not found, skipping\n", dsName)
					continue
				}

				result, err := validator.Validate(ds)
				if err != nil {
					fmt.Printf("❌ Failed to validate '%s': %v\n", dsName, err)
					continue
				}

				if jsonOutput {
					json, _ := result.FormatJSON()
					fmt.Println(json)
				} else {
					fmt.Print(result.FormatResult())
					fmt.Println()
				}

				if !result.IsValid {
					allValid = false
				}
			}

			if !allValid {
				return fmt.Errorf("some data streams have ECS validation issues")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&packagePath, "package-path", "p", "", "Path to the integration package (required)")
	cmd.Flags().StringVarP(&dataStream, "data-stream", "d", "", "Data stream name(s) to validate (comma-separated)")
	cmd.Flags().BoolVarP(&allStreams, "all", "a", false, "Validate all data streams")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&ecsVersion, "ecs-version", "8.11.0", "ECS version to validate against")

	return cmd
}
