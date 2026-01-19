// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elastic/elastic-integration-corpus-generator-tool/pkg/integrations"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// ListPackageCmd returns the list-package command
func ListPackageCmd() *cobra.Command {
	var packagePath string
	var dataStream string
	var showFields bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list-package",
		Short: "List information about an integration package",
		Long: `Display information about an integration package including its
data streams, fields, and benchmark status.

This is useful for exploring packages before generating benchmarks.

Example:
  # List all data streams in a package
  elastic-integration-corpus-generator-tool list-package \
    --package-path /path/to/packages/aws

  # Show detailed field information for a data stream
  elastic-integration-corpus-generator-tool list-package \
    --package-path /path/to/packages/aws \
    --data-stream billing \
    --fields

  # Output as JSON
  elastic-integration-corpus-generator-tool list-package \
    --package-path /path/to/packages/kubernetes \
    --json
`,
		RunE: func(c *cobra.Command, args []string) error {
			if packagePath == "" {
				return fmt.Errorf("--package-path is required")
			}

			fs := afero.NewOsFs()

			// Parse package
			pkg, err := integrations.ParsePackage(fs, packagePath)
			if err != nil {
				return fmt.Errorf("failed to parse package: %w", err)
			}

			if jsonOutput {
				return outputPackageAsJSON(pkg, dataStream, showFields)
			}

			return outputPackageAsText(pkg, dataStream, showFields)
		},
	}

	cmd.Flags().StringVarP(&packagePath, "package-path", "p", "", "Path to the integration package (required)")
	cmd.Flags().StringVarP(&dataStream, "data-stream", "d", "", "Show details for a specific data stream")
	cmd.Flags().BoolVarP(&showFields, "fields", "f", false, "Show field details")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func outputPackageAsText(pkg *integrations.Package, dataStream string, showFields bool) error {
	fmt.Println("📦 Package Information")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Name:    %s\n", pkg.Manifest.Name)
	fmt.Printf("Title:   %s\n", pkg.Manifest.Title)
	fmt.Printf("Version: %s\n", pkg.Manifest.Version)
	if pkg.Manifest.Owner.Github != "" {
		fmt.Printf("Owner:   %s\n", pkg.Manifest.Owner.Github)
	}
	fmt.Println()

	if dataStream != "" {
		ds := pkg.GetDataStream(dataStream)
		if ds == nil {
			return fmt.Errorf("data stream '%s' not found", dataStream)
		}

		fmt.Printf("📊 Data Stream: %s\n", ds.Name)
		fmt.Println(strings.Repeat("-", 40))
		fmt.Printf("Fields: %d\n", len(ds.Fields))

		if showFields {
			fmt.Println("\nField List:")
			for _, f := range ds.Fields {
				fmt.Printf("  • %s (%s)\n", f.Name, f.Type)
				if f.Description != "" {
					fmt.Printf("    %s\n", truncateString(f.Description, 60))
				}
			}
		}
	} else {
		fmt.Printf("📊 Data Streams (%d total)\n", len(pkg.DataStreams))
		fmt.Println(strings.Repeat("-", 40))

		for _, ds := range pkg.DataStreams {
			fmt.Printf("  • %s (%d fields)\n", ds.Name, len(ds.Fields))
		}

		fmt.Println()
		fmt.Println("💡 Tip: Use --data-stream <name> --fields to see field details")
	}

	return nil
}

func outputPackageAsJSON(pkg *integrations.Package, dataStream string, showFields bool) error {
	type fieldInfo struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description,omitempty"`
	}

	type dataStreamInfo struct {
		Name       string      `json:"name"`
		FieldCount int         `json:"field_count"`
		Fields     []fieldInfo `json:"fields,omitempty"`
	}

	type packageInfo struct {
		Name        string           `json:"name"`
		Title       string           `json:"title"`
		Version     string           `json:"version"`
		Owner       string           `json:"owner,omitempty"`
		DataStreams []dataStreamInfo `json:"data_streams"`
	}

	info := packageInfo{
		Name:        pkg.Manifest.Name,
		Title:       pkg.Manifest.Title,
		Version:     pkg.Manifest.Version,
		Owner:       pkg.Manifest.Owner.Github,
		DataStreams: []dataStreamInfo{},
	}

	if dataStream != "" {
		ds := pkg.GetDataStream(dataStream)
		if ds == nil {
			return fmt.Errorf("data stream '%s' not found", dataStream)
		}

		dsInfo := dataStreamInfo{
			Name:       ds.Name,
			FieldCount: len(ds.Fields),
		}

		if showFields {
			for _, f := range ds.Fields {
				dsInfo.Fields = append(dsInfo.Fields, fieldInfo{
					Name:        f.Name,
					Type:        f.Type,
					Description: f.Description,
				})
			}
		}

		info.DataStreams = []dataStreamInfo{dsInfo}
	} else {
		for _, ds := range pkg.DataStreams {
			dsInfo := dataStreamInfo{
				Name:       ds.Name,
				FieldCount: len(ds.Fields),
			}

			if showFields {
				for _, f := range ds.Fields {
					dsInfo.Fields = append(dsInfo.Fields, fieldInfo{
						Name:        f.Name,
						Type:        f.Type,
						Description: f.Description,
					})
				}
			}

			info.DataStreams = append(info.DataStreams, dsInfo)
		}
	}

	output, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
