// Package source provides vocabulary predicates for source entities.
// This file defines predicates for configuration file entities produced by the
// cfgfile handler. Entity types covered: module (go.mod, pom.xml sub-modules),
// package (package.json), dependency (all ecosystems), project (pom.xml,
// build.gradle), and image (Dockerfile).
package source

import "github.com/c360studio/semstreams/vocabulary"

// Config file predicates shared across all entity types.
const (
	// ConfigFilePath is the absolute path to the source config file on disk.
	// Present on module, package, project, and image entities.
	ConfigFilePath = "source.config.file_path"
)

// Go module predicates (go.mod).
const (
	// ConfigModulePath is the Go module path declared in go.mod.
	// Example: "github.com/c360studio/semsource"
	ConfigModulePath = "source.config.module.path"

	// ConfigModuleGoVer is the minimum Go toolchain version from the go directive.
	// Example: "1.22"
	ConfigModuleGoVer = "source.config.module.go_version"
)

// Dependency predicates shared across Go (go.mod), npm (package.json),
// Maven (pom.xml), and Gradle (build.gradle) ecosystems.
const (
	// ConfigDepName is the dependency module or package name.
	// For Go: module path. For npm: package name. For Maven/Gradle: "groupId:artifactId".
	ConfigDepName = "source.config.dependency.name"

	// ConfigDepVersion is the declared version string.
	// Format varies by ecosystem: semver, maven version, npm range specifier.
	ConfigDepVersion = "source.config.dependency.version"

	// ConfigDepIndirect indicates the dependency is indirect (go.mod only).
	// True when the // indirect comment is present on the require line.
	ConfigDepIndirect = "source.config.dependency.indirect"

	// ConfigDepKind identifies the ecosystem and production/dev classification.
	// Values: "go", "npm-prod", "npm-dev", "maven", "gradle"
	ConfigDepKind = "source.config.dependency.kind"

	// ConfigDepScope is the Maven dependency scope (pom.xml only).
	// Values: compile, test, provided, runtime, system, import
	ConfigDepScope = "source.config.dependency.scope"

	// ConfigDepConfiguration is the Gradle dependency configuration (build.gradle only).
	// Example: "implementation", "testImplementation", "api"
	ConfigDepConfiguration = "source.config.dependency.configuration"
)

// npm package predicates (package.json).
const (
	// ConfigPkgName is the npm package name from the "name" field.
	ConfigPkgName = "source.config.package.name"

	// ConfigPkgVersion is the package version from the "version" field.
	ConfigPkgVersion = "source.config.package.version"
)

// Maven/Gradle project predicates (pom.xml, build.gradle).
const (
	// ConfigProjectGroup is the Maven groupId or Gradle group identifier.
	// Example: "com.example", "org.springframework"
	ConfigProjectGroup = "source.config.project.group_id"

	// ConfigProjectArtifact is the Maven artifactId or Gradle project name.
	// Example: "my-service", "spring-boot"
	ConfigProjectArtifact = "source.config.project.artifact_id"

	// ConfigProjectVersion is the project version declared in pom.xml or inferred.
	ConfigProjectVersion = "source.config.project.version"

	// ConfigProjectPackaging is the Maven packaging type (pom.xml only).
	// Values: jar, war, pom, ear, etc.
	ConfigProjectPackaging = "source.config.project.packaging"

	// ConfigProjectBuild identifies the build system for project entities.
	// Values: "gradle", "maven"
	ConfigProjectBuild = "source.config.project.build"
)

// Dockerfile image predicates.
const (
	// ConfigImageName is the base image name from a FROM instruction.
	// Example: "golang:1.22-alpine", "node:20"
	ConfigImageName = "source.config.image.name"

	// ConfigImagePorts is the array of port numbers declared via EXPOSE instructions.
	// Stored as an array of strings; each element is a port or port/protocol pair.
	ConfigImagePorts = "source.config.image.exposed_ports"
)

