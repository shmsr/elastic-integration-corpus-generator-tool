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

func TestParsePackage(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create a mock package structure
	packagePath := "/test/packages/testpkg"

	// Create manifest.yml
	manifestContent := `name: testpkg
title: Test Package
version: 1.0.0
type: integration
owner:
  github: elastic/test-team
  type: elastic
`
	require.NoError(t, afero.WriteFile(fs, packagePath+"/manifest.yml", []byte(manifestContent), 0644))

	// Create data_stream directory
	require.NoError(t, fs.MkdirAll(packagePath+"/data_stream/metrics/fields", 0755))

	// Create fields.yml
	fieldsContent := `- name: testpkg
  type: group
  fields:
    - name: cpu
      type: double
      description: CPU usage percentage
    - name: memory
      type: long
      description: Memory usage in bytes
    - name: status
      type: keyword
      description: Status of the service
`
	require.NoError(t, afero.WriteFile(fs, packagePath+"/data_stream/metrics/fields/fields.yml", []byte(fieldsContent), 0644))

	// Parse the package
	pkg, err := ParsePackage(fs, packagePath)
	require.NoError(t, err)

	// Verify manifest
	assert.Equal(t, "testpkg", pkg.Manifest.Name)
	assert.Equal(t, "Test Package", pkg.Manifest.Title)
	assert.Equal(t, "1.0.0", pkg.Manifest.Version)
	assert.Equal(t, "elastic/test-team", pkg.Manifest.Owner.Github)

	// Verify data streams
	assert.Len(t, pkg.DataStreams, 1)
	assert.Equal(t, "metrics", pkg.DataStreams[0].Name)

	// Verify fields
	ds := pkg.GetDataStream("metrics")
	require.NotNil(t, ds)
	assert.Len(t, ds.Fields, 3)

	// Check flattened field names
	fieldNames := make(map[string]bool)
	for _, f := range ds.Fields {
		fieldNames[f.Name] = true
	}
	assert.True(t, fieldNames["testpkg.cpu"])
	assert.True(t, fieldNames["testpkg.memory"])
	assert.True(t, fieldNames["testpkg.status"])
}

func TestParsePackageNoDataStreams(t *testing.T) {
	fs := afero.NewMemMapFs()

	packagePath := "/test/packages/emptypkg"

	manifestContent := `name: emptypkg
title: Empty Package
version: 1.0.0
`
	require.NoError(t, afero.WriteFile(fs, packagePath+"/manifest.yml", []byte(manifestContent), 0644))

	pkg, err := ParsePackage(fs, packagePath)
	require.NoError(t, err)

	assert.Equal(t, "emptypkg", pkg.Manifest.Name)
	assert.Empty(t, pkg.DataStreams)
}

func TestListDataStreams(t *testing.T) {
	pkg := &Package{
		DataStreams: []DataStream{
			{Name: "metrics"},
			{Name: "logs"},
			{Name: "traces"},
		},
	}

	names := pkg.ListDataStreams()
	assert.Len(t, names, 3)
	assert.Contains(t, names, "metrics")
	assert.Contains(t, names, "logs")
	assert.Contains(t, names, "traces")
}

func TestGetDataStream(t *testing.T) {
	pkg := &Package{
		DataStreams: []DataStream{
			{Name: "metrics", Fields: []PackageField{{Name: "test"}}},
			{Name: "logs"},
		},
	}

	// Found
	ds := pkg.GetDataStream("metrics")
	require.NotNil(t, ds)
	assert.Equal(t, "metrics", ds.Name)
	assert.Len(t, ds.Fields, 1)

	// Not found
	ds = pkg.GetDataStream("nonexistent")
	assert.Nil(t, ds)
}

func TestFlattenFields(t *testing.T) {
	fields := []PackageField{
		{
			Name: "outer",
			Type: "group",
			Fields: []PackageField{
				{Name: "inner1", Type: "keyword"},
				{Name: "inner2", Type: "long"},
				{
					Name: "nested",
					Type: "group",
					Fields: []PackageField{
						{Name: "deep", Type: "boolean"},
					},
				},
			},
		},
	}

	flat := flattenFields(fields, "")

	assert.Len(t, flat, 3)

	fieldNames := make(map[string]string)
	for _, f := range flat {
		fieldNames[f.Name] = f.Type
	}

	assert.Equal(t, "keyword", fieldNames["outer.inner1"])
	assert.Equal(t, "long", fieldNames["outer.inner2"])
	assert.Equal(t, "boolean", fieldNames["outer.nested.deep"])
}
