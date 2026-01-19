// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package integrations provides functionality to work with elastic/integrations packages
package integrations

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

// Package represents an Elastic integration package
type Package struct {
	Path        string
	Manifest    Manifest
	DataStreams []DataStream
}

// Manifest represents the package manifest.yml
type Manifest struct {
	Name    string `yaml:"name"`
	Title   string `yaml:"title"`
	Version string `yaml:"version"`
	Type    string `yaml:"type"`
	Owner   struct {
		Github string `yaml:"github"`
		Type   string `yaml:"type"`
	} `yaml:"owner"`
}

// DataStream represents a data_stream within a package
type DataStream struct {
	Name   string
	Path   string
	Fields []PackageField
}

// PackageField represents a field definition from the package
type PackageField struct {
	Name        string         `yaml:"name"`
	Type        string         `yaml:"type"`
	Description string         `yaml:"description,omitempty"`
	MetricType  string         `yaml:"metric_type,omitempty"`
	Dimension   bool           `yaml:"dimension,omitempty"`
	Example     interface{}    `yaml:"example,omitempty"`
	ObjectType  string         `yaml:"object_type,omitempty"`
	Fields      []PackageField `yaml:"fields,omitempty"`
}

// ParsePackage parses an integration package from the given path
func ParsePackage(fs afero.Fs, packagePath string) (*Package, error) {
	pkg := &Package{
		Path: packagePath,
	}

	// Read manifest.yml
	manifestPath := filepath.Join(packagePath, "manifest.yml")
	manifestData, err := afero.ReadFile(fs, manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest.yml: %w", err)
	}

	if err := yaml.Unmarshal(manifestData, &pkg.Manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.yml: %w", err)
	}

	// Find all data streams
	dataStreamPath := filepath.Join(packagePath, "data_stream")
	entries, err := afero.ReadDir(fs, dataStreamPath)
	if err != nil {
		if os.IsNotExist(err) {
			return pkg, nil // No data streams is valid
		}
		return nil, fmt.Errorf("failed to read data_stream directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		ds, err := parseDataStream(fs, filepath.Join(dataStreamPath, entry.Name()), entry.Name())
		if err != nil {
			// Log but continue - some data streams may not have fields
			continue
		}
		pkg.DataStreams = append(pkg.DataStreams, *ds)
	}

	return pkg, nil
}

// parseDataStream parses a single data stream
func parseDataStream(fs afero.Fs, dsPath, name string) (*DataStream, error) {
	ds := &DataStream{
		Name: name,
		Path: dsPath,
	}

	// Read all fields.yml files in the fields directory
	fieldsDir := filepath.Join(dsPath, "fields")
	entries, err := afero.ReadDir(fs, fieldsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read fields directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		fieldsPath := filepath.Join(fieldsDir, entry.Name())
		data, err := afero.ReadFile(fs, fieldsPath)
		if err != nil {
			continue
		}

		var fields []PackageField
		if err := yaml.Unmarshal(data, &fields); err != nil {
			continue
		}

		// Flatten fields and add to data stream
		flatFields := flattenFields(fields, "")
		ds.Fields = append(ds.Fields, flatFields...)
	}

	return ds, nil
}

// flattenFields flattens nested field definitions into a flat list with dotted names
func flattenFields(fields []PackageField, prefix string) []PackageField {
	var result []PackageField

	for _, f := range fields {
		fieldName := f.Name
		if prefix != "" {
			fieldName = prefix + "." + f.Name
		}

		// If field has nested fields, recurse
		if len(f.Fields) > 0 {
			nested := flattenFields(f.Fields, fieldName)
			result = append(result, nested...)
		} else if f.Type != "group" {
			// Add leaf fields (ignore groups without subfields)
			result = append(result, PackageField{
				Name:        fieldName,
				Type:        f.Type,
				Description: f.Description,
				MetricType:  f.MetricType,
				Dimension:   f.Dimension,
				Example:     f.Example,
				ObjectType:  f.ObjectType,
			})
		}
	}

	return result
}

// GetDataStream returns a data stream by name, or nil if not found
func (p *Package) GetDataStream(name string) *DataStream {
	for i := range p.DataStreams {
		if p.DataStreams[i].Name == name {
			return &p.DataStreams[i]
		}
	}
	return nil
}

// ListDataStreams returns the names of all data streams
func (p *Package) ListDataStreams() []string {
	names := make([]string, len(p.DataStreams))
	for i, ds := range p.DataStreams {
		names[i] = ds.Name
	}
	return names
}
