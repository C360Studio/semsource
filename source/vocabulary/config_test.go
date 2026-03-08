package source

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

func TestConfigPredicatesRegistered(t *testing.T) {
	predicates := []string{
		ConfigFilePath,
		ConfigModulePath,
		ConfigModuleGoVer,
		ConfigDepName,
		ConfigDepVersion,
		ConfigDepIndirect,
		ConfigDepKind,
		ConfigDepScope,
		ConfigDepConfiguration,
		ConfigPkgName,
		ConfigPkgVersion,
		ConfigProjectGroup,
		ConfigProjectArtifact,
		ConfigProjectVersion,
		ConfigProjectPackaging,
		ConfigProjectBuild,
		ConfigImageName,
		ConfigImagePorts,
		ConfigRequires,
		ConfigDepends,
		ConfigContains,
	}

	for _, pred := range predicates {
		t.Run(pred, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(pred)
			if meta == nil {
				t.Fatalf("predicate %s not registered", pred)
			}
			if meta.Description == "" {
				t.Errorf("predicate %s missing description", pred)
			}
		})
	}
}

func TestConfigPredicateDataTypes(t *testing.T) {
	tests := []struct {
		predicate    string
		expectedType string
	}{
		// Strings
		{ConfigFilePath, "string"},
		{ConfigModulePath, "string"},
		{ConfigModuleGoVer, "string"},
		{ConfigDepName, "string"},
		{ConfigDepVersion, "string"},
		{ConfigDepKind, "string"},
		{ConfigDepScope, "string"},
		{ConfigDepConfiguration, "string"},
		{ConfigPkgName, "string"},
		{ConfigPkgVersion, "string"},
		{ConfigProjectGroup, "string"},
		{ConfigProjectArtifact, "string"},
		{ConfigProjectVersion, "string"},
		{ConfigProjectPackaging, "string"},
		{ConfigProjectBuild, "string"},
		{ConfigImageName, "string"},
		// Bool
		{ConfigDepIndirect, "bool"},
		// Array
		{ConfigImagePorts, "array"},
		// Relationship predicates carry entity_id data type
		{ConfigRequires, "entity_id"},
		{ConfigDepends, "entity_id"},
		{ConfigContains, "entity_id"},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(tt.predicate)
			if meta == nil {
				t.Fatalf("predicate %s not registered", tt.predicate)
			}
			if meta.DataType != tt.expectedType {
				t.Errorf("predicate %s: expected type %s, got %s", tt.predicate, tt.expectedType, meta.DataType)
			}
		})
	}
}

func TestConfigPredicateIRIMappings(t *testing.T) {
	tests := []struct {
		predicate   string
		expectedIRI string
	}{
		{ConfigFilePath, Namespace + "configFilePath"},
		{ConfigModulePath, Namespace + "modulePath"},
		{ConfigModuleGoVer, Namespace + "moduleGoVersion"},
		{ConfigDepName, Namespace + "depName"},
		{ConfigDepVersion, Namespace + "depVersion"},
		{ConfigDepIndirect, Namespace + "depIndirect"},
		{ConfigDepKind, Namespace + "depKind"},
		{ConfigPkgName, Namespace + "pkgName"},
		{ConfigProjectGroup, Namespace + "projectGroupID"},
		{ConfigProjectArtifact, Namespace + "projectArtifactID"},
		{ConfigImageName, Namespace + "imageName"},
		{ConfigImagePorts, Namespace + "imageExposedPorts"},
		{ConfigRequires, Namespace + "configRequires"},
		{ConfigDepends, Namespace + "configDepends"},
		{ConfigContains, Namespace + "configContains"},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(tt.predicate)
			if meta == nil {
				t.Fatalf("predicate %s not registered", tt.predicate)
			}
			if meta.StandardIRI != tt.expectedIRI {
				t.Errorf("predicate %s: expected IRI %s, got %s", tt.predicate, tt.expectedIRI, meta.StandardIRI)
			}
		})
	}
}

func TestConfigPredicateConstantValues(t *testing.T) {
	// Verify the string values of the constants match the intended predicate
	// naming convention: source.{domain}.{entity_type}.{property}
	checks := []struct {
		name     string
		got      string
		expected string
	}{
		{"ConfigFilePath", ConfigFilePath, "source.config.file_path"},
		{"ConfigModulePath", ConfigModulePath, "source.config.module.path"},
		{"ConfigModuleGoVer", ConfigModuleGoVer, "source.config.module.go_version"},
		{"ConfigDepName", ConfigDepName, "source.config.dependency.name"},
		{"ConfigDepVersion", ConfigDepVersion, "source.config.dependency.version"},
		{"ConfigDepIndirect", ConfigDepIndirect, "source.config.dependency.indirect"},
		{"ConfigDepKind", ConfigDepKind, "source.config.dependency.kind"},
		{"ConfigDepScope", ConfigDepScope, "source.config.dependency.scope"},
		{"ConfigDepConfiguration", ConfigDepConfiguration, "source.config.dependency.configuration"},
		{"ConfigPkgName", ConfigPkgName, "source.config.package.name"},
		{"ConfigPkgVersion", ConfigPkgVersion, "source.config.package.version"},
		{"ConfigProjectGroup", ConfigProjectGroup, "source.config.project.group_id"},
		{"ConfigProjectArtifact", ConfigProjectArtifact, "source.config.project.artifact_id"},
		{"ConfigProjectVersion", ConfigProjectVersion, "source.config.project.version"},
		{"ConfigProjectPackaging", ConfigProjectPackaging, "source.config.project.packaging"},
		{"ConfigProjectBuild", ConfigProjectBuild, "source.config.project.build"},
		{"ConfigImageName", ConfigImageName, "source.config.image.name"},
		{"ConfigImagePorts", ConfigImagePorts, "source.config.image.exposed_ports"},
		{"ConfigRequires", ConfigRequires, "source.config.requires"},
		{"ConfigDepends", ConfigDepends, "source.config.depends"},
		{"ConfigContains", ConfigContains, "source.config.contains"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.expected {
				t.Errorf("%s = %q, want %q", c.name, c.got, c.expected)
			}
		})
	}
}
