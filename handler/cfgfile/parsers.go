package cfgfile

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
)

// GoModResult holds parsed data from a go.mod file.
type GoModResult struct {
	// Module is the module path declared by the module directive.
	Module string
	// GoVersion is the minimum Go version declared by the go directive.
	GoVersion string
	// Deps is the list of required modules from require directives.
	Deps []GoModDep
}

// GoModDep is a single require entry from a go.mod file.
type GoModDep struct {
	// Path is the module path.
	Path string
	// Version is the module version.
	Version string
	// Indirect is true if the dependency is marked // indirect.
	Indirect bool
}

// ParseGoMod parses the content of a go.mod file.
// Returns an error if the content is empty or the module directive is missing.
func ParseGoMod(content []byte) (*GoModResult, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, fmt.Errorf("go.mod: empty content")
	}

	result := &GoModResult{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	inRequire := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "module ") {
			result.Module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			continue
		}
		if strings.HasPrefix(line, "go ") {
			result.GoVersion = strings.TrimSpace(strings.TrimPrefix(line, "go "))
			continue
		}
		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		// Single-line require: require module version
		if strings.HasPrefix(line, "require ") {
			rest := strings.TrimPrefix(line, "require ")
			dep := parseRequireLine(rest)
			if dep != nil {
				result.Deps = append(result.Deps, *dep)
			}
			continue
		}
		if inRequire && line != "" {
			dep := parseRequireLine(line)
			if dep != nil {
				result.Deps = append(result.Deps, *dep)
			}
		}
	}

	if result.Module == "" {
		return nil, fmt.Errorf("go.mod: missing module directive")
	}
	return result, nil
}

// parseRequireLine parses a single require line: "module/path v1.2.3 // indirect"
func parseRequireLine(line string) *GoModDep {
	// Strip inline comment
	if idx := strings.Index(line, "//"); idx >= 0 {
		indirect := strings.Contains(line[idx:], "indirect")
		line = strings.TrimSpace(line[:idx])
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil
		}
		return &GoModDep{Path: parts[0], Version: parts[1], Indirect: indirect}
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil
	}
	return &GoModDep{Path: parts[0], Version: parts[1]}
}

// PackageJSONResult holds parsed data from a package.json file.
type PackageJSONResult struct {
	// Name is the package name.
	Name string
	// Version is the package version.
	Version string
	// Deps is the combined list of dependencies and devDependencies.
	Deps []PackageJSONDep
}

// PackageJSONDep is a single dependency entry from a package.json file.
type PackageJSONDep struct {
	// Name is the npm package name.
	Name string
	// Version is the version specifier.
	Version string
	// Dev is true if the dependency is in devDependencies.
	Dev bool
}

// ParsePackageJSON parses the content of a package.json file.
func ParsePackageJSON(content []byte) (*PackageJSONResult, error) {
	var raw struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}

	result := &PackageJSONResult{
		Name:    raw.Name,
		Version: raw.Version,
	}

	for name, ver := range raw.Dependencies {
		result.Deps = append(result.Deps, PackageJSONDep{Name: name, Version: ver, Dev: false})
	}
	for name, ver := range raw.DevDependencies {
		result.Deps = append(result.Deps, PackageJSONDep{Name: name, Version: ver, Dev: true})
	}

	return result, nil
}

// DockerfileResult holds parsed data from a Dockerfile.
type DockerfileResult struct {
	// BaseImages is the list of base images from FROM directives (deduplicated).
	BaseImages []string
	// ExposedPorts is the list of ports declared by EXPOSE directives.
	ExposedPorts []string
}

