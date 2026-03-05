package cfgfile

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
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