// Relationship predicates for config entity edges.
const (
	// ConfigRequires is the edge type from a module or project to a dependency
	// that it requires. Used by go.mod module → dependency and pom.xml/Gradle
	// project → dependency edges.
	ConfigRequires = "source.config.requires"

	// ConfigDepends is the edge type from a package.json package to a dependency.
	// Mirrors the npm "dependencies" / "devDependencies" semantics.
	ConfigDepends = "source.config.depends"

	// ConfigContains is the edge type from a Maven parent project to a
	// sub-module declared in the <modules> section of pom.xml.
	ConfigContains = "source.config.contains"
)

// registerConfigPredicates registers all config entity predicates with the
// vocabulary. Called from the init() in predicates.go.
func registerConfigPredicates() {
	// Shared
	vocabulary.Register(ConfigFilePath,
		vocabulary.WithDescription("Absolute path to the source config file on disk"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"configFilePath"))

	// Go module
	vocabulary.Register(ConfigModulePath,
		vocabulary.WithDescription("Go module path declared in go.mod"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"modulePath"))

	vocabulary.Register(ConfigModuleGoVer,
		vocabulary.WithDescription("Minimum Go toolchain version from the go directive in go.mod"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"moduleGoVersion"))

	// Dependency
	vocabulary.Register(ConfigDepName,
		vocabulary.WithDescription("Dependency name: module path (Go), package name (npm), or groupId:artifactId (Maven/Gradle)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"depName"))

	vocabulary.Register(ConfigDepVersion,
		vocabulary.WithDescription("Declared version string; format varies by ecosystem"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"depVersion"))

	vocabulary.Register(ConfigDepIndirect,
		vocabulary.WithDescription("Whether the dependency is indirect (go.mod // indirect comment)"),
		vocabulary.WithDataType("bool"),
		vocabulary.WithIRI(Namespace+"depIndirect"))

	vocabulary.Register(ConfigDepKind,
		vocabulary.WithDescription("Ecosystem and classification: go, npm-prod, npm-dev, maven, gradle"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"depKind"))

	vocabulary.Register(ConfigDepScope,
		vocabulary.WithDescription("Maven dependency scope: compile, test, provided, runtime, system, import"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"depScope"))

	vocabulary.Register(ConfigDepConfiguration,
		vocabulary.WithDescription("Gradle dependency configuration: implementation, testImplementation, api, etc."),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"depConfiguration"))

	// npm package
	vocabulary.Register(ConfigPkgName,
		vocabulary.WithDescription("npm package name from the 'name' field in package.json"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"pkgName"))

	vocabulary.Register(ConfigPkgVersion,
		vocabulary.WithDescription("npm package version from the 'version' field in package.json"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"pkgVersion"))

	// Maven/Gradle project
	vocabulary.Register(ConfigProjectGroup,
		vocabulary.WithDescription("Maven groupId or Gradle group identifier"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"projectGroupID"))

	vocabulary.Register(ConfigProjectArtifact,
		vocabulary.WithDescription("Maven artifactId or Gradle project name"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"projectArtifactID"))

	vocabulary.Register(ConfigProjectVersion,
		vocabulary.WithDescription("Project version declared in pom.xml or inferred for Gradle"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"projectVersion"))

	vocabulary.Register(ConfigProjectPackaging,
		vocabulary.WithDescription("Maven packaging type: jar, war, pom, ear, etc."),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"projectPackaging"))

	vocabulary.Register(ConfigProjectBuild,
		vocabulary.WithDescription("Build system for the project entity: gradle or maven"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"projectBuild"))

	// Dockerfile image
	vocabulary.Register(ConfigImageName,
		vocabulary.WithDescription("Base image name from a Dockerfile FROM instruction"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"imageName"))

	vocabulary.Register(ConfigImagePorts,
		vocabulary.WithDescription("Array of port numbers or port/protocol pairs declared via Dockerfile EXPOSE"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"imageExposedPorts"))

	// Relationship predicates
	vocabulary.Register(ConfigRequires,
		vocabulary.WithDescription("Relationship: module or project requires a dependency (go.mod, pom.xml, build.gradle)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(Namespace+"configRequires"))

	vocabulary.Register(ConfigDepends,
		vocabulary.WithDescription("Relationship: npm package depends on a dependency (package.json)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(Namespace+"configDepends"))

	vocabulary.Register(ConfigContains,
		vocabulary.WithDescription("Relationship: Maven parent project contains a sub-module (pom.xml <modules>)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(Namespace+"configContains"))
}