// ParseDockerfile parses the content of a Dockerfile.
func ParseDockerfile(content []byte) (*DockerfileResult, error) {
	result := &DockerfileResult{}
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		upper := strings.ToUpper(line)

		if strings.HasPrefix(upper, "FROM ") {
			rest := strings.TrimSpace(line[5:])
			// Strip AS alias: "golang:1.21 AS builder" -> "golang:1.21"
			if idx := strings.Index(strings.ToUpper(rest), " AS "); idx >= 0 {
				rest = strings.TrimSpace(rest[:idx])
			}
			if rest != "" && !seen[rest] {
				seen[rest] = true
				result.BaseImages = append(result.BaseImages, rest)
			}
			continue
		}

		if strings.HasPrefix(upper, "EXPOSE ") {
			port := strings.TrimSpace(line[7:])
			if port != "" {
				result.ExposedPorts = append(result.ExposedPorts, port)
			}
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Maven POM parser
// ---------------------------------------------------------------------------

// POMResult holds parsed data from a Maven pom.xml file.
type POMResult struct {
	// GroupID is the Maven group identifier (e.g. "org.example").
	GroupID string
	// ArtifactID is the Maven artifact identifier (e.g. "my-app").
	ArtifactID string
	// Version is the declared project version.
	Version string
	// Packaging is the declared packaging type (e.g. "jar", "war", "pom").
	// Defaults to "jar" when absent per Maven convention.
	Packaging string
	// Deps is the list of declared dependencies.
	Deps []POMDep
	// Modules is the list of child module names for multi-module POMs.
	Modules []string
}

// POMDep is a single <dependency> entry from a pom.xml file.
type POMDep struct {
	// GroupID is the dependency group identifier.
	GroupID string
	// ArtifactID is the dependency artifact identifier.
	ArtifactID string
	// Version is the dependency version (may be empty when managed by a BOM).
	Version string
	// Scope is the Maven dependency scope: compile, test, provided, runtime, system.
	// Defaults to "compile" when absent.
	Scope string
}

// pomXML mirrors the pom.xml structure for xml.Unmarshal.
type pomXML struct {
	XMLName    xml.Name    `xml:"project"`
	GroupID    string      `xml:"groupId"`
	ArtifactID string      `xml:"artifactId"`
	Version    string      `xml:"version"`
	Packaging  string      `xml:"packaging"`
	Modules    pomModules  `xml:"modules"`
	Deps       pomDepsList `xml:"dependencies"`
}

type pomModules struct {
	Module []string `xml:"module"`
}

type pomDepsList struct {
	Dependency []pomDepXML `xml:"dependency"`
}

type pomDepXML struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

// ParsePOM parses the content of a Maven pom.xml file.
// Returns an error if content is empty or XML cannot be decoded.
func ParsePOM(content []byte) (*POMResult, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, fmt.Errorf("pom.xml: empty content")
	}

	var raw pomXML
	if err := xml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("pom.xml: %w", err)
	}

	result := &POMResult{
		GroupID:    raw.GroupID,
		ArtifactID: raw.ArtifactID,
		Version:    raw.Version,
		Packaging:  raw.Packaging,
		Modules:    raw.Modules.Module,
	}

	for _, d := range raw.Deps.Dependency {
		scope := d.Scope
		if scope == "" {
			scope = "compile"
		}
		result.Deps = append(result.Deps, POMDep{
			GroupID:    d.GroupID,
			ArtifactID: d.ArtifactID,
			Version:    d.Version,
			Scope:      scope,
		})
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Gradle build file parser (MVP regex-based)
// ---------------------------------------------------------------------------

// GradleResult holds parsed data from a build.gradle file.
type GradleResult struct {
	// Deps is the list of parsed dependency declarations.
	Deps []GradleDep
}

// GradleDep is a single dependency declaration from a build.gradle file.
type GradleDep struct {
	// Configuration is the Gradle configuration name
	// (e.g. "implementation", "testImplementation", "api").
	Configuration string
	// Group is the Maven group identifier portion of the dependency notation.
	Group string
	// Name is the Maven artifact identifier portion of the dependency notation.
	Name string
	// Version is the declared version string.
	Version string
}

// gradleDepRE matches lines of the form:
//
//	<configuration> '<group>:<name>:<version>'
//	<configuration> "<group>:<name>:<version>"
//
// Supported configurations: implementation, testImplementation, api,
// compileOnly, runtimeOnly.
var gradleDepRE = regexp.MustCompile(
	`^\s*(implementation|testImplementation|api|compileOnly|runtimeOnly)\s+['"]([^:'"]+):([^:'"]+):([^'"]+)['"]`,
)

// ParseGradle parses the content of a Gradle build file using regex matching.
// It never returns an error; an empty file yields an empty GradleResult.
func ParseGradle(content []byte) (*GradleResult, error) {
	result := &GradleResult{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		m := gradleDepRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		result.Deps = append(result.Deps, GradleDep{
			Configuration: m[1],
			Group:         m[2],
			Name:          m[3],
			Version:       strings.TrimSpace(m[4]),
		})
	}
	return result, nil
}
