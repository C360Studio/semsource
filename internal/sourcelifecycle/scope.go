package sourcelifecycle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/source/weburl"
	"github.com/c360studio/semsource/workspace"
	"github.com/c360studio/semstreams/types"
)

// ConfigDigest compares desired configurations independently of enabled state.
func ConfigDigest(c types.ComponentConfig) (string, error) {
	var config any
	decoder := json.NewDecoder(bytes.NewReader(c.Config))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return "", err
	}
	raw, err := json.Marshal(struct {
		Name   string
		Type   types.ComponentType
		Config any
	}{c.Name, c.Type, config})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type scopeConfig struct {
	Paths      []string `json:"paths"`
	Project    string   `json:"project"`
	Version    string   `json:"version"`
	Bucket     string   `json:"bucket"`
	URLs       []string `json:"urls"`
	RepoURL    string   `json:"repo_url"`
	RepoPath   string   `json:"repo_path"`
	BranchSlug string   `json:"branch_slug"`
	WatchPaths []struct {
		Project string `json:"project"`
		Version string `json:"version"`
	} `json:"watch_paths"`
}

// ScopeFor derives exact taxonomy prefixes from persisted producer identity inputs.
// Same-taxonomy registrations sharing a prefix must be rejected by CheckExclusive.
func ScopeFor(a entityid.Authority, handle string, cc types.ComponentConfig, inputs []sourceintent.StreamInput) (sourceintent.SourceScope, error) {
	scope := sourceintent.SourceScope{Authority: a, Handle: handle, Factory: cc.Name, Inputs: inputs}
	if err := a.Validate(); err != nil {
		return scope, err
	}
	digest, err := ConfigDigest(cc)
	if err != nil {
		return scope, err
	}
	scope.ConfigDigest = digest
	var cfg scopeConfig
	if err := json.Unmarshal(cc.Config, &cfg); err != nil {
		return scope, err
	}
	pairs, err := scopePairs(cc.Name, cfg)
	if err != nil {
		return scope, err
	}
	seen := map[string]bool{}
	for _, pair := range pairs {
		system, domain := pair[0], pair[1]
		if system == "" {
			return scope, errors.New("source scope has empty system")
		}
		prefix := a.Org + "." + a.Platform + "." + system + "." + domain + "."
		if !seen[prefix] {
			scope.Selectors = append(scope.Selectors, sourceintent.Selector{System: system, Domain: domain, Prefix: prefix})
			seen[prefix] = true
		}
	}
	if len(scope.Selectors) == 0 {
		return scope, &sourceintent.Blocker{Code: sourceintent.CodeUnsupportedScope, Message: "source has no provable selectors"}
	}
	sort.Slice(scope.Selectors, func(i, j int) bool { return scope.Selectors[i].Prefix < scope.Selectors[j].Prefix })
	return scope, nil
}
func scopePairs(factory string, c scopeConfig) ([][2]string, error) {
	var pairs [][2]string
	add := func(system string, domains ...string) {
		for _, domain := range domains {
			pairs = append(pairs, [2]string{system, domain})
		}
	}
	switch factory {
	case "ast-source":
		for _, path := range c.WatchPaths {
			add(entityid.ScopedSystemSlug(path.Project, path.Version), "code", "golang", "typescript", "javascript", "java", "python", "svelte", "c", "cpp")
		}
	case "doc-source", "cfgfile-source":
		roots := c.Paths
		if c.Project != "" {
			roots = []string{c.Project}
		}
		for _, root := range roots {
			system := entityid.SystemSlug(root)
			domain := "web"
			if factory == "cfgfile-source" {
				system = strings.ToLower(system)
				domain = "config"
			}
			add(system, domain)
		}
	case "url-source":
		for _, raw := range c.URLs {
			domain := weburl.ExtractDomain(raw)
			if domain == "" {
				return nil, errors.New("URL scope requires valid hostname")
			}
			add(entityid.SystemSlug(strings.ToLower(domain)), "web")
		}
	case "objectstore-source":
		project := c.Project
		if project == "" {
			project = c.Bucket
		}
		add(entityid.ScopedSystemSlug(project, c.Version), "web")
	case "git-source":
		system := workspace.URLToSlug(c.RepoURL)
		if c.RepoURL == "" {
			parts := strings.Split(strings.TrimRight(c.RepoPath, "/"), "/")
			system = strings.ReplaceAll(parts[len(parts)-1], ".", "-")
		}
		add(entityid.BranchScopedSlug(system, c.BranchSlug), "git")
	case "image-source", "audio-source", "video-source":
		for _, root := range c.Paths {
			add(entityid.SystemSlug(root), "media")
		}
	default:
		return nil, &sourceintent.Blocker{Code: sourceintent.CodeUnsupportedScope, Message: "unsupported source factory " + factory}
	}
	return pairs, nil
}

// IsSourceFactory identifies product producers; operational components are not sources.
func IsSourceFactory(factory string) bool {
	switch factory {
	case "ast-source", "doc-source", "cfgfile-source", "url-source", "objectstore-source", "git-source", "image-source", "audio-source", "video-source":
		return true
	}
	return false
}

// CheckExclusive refuses ambiguous ownership; retaining a sibling is safer than
// granting a removed registration authority over indistinguishable shared entities.
func CheckExclusive(scope sourceintent.SourceScope, components map[string]types.ComponentConfig) error {
	for handle, cc := range components {
		if handle == scope.Handle || !cc.Enabled || !IsSourceFactory(cc.Name) {
			continue
		}
		other, err := ScopeFor(scope.Authority, handle, cc, nil)
		if err != nil {
			return err
		}
		for _, left := range scope.Selectors {
			for _, right := range other.Selectors {
				if left.Prefix == right.Prefix {
					return &sourceintent.Blocker{Code: sourceintent.CodeAmbiguousScope, Message: fmt.Sprintf("source scope overlaps enabled sibling %s", handle)}
				}
			}
		}
	}
	return nil
}
