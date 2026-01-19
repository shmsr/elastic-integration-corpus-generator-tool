// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package integrations

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ECSValidator validates fields against the Elastic Common Schema
type ECSValidator struct {
	ecsFields  map[string]ECSField
	loadOnce   sync.Once
	loadErr    error
	ecsVersion string
	useOnline  bool
	httpClient *http.Client
}

// ECSField represents an ECS field definition
type ECSField struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description" yaml:"description"`
	Level       string `json:"level" yaml:"level"` // core, extended, custom
	FlatName    string `json:"flat_name" yaml:"flat_name"`
}

// ECSValidationResult represents the result of ECS validation
type ECSValidationResult struct {
	DataStreamName string
	TotalFields    int
	ECSFields      int
	CustomFields   int
	InvalidFields  []ECSFieldIssue
	IsValid        bool
}

// ECSFieldIssue represents an issue with a field
type ECSFieldIssue struct {
	FieldName    string
	IssueType    string // "type_mismatch", "unknown_ecs_field", "naming_convention"
	ExpectedType string
	ActualType   string
	Message      string
}

// NewECSValidator creates a new ECS validator
func NewECSValidator(version string) *ECSValidator {
	return &ECSValidator{
		ecsVersion: version,
		ecsFields:  make(map[string]ECSField),
		useOnline:  false,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewECSValidatorOnline creates a validator that fetches ECS schema from GitHub
func NewECSValidatorOnline(version string) *ECSValidator {
	return &ECSValidator{
		ecsVersion: version,
		ecsFields:  make(map[string]ECSField),
		useOnline:  true,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// loadECSFields loads ECS field definitions
func (v *ECSValidator) loadECSFields() error {
	v.loadOnce.Do(func() {
		if v.useOnline {
			v.loadErr = v.fetchECSFromGitHub()
		}
		// Always load embedded fields as fallback/base
		if v.loadErr != nil || !v.useOnline {
			v.loadEmbeddedECSFields()
			v.loadErr = nil
		}
	})
	return v.loadErr
}

// fetchECSFromGitHub fetches the ECS schema from GitHub
func (v *ECSValidator) fetchECSFromGitHub() error {
	// Use the ecs_flat.yml which has a simpler structure
	url := fmt.Sprintf("https://raw.githubusercontent.com/elastic/ecs/%s/generated/ecs/ecs_flat.yml", v.ecsVersion)

	resp, err := v.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch ECS schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try with 'main' branch as fallback
		url = "https://raw.githubusercontent.com/elastic/ecs/main/generated/ecs/ecs_flat.yml"
		resp, err = v.httpClient.Get(url)
		if err != nil {
			return fmt.Errorf("failed to fetch ECS schema: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to fetch ECS schema: status %d", resp.StatusCode)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read ECS schema: %w", err)
	}

	// Parse the YAML - ecs_flat.yml is a map of field name to field definition
	var flatFields map[string]struct {
		Type      string `yaml:"type"`
		Level     string `yaml:"level"`
		FlatName  string `yaml:"flat_name"`
		ShortDesc string `yaml:"short"`
	}

	if err := yaml.Unmarshal(body, &flatFields); err != nil {
		return fmt.Errorf("failed to parse ECS schema: %w", err)
	}

	for name, f := range flatFields {
		v.ecsFields[name] = ECSField{
			Name:        name,
			Type:        f.Type,
			Level:       f.Level,
			FlatName:    f.FlatName,
			Description: f.ShortDesc,
		}
	}

	return nil
}

// loadEmbeddedECSFields loads commonly used ECS fields
func (v *ECSValidator) loadEmbeddedECSFields() {
	// Core ECS fields - these are the most commonly used
	coreFields := []ECSField{
		// Base fields
		{Name: "@timestamp", Type: "date", Level: "core"},
		{Name: "message", Type: "match_only_text", Level: "core"},
		{Name: "tags", Type: "keyword", Level: "core"},
		{Name: "labels", Type: "object", Level: "core"},

		// Agent fields
		{Name: "agent.build.original", Type: "keyword", Level: "core"},
		{Name: "agent.ephemeral_id", Type: "keyword", Level: "extended"},
		{Name: "agent.id", Type: "keyword", Level: "core"},
		{Name: "agent.name", Type: "keyword", Level: "core"},
		{Name: "agent.type", Type: "keyword", Level: "core"},
		{Name: "agent.version", Type: "keyword", Level: "core"},

		// Cloud fields
		{Name: "cloud.account.id", Type: "keyword", Level: "extended"},
		{Name: "cloud.account.name", Type: "keyword", Level: "extended"},
		{Name: "cloud.availability_zone", Type: "keyword", Level: "extended"},
		{Name: "cloud.instance.id", Type: "keyword", Level: "extended"},
		{Name: "cloud.instance.name", Type: "keyword", Level: "extended"},
		{Name: "cloud.machine.type", Type: "keyword", Level: "extended"},
		{Name: "cloud.project.id", Type: "keyword", Level: "extended"},
		{Name: "cloud.project.name", Type: "keyword", Level: "extended"},
		{Name: "cloud.provider", Type: "keyword", Level: "extended"},
		{Name: "cloud.region", Type: "keyword", Level: "extended"},
		{Name: "cloud.service.name", Type: "keyword", Level: "extended"},

		// Container fields
		{Name: "container.id", Type: "keyword", Level: "core"},
		{Name: "container.image.name", Type: "keyword", Level: "extended"},
		{Name: "container.image.tag", Type: "keyword", Level: "extended"},
		{Name: "container.name", Type: "keyword", Level: "extended"},
		{Name: "container.runtime", Type: "keyword", Level: "extended"},

		// Data stream fields
		{Name: "data_stream.dataset", Type: "constant_keyword", Level: "extended"},
		{Name: "data_stream.namespace", Type: "constant_keyword", Level: "extended"},
		{Name: "data_stream.type", Type: "constant_keyword", Level: "extended"},

		// ECS fields
		{Name: "ecs.version", Type: "keyword", Level: "core"},

		// Error fields
		{Name: "error.code", Type: "keyword", Level: "core"},
		{Name: "error.id", Type: "keyword", Level: "core"},
		{Name: "error.message", Type: "match_only_text", Level: "core"},
		{Name: "error.stack_trace", Type: "wildcard", Level: "extended"},
		{Name: "error.type", Type: "keyword", Level: "extended"},

		// Event fields
		{Name: "event.action", Type: "keyword", Level: "core"},
		{Name: "event.category", Type: "keyword", Level: "core"},
		{Name: "event.code", Type: "keyword", Level: "extended"},
		{Name: "event.created", Type: "date", Level: "core"},
		{Name: "event.dataset", Type: "keyword", Level: "core"},
		{Name: "event.duration", Type: "long", Level: "core"},
		{Name: "event.end", Type: "date", Level: "extended"},
		{Name: "event.hash", Type: "keyword", Level: "extended"},
		{Name: "event.id", Type: "keyword", Level: "core"},
		{Name: "event.ingested", Type: "date", Level: "core"},
		{Name: "event.kind", Type: "keyword", Level: "core"},
		{Name: "event.module", Type: "keyword", Level: "core"},
		{Name: "event.original", Type: "keyword", Level: "core"},
		{Name: "event.outcome", Type: "keyword", Level: "core"},
		{Name: "event.provider", Type: "keyword", Level: "extended"},
		{Name: "event.reason", Type: "keyword", Level: "extended"},
		{Name: "event.reference", Type: "keyword", Level: "extended"},
		{Name: "event.risk_score", Type: "float", Level: "core"},
		{Name: "event.risk_score_norm", Type: "float", Level: "extended"},
		{Name: "event.sequence", Type: "long", Level: "extended"},
		{Name: "event.severity", Type: "long", Level: "core"},
		{Name: "event.start", Type: "date", Level: "extended"},
		{Name: "event.timezone", Type: "keyword", Level: "extended"},
		{Name: "event.type", Type: "keyword", Level: "core"},
		{Name: "event.url", Type: "keyword", Level: "extended"},

		// Host fields
		{Name: "host.architecture", Type: "keyword", Level: "core"},
		{Name: "host.domain", Type: "keyword", Level: "extended"},
		{Name: "host.hostname", Type: "keyword", Level: "core"},
		{Name: "host.id", Type: "keyword", Level: "core"},
		{Name: "host.ip", Type: "ip", Level: "core"},
		{Name: "host.mac", Type: "keyword", Level: "core"},
		{Name: "host.name", Type: "keyword", Level: "core"},
		{Name: "host.os.family", Type: "keyword", Level: "extended"},
		{Name: "host.os.full", Type: "keyword", Level: "extended"},
		{Name: "host.os.kernel", Type: "keyword", Level: "extended"},
		{Name: "host.os.name", Type: "keyword", Level: "extended"},
		{Name: "host.os.platform", Type: "keyword", Level: "extended"},
		{Name: "host.os.type", Type: "keyword", Level: "extended"},
		{Name: "host.os.version", Type: "keyword", Level: "extended"},
		{Name: "host.type", Type: "keyword", Level: "core"},
		{Name: "host.uptime", Type: "long", Level: "extended"},

		// Service fields
		{Name: "service.address", Type: "keyword", Level: "extended"},
		{Name: "service.environment", Type: "keyword", Level: "extended"},
		{Name: "service.ephemeral_id", Type: "keyword", Level: "extended"},
		{Name: "service.id", Type: "keyword", Level: "core"},
		{Name: "service.name", Type: "keyword", Level: "core"},
		{Name: "service.node.name", Type: "keyword", Level: "extended"},
		{Name: "service.node.role", Type: "keyword", Level: "extended"},
		{Name: "service.state", Type: "keyword", Level: "extended"},
		{Name: "service.type", Type: "keyword", Level: "core"},
		{Name: "service.version", Type: "keyword", Level: "core"},

		// User fields
		{Name: "user.domain", Type: "keyword", Level: "extended"},
		{Name: "user.email", Type: "keyword", Level: "extended"},
		{Name: "user.full_name", Type: "keyword", Level: "extended"},
		{Name: "user.hash", Type: "keyword", Level: "extended"},
		{Name: "user.id", Type: "keyword", Level: "core"},
		{Name: "user.name", Type: "keyword", Level: "core"},
		{Name: "user.roles", Type: "keyword", Level: "extended"},

		// Network fields
		{Name: "network.application", Type: "keyword", Level: "extended"},
		{Name: "network.bytes", Type: "long", Level: "core"},
		{Name: "network.community_id", Type: "keyword", Level: "extended"},
		{Name: "network.direction", Type: "keyword", Level: "core"},
		{Name: "network.forwarded_ip", Type: "ip", Level: "core"},
		{Name: "network.iana_number", Type: "keyword", Level: "extended"},
		{Name: "network.name", Type: "keyword", Level: "extended"},
		{Name: "network.packets", Type: "long", Level: "core"},
		{Name: "network.protocol", Type: "keyword", Level: "core"},
		{Name: "network.transport", Type: "keyword", Level: "core"},
		{Name: "network.type", Type: "keyword", Level: "core"},

		// Source fields
		{Name: "source.address", Type: "keyword", Level: "extended"},
		{Name: "source.bytes", Type: "long", Level: "core"},
		{Name: "source.domain", Type: "keyword", Level: "core"},
		{Name: "source.ip", Type: "ip", Level: "core"},
		{Name: "source.mac", Type: "keyword", Level: "core"},
		{Name: "source.nat.ip", Type: "ip", Level: "extended"},
		{Name: "source.nat.port", Type: "long", Level: "extended"},
		{Name: "source.packets", Type: "long", Level: "core"},
		{Name: "source.port", Type: "long", Level: "core"},

		// Destination fields
		{Name: "destination.address", Type: "keyword", Level: "extended"},
		{Name: "destination.bytes", Type: "long", Level: "core"},
		{Name: "destination.domain", Type: "keyword", Level: "core"},
		{Name: "destination.ip", Type: "ip", Level: "core"},
		{Name: "destination.mac", Type: "keyword", Level: "core"},
		{Name: "destination.nat.ip", Type: "ip", Level: "extended"},
		{Name: "destination.nat.port", Type: "long", Level: "extended"},
		{Name: "destination.packets", Type: "long", Level: "core"},
		{Name: "destination.port", Type: "long", Level: "core"},

		// URL fields
		{Name: "url.domain", Type: "keyword", Level: "extended"},
		{Name: "url.extension", Type: "keyword", Level: "extended"},
		{Name: "url.fragment", Type: "keyword", Level: "extended"},
		{Name: "url.full", Type: "wildcard", Level: "extended"},
		{Name: "url.original", Type: "wildcard", Level: "extended"},
		{Name: "url.password", Type: "keyword", Level: "extended"},
		{Name: "url.path", Type: "wildcard", Level: "extended"},
		{Name: "url.port", Type: "long", Level: "extended"},
		{Name: "url.query", Type: "keyword", Level: "extended"},
		{Name: "url.registered_domain", Type: "keyword", Level: "extended"},
		{Name: "url.scheme", Type: "keyword", Level: "extended"},
		{Name: "url.subdomain", Type: "keyword", Level: "extended"},
		{Name: "url.top_level_domain", Type: "keyword", Level: "extended"},
		{Name: "url.username", Type: "keyword", Level: "extended"},

		// Process fields
		{Name: "process.args", Type: "keyword", Level: "extended"},
		{Name: "process.args_count", Type: "long", Level: "extended"},
		{Name: "process.command_line", Type: "wildcard", Level: "extended"},
		{Name: "process.entity_id", Type: "keyword", Level: "extended"},
		{Name: "process.executable", Type: "keyword", Level: "extended"},
		{Name: "process.exit_code", Type: "long", Level: "extended"},
		{Name: "process.name", Type: "keyword", Level: "extended"},
		{Name: "process.pgid", Type: "long", Level: "extended"},
		{Name: "process.pid", Type: "long", Level: "core"},
		{Name: "process.ppid", Type: "long", Level: "extended"},
		{Name: "process.start", Type: "date", Level: "extended"},
		{Name: "process.thread.id", Type: "long", Level: "extended"},
		{Name: "process.thread.name", Type: "keyword", Level: "extended"},
		{Name: "process.title", Type: "keyword", Level: "extended"},
		{Name: "process.uptime", Type: "long", Level: "extended"},
		{Name: "process.working_directory", Type: "keyword", Level: "extended"},

		// File fields
		{Name: "file.accessed", Type: "date", Level: "extended"},
		{Name: "file.created", Type: "date", Level: "extended"},
		{Name: "file.directory", Type: "keyword", Level: "extended"},
		{Name: "file.extension", Type: "keyword", Level: "extended"},
		{Name: "file.gid", Type: "keyword", Level: "extended"},
		{Name: "file.group", Type: "keyword", Level: "extended"},
		{Name: "file.inode", Type: "keyword", Level: "extended"},
		{Name: "file.mime_type", Type: "keyword", Level: "extended"},
		{Name: "file.mode", Type: "keyword", Level: "extended"},
		{Name: "file.mtime", Type: "date", Level: "extended"},
		{Name: "file.name", Type: "keyword", Level: "extended"},
		{Name: "file.owner", Type: "keyword", Level: "extended"},
		{Name: "file.path", Type: "keyword", Level: "extended"},
		{Name: "file.size", Type: "long", Level: "extended"},
		{Name: "file.type", Type: "keyword", Level: "extended"},
		{Name: "file.uid", Type: "keyword", Level: "extended"},

		// Geo fields
		{Name: "geo.city_name", Type: "keyword", Level: "core"},
		{Name: "geo.continent_code", Type: "keyword", Level: "core"},
		{Name: "geo.continent_name", Type: "keyword", Level: "core"},
		{Name: "geo.country_iso_code", Type: "keyword", Level: "core"},
		{Name: "geo.country_name", Type: "keyword", Level: "core"},
		{Name: "geo.location", Type: "geo_point", Level: "core"},
		{Name: "geo.name", Type: "keyword", Level: "extended"},
		{Name: "geo.postal_code", Type: "keyword", Level: "core"},
		{Name: "geo.region_iso_code", Type: "keyword", Level: "core"},
		{Name: "geo.region_name", Type: "keyword", Level: "core"},
		{Name: "geo.timezone", Type: "keyword", Level: "core"},

		// Orchestrator fields (Kubernetes/Docker)
		{Name: "orchestrator.api_version", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.cluster.id", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.cluster.name", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.cluster.url", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.cluster.version", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.namespace", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.organization", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.resource.id", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.resource.ip", Type: "ip", Level: "extended"},
		{Name: "orchestrator.resource.name", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.resource.parent.type", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.resource.type", Type: "keyword", Level: "extended"},
		{Name: "orchestrator.type", Type: "keyword", Level: "extended"},

		// Log fields
		{Name: "log.file.path", Type: "keyword", Level: "extended"},
		{Name: "log.level", Type: "keyword", Level: "core"},
		{Name: "log.logger", Type: "keyword", Level: "core"},
		{Name: "log.origin.file.line", Type: "long", Level: "extended"},
		{Name: "log.origin.file.name", Type: "keyword", Level: "extended"},
		{Name: "log.origin.function", Type: "keyword", Level: "extended"},
		{Name: "log.syslog.facility.code", Type: "long", Level: "extended"},
		{Name: "log.syslog.facility.name", Type: "keyword", Level: "extended"},
		{Name: "log.syslog.priority", Type: "long", Level: "extended"},
		{Name: "log.syslog.severity.code", Type: "long", Level: "extended"},
		{Name: "log.syslog.severity.name", Type: "keyword", Level: "extended"},

		// HTTP fields
		{Name: "http.request.body.bytes", Type: "long", Level: "extended"},
		{Name: "http.request.bytes", Type: "long", Level: "extended"},
		{Name: "http.request.method", Type: "keyword", Level: "extended"},
		{Name: "http.request.mime_type", Type: "keyword", Level: "extended"},
		{Name: "http.request.referrer", Type: "keyword", Level: "extended"},
		{Name: "http.response.body.bytes", Type: "long", Level: "extended"},
		{Name: "http.response.bytes", Type: "long", Level: "extended"},
		{Name: "http.response.mime_type", Type: "keyword", Level: "extended"},
		{Name: "http.response.status_code", Type: "long", Level: "extended"},
		{Name: "http.version", Type: "keyword", Level: "extended"},

		// Metricset fields (custom but common)
		{Name: "metricset.name", Type: "keyword", Level: "custom"},
		{Name: "metricset.period", Type: "long", Level: "custom"},
	}

	for _, f := range coreFields {
		v.ecsFields[f.Name] = f
	}
}

// Validate validates fields against ECS
func (v *ECSValidator) Validate(ds *DataStream) (*ECSValidationResult, error) {
	if err := v.loadECSFields(); err != nil {
		return nil, err
	}

	result := &ECSValidationResult{
		DataStreamName: ds.Name,
		TotalFields:    len(ds.Fields),
		IsValid:        true,
	}

	for _, f := range ds.Fields {
		// Skip fields with empty types - they're likely imported/external references
		if f.Type == "" {
			result.CustomFields++
			continue
		}

		// Check if it's an ECS field
		if ecsField, isECS := v.isECSField(f.Name); isECS {
			result.ECSFields++

			// Check type compatibility (skip if field type is empty)
			if f.Type != "" && !v.isTypeCompatible(f.Type, ecsField.Type) {
				result.InvalidFields = append(result.InvalidFields, ECSFieldIssue{
					FieldName:    f.Name,
					IssueType:    "type_mismatch",
					ExpectedType: ecsField.Type,
					ActualType:   f.Type,
					Message:      fmt.Sprintf("ECS field '%s' expects type '%s', got '%s'", f.Name, ecsField.Type, f.Type),
				})
				result.IsValid = false
			}
		} else {
			result.CustomFields++

			// Check naming conventions for custom fields
			if issue := v.checkNamingConvention(f.Name); issue != nil {
				result.InvalidFields = append(result.InvalidFields, *issue)
				// Naming convention issues are warnings, not failures
			}
		}
	}

	// Sort issues for consistent output
	sort.Slice(result.InvalidFields, func(i, j int) bool {
		return result.InvalidFields[i].FieldName < result.InvalidFields[j].FieldName
	})

	return result, nil
}

// isECSField checks if a field name matches an ECS field
func (v *ECSValidator) isECSField(name string) (ECSField, bool) {
	if field, ok := v.ecsFields[name]; ok {
		return field, true
	}

	// Check for prefix matches (e.g., source.geo.* matches source.geo.city_name)
	parts := strings.Split(name, ".")
	for i := len(parts) - 1; i > 0; i-- {
		prefix := strings.Join(parts[:i], ".")
		if field, ok := v.ecsFields[prefix]; ok {
			// Check if it's an object type that could contain nested fields
			if field.Type == "object" || field.Type == "nested" || field.Type == "group" {
				return field, true
			}
		}
	}

	return ECSField{}, false
}

// isTypeCompatible checks if two field types are compatible
func (v *ECSValidator) isTypeCompatible(actual, expected string) bool {
	if actual == expected {
		return true
	}

	// Normalize types
	actual = strings.ToLower(strings.TrimSpace(actual))
	expected = strings.ToLower(strings.TrimSpace(expected))

	if actual == expected {
		return true
	}

	// Type compatibility mappings
	compatible := map[string][]string{
		"keyword":         {"constant_keyword", "text", "match_only_text", "wildcard"},
		"text":            {"keyword", "match_only_text", "wildcard"},
		"match_only_text": {"keyword", "text", "wildcard"},
		"long":            {"integer", "short", "byte", "unsigned_long"},
		"integer":         {"long", "short", "byte"},
		"double":          {"float", "half_float", "scaled_float"},
		"float":           {"double", "half_float", "scaled_float"},
		"date":            {"date_nanos"},
		"wildcard":        {"keyword", "text"},
	}

	if compatTypes, ok := compatible[expected]; ok {
		for _, t := range compatTypes {
			if actual == t {
				return true
			}
		}
	}

	return false
}

// checkNamingConvention checks if custom field follows naming conventions
func (v *ECSValidator) checkNamingConvention(name string) *ECSFieldIssue {
	// Custom fields should follow snake_case and be namespaced
	parts := strings.Split(name, ".")

	// Should have at least a namespace
	if len(parts) < 2 {
		return &ECSFieldIssue{
			FieldName: name,
			IssueType: "naming_convention",
			Message:   fmt.Sprintf("Field '%s' should be namespaced (e.g., 'custom.%s')", name, name),
		}
	}

	// Skip naming convention checks for well-established integration namespaces
	// These often use product-specific naming that doesn't follow snake_case
	knownNamespaces := []string{
		"aws", "azure", "gcp", "google", "elasticsearch", "kibana", "logstash",
		"kafka", "mongodb", "mysql", "postgresql", "redis", "nginx", "apache",
		"kubernetes", "docker", "prometheus", "graphite", "statsd", "jolokia",
		"haproxy", "memcached", "rabbitmq", "activemq", "nats", "stan",
		"oracle", "mssql", "couchbase", "cassandra", "couchdb", "influxdb",
		"vsphere", "vmware", "citrix", "cisco", "juniper", "f5",
		"openai", "salesforce", "okta", "crowdstrike", "zscaler",
	}

	namespace := strings.ToLower(parts[0])
	for _, ns := range knownNamespaces {
		if namespace == ns {
			// Skip naming checks for known integration namespaces
			return nil
		}
	}

	// Check for camelCase (should be snake_case) - only for unknown namespaces
	for _, part := range parts {
		if strings.ToLower(part) != part {
			// Allow common abbreviations
			if !isCommonAbbreviation(part) {
				return &ECSFieldIssue{
					FieldName: name,
					IssueType: "naming_convention",
					Message:   fmt.Sprintf("Field '%s' should use snake_case (found: '%s')", name, part),
				}
			}
		}
	}

	return nil
}

// isCommonAbbreviation checks if a string is a common abbreviation
func isCommonAbbreviation(s string) bool {
	abbreviations := []string{
		// Cloud providers
		"AWS", "GCP", "EC2", "ECS", "EKS", "RDS", "S3", "VPC", "IAM", "ALB", "NLB", "ELB",
		"SQS", "SNS", "SES", "KMS", "ACM", "WAF", "EMR", "ASG", "AMI", "EBS", "EFS",
		// Azure
		"ARM", "AKS", "ACR", "AAD", "NSG", "NIC", "VHD",
		// GCP
		"GKE", "GCS", "GCE", "GAE", "GCF",
		// General tech
		"CPU", "RAM", "IO", "ID", "UUID", "UID", "GID", "PID", "TTL", "MTU",
		"URL", "URI", "HTTP", "HTTPS", "TCP", "UDP", "IP", "DNS", "TLS", "SSL", "SSH", "FTP",
		"API", "REST", "RPC", "GRPC", "JSON", "XML", "YAML", "CSV", "HTML", "CSS", "JS",
		// Storage
		"GB", "MB", "KB", "TB", "PB", "EB", "IOPS", "SSD", "HDD", "NFS", "CIFS", "SMB",
		// Time
		"MS", "NS", "US", "UTC", "GMT",
		// OS/System
		"OS", "VM", "DB", "SQL", "CLI", "GUI", "UI", "UX",
		// Protocols/Standards
		"LDAP", "SAML", "OIDC", "OAuth", "JWT", "MFA", "SSO", "RBAC",
		// Metrics
		"QPS", "TPS", "RPS", "P50", "P90", "P95", "P99", "AVG", "MAX", "MIN", "SUM",
		// Database
		"DDL", "DML", "DCL", "CRUD", "ACID",
		// Kubernetes
		"K8s", "POD", "PVC", "PV", "CRD",
	}
	for _, abbr := range abbreviations {
		if strings.EqualFold(s, abbr) {
			return true
		}
	}
	return false
}

// FormatResult formats the validation result as a string
func (r *ECSValidationResult) FormatResult() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🔍 ECS Validation Report: %s\n", r.DataStreamName))
	sb.WriteString(strings.Repeat("=", 50) + "\n\n")

	sb.WriteString(fmt.Sprintf("Total Fields: %d\n", r.TotalFields))
	sb.WriteString(fmt.Sprintf("  - ECS Fields: %d\n", r.ECSFields))
	sb.WriteString(fmt.Sprintf("  - Custom Fields: %d\n", r.CustomFields))
	sb.WriteString("\n")

	if r.IsValid && len(r.InvalidFields) == 0 {
		sb.WriteString("✅ All fields pass ECS validation!\n")
	} else if r.IsValid && len(r.InvalidFields) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️  Found %d warning(s):\n\n", len(r.InvalidFields)))
		for _, issue := range r.InvalidFields {
			sb.WriteString(fmt.Sprintf("  • %s\n", issue.Message))
		}
	} else {
		sb.WriteString(fmt.Sprintf("❌ Found %d issue(s):\n\n", len(r.InvalidFields)))
		for _, issue := range r.InvalidFields {
			sb.WriteString(fmt.Sprintf("  • %s\n", issue.Message))
		}
	}

	return sb.String()
}

// FormatJSON returns the result as JSON
func (r *ECSValidationResult) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
