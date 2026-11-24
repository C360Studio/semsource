package sourcespawn

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
)

// PlannedComponent is one component a deployment would spawn. Pin carries the
// one fact the component config does not: boot discovery made it as a
// submodule pin (config.SourceEntry.InstanceSuffix is never serialized).
type PlannedComponent struct {
	Instance string
	Config   types.ComponentConfig
	Pin      bool

	pending bool // an add_source spec, not yet registered
}

// systemKey is what two roots must share to mint the same entity IDs (#349):
// the component kind (ast and doc IDs differ in their domain segment), the
// org and the system segment.
type systemKey struct{ kind, org, system string }

type systemRoot struct {
	key          systemKey
	instance     string
	root         string
	pin, pending bool
}

// collisionConfig reads only the identity inputs of ast-source and doc-source
// configs, as cmd/semsource sourceScopeSystems does.
type collisionConfig struct {
	WatchPaths []struct {
		Path    string `json:"path"`
		Org     string `json:"org"`
		Project string `json:"project"`
		Version string `json:"version"`
	} `json:"watch_paths"`
	Org     string   `json:"org"`
	Paths   []string `json:"paths"`
	Project string   `json:"project"`
}

// systemRoots returns the roots of c with the system each mints IDs under,
// computed as the component does: ast-source ScopedSystemSlug(project,
// version) per watch path (processor/ast-source initializeWatchers), doc-source
// SystemSlug(project) when set, else SystemSlug(root) (handler/doc system).
// Other kinds are outside the rule and yield none.
func systemRoots(c PlannedComponent) ([]systemRoot, error) {
	if c.Config.Name != "ast-source" && c.Config.Name != "doc-source" {
		return nil, nil
	}
	var cfg collisionConfig
	if err := json.Unmarshal(c.Config.Config, &cfg); err != nil {
		return nil, fmt.Errorf("read identity of %s (%s): %w", c.Instance, c.Config.Name, err)
	}
	var roots []systemRoot
	add := func(org, system, root string) {
		roots = append(roots, systemRoot{
			key:      systemKey{kind: c.Config.Name, org: org, system: system},
			instance: c.Instance, root: root, pin: c.Pin, pending: c.pending,
		})
	}
	if c.Config.Name == "ast-source" {
		for _, wp := range cfg.WatchPaths {
			add(wp.Org, entityid.ScopedSystemSlug(wp.Project, wp.Version), wp.Path)
		}
		return roots, nil
	}
	for _, path := range cfg.Paths {
		system := entityid.SystemSlug(path)
		if cfg.Project != "" {
			system = entityid.SystemSlug(cfg.Project)
		}
		add(cfg.Org, system, path)
	}
	return roots, nil
}

// collision is one system that more than one root would mint IDs under.
type collision struct {
	key   systemKey
	roots []systemRoot
}

func (c collision) involvesPending() bool {
	return slices.ContainsFunc(c.roots, func(r systemRoot) bool { return r.pending })
}

func (c collision) String() string {
	members := make([]string, 0, len(c.roots))
	for _, r := range c.roots {
		members = append(members, fmt.Sprintf("%s (root %s)", r.instance, r.root))
	}
	return fmt.Sprintf("%s roots %s would share entity-ID system %q in org %q",
		strings.TrimSuffix(c.key.kind, "-source"), strings.Join(members, " and "), c.key.system, c.key.org)
}

// collisions groups every enabled root by its system key and returns each
// group with more than one root, except a group made only of submodule pins:
// discovery expands one submodule at one gitlink SHA into byte-identical
// sources, so their statement sets agree (D2).
func collisions(components []PlannedComponent) ([]collision, error) {
	roots, err := enabledRoots(components)
	if err != nil {
		return nil, err
	}
	return groupCollisions(roots), nil
}

// enabledRoots is every root of the enabled components, ordered by instance.
func enabledRoots(components []PlannedComponent) ([]systemRoot, error) {
	sorted := slices.Clone(components)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Instance < sorted[j].Instance })
	var out []systemRoot
	for _, c := range sorted {
		if !c.Config.Enabled {
			continue // a disabled envelope is removal intent; it does not spawn
		}
		roots, err := systemRoots(c)
		if err != nil {
			return nil, err
		}
		out = append(out, roots...)
	}
	return out, nil
}

func groupCollisions(roots []systemRoot) []collision {
	groups := map[systemKey][]systemRoot{}
	var order []systemKey
	for _, r := range roots {
		if _, seen := groups[r.key]; !seen {
			order = append(order, r.key)
		}
		groups[r.key] = append(groups[r.key], r)
	}
	var out []collision
	for _, key := range order {
		members := groups[key]
		if len(members) < 2 || !slices.ContainsFunc(members, func(r systemRoot) bool { return !r.pin }) {
			continue
		}
		out = append(out, collision{key: key, roots: members})
	}
	return out
}

func collisionError(found []collision) error {
	if len(found) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(found))
	for _, c := range found {
		msgs = append(msgs, c.String())
	}
	return &Error{
		Code: CodeValidationFailed,
		Message: strings.Join(msgs, "; ") +
			"; each would replace the other's entities, so give each source a distinct project (or version) (#349)",
	}
}

// CheckSystemCollisions refuses a component set in which two ast-source roots,
// or two doc-source roots, would mint entity IDs under one org and system,
// unless every root in the collision is a submodule pin. The error names each
// colliding instance and root and the shared system, with CodeValidationFailed.
func CheckSystemCollisions(components []PlannedComponent) error {
	found, err := collisions(components)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	return collisionError(found)
}

// Planned returns the components a component snapshot would spawn, marking
// as pins the instances in pins.
func Planned(components semconfig.ComponentConfigs, pins map[string]bool) []PlannedComponent {
	out := make([]PlannedComponent, 0, len(components))
	for name, cc := range components {
		out = append(out, PlannedComponent{Instance: name, Config: cc, Pin: pins[name]})
	}
	return out
}

// CheckSourceCollisions builds the ast and doc components of sources and runs
// CheckSystemCollisions over them, for a file's sources before boot. Two
// entries that build one instance over the same roots are one source; two that
// build one instance over different roots collide (the second would replace
// the first's component). Multi-branch repos are skipped: their branches are
// listed from a checkout, which is boot's job, and boot checks them. Nothing
// here clones or contacts a remote: single-branch repo expansion derives its
// paths from the URL, and only the git leaf, which is outside the rule,
// resolves a remote branch.
func CheckSourceCollisions(ctx context.Context, sources []config.SourceEntry, opts Options) error {
	var planned []PlannedComponent
	seen := map[string][]string{}
	for _, src := range sources {
		entries := []config.SourceEntry{src}
		if src.Type == "repo" {
			if len(src.Branches) > 0 {
				continue
			}
			expanded, err := config.ExpandRepoSources(ctx, entries, opts.WorkspaceDir)
			if err != nil {
				return &Error{Code: CodeValidationFailed, Message: "expand repo source", Cause: err}
			}
			entries = expanded.Sources
		}
		for _, entry := range entries {
			if entry.Type != "ast" && entry.Type != "docs" {
				continue
			}
			specs, err := buildSpecs(ctx, entry, opts)
			if err != nil {
				return err
			}
			for _, spec := range specs {
				pc, err := plannedFromSpec(spec, entry.InstanceSuffix != "")
				if err != nil {
					return err
				}
				roots, err := rootList(pc)
				if err != nil {
					return err
				}
				if prev, dup := seen[pc.Instance]; dup && slices.Equal(prev, roots) {
					continue
				}
				seen[pc.Instance] = roots
				planned = append(planned, pc)
			}
		}
	}
	return CheckSystemCollisions(planned)
}

// checkAddCollisions refuses an add whose specs would share a system with a
// registered source or with each other. A registered root the add rebuilds
// under the same instance and system is the same writer (a re-add or refresh)
// and leaves the comparison. A registered root the add's instance would drop
// stays: the add would move that instance onto another root of one system,
// which is a second writer, not an update. Collisions among registered sources
// alone predate the request and are boot's to refuse.
func checkAddCollisions(existing semconfig.ComponentConfigs, specs []componentSpec) error {
	pending := make([]PlannedComponent, 0, len(specs))
	for _, spec := range specs {
		pc, err := plannedFromSpec(spec, false)
		if err != nil {
			return err
		}
		pc.pending = true
		pending = append(pending, pc)
	}
	registered := make([]PlannedComponent, 0, len(existing))
	for name, cc := range existing {
		registered = append(registered, PlannedComponent{Instance: name, Config: cc})
	}
	pendingRoots, err := enabledRoots(pending)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	registeredRoots, err := enabledRoots(registered)
	if err != nil {
		return &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	same := map[string]bool{}
	for _, r := range pendingRoots {
		same[r.identity()] = true
	}
	roots := pendingRoots
	for _, r := range registeredRoots {
		if !same[r.identity()] {
			roots = append(roots, r)
		}
	}
	found := groupCollisions(roots)
	return collisionError(slices.DeleteFunc(found, func(c collision) bool { return !c.involvesPending() }))
}

// identity is what makes two roots one writer: one instance minting one
// system from one directory.
func (r systemRoot) identity() string {
	return r.instance + "\x00" + r.key.kind + "\x00" + r.key.org + "\x00" + r.key.system + "\x00" + filepath.Clean(r.root)
}

func plannedFromSpec(spec componentSpec, pin bool) (PlannedComponent, error) {
	raw, err := json.Marshal(spec.compCfg)
	if err != nil {
		return PlannedComponent{}, &Error{
			Code:    CodeValidationFailed,
			Message: fmt.Sprintf("marshal component config for %q", spec.instanceName),
			Cause:   err,
		}
	}
	return PlannedComponent{
		Instance: spec.instanceName,
		Config:   types.ComponentConfig{Name: spec.factoryName, Type: types.ComponentTypeProcessor, Enabled: true, Config: raw},
		Pin:      pin,
	}, nil
}

// rootList is a component's root identities in config order: what a second
// listing of one source must match to be the same source.
func rootList(c PlannedComponent) ([]string, error) {
	roots, err := systemRoots(c)
	if err != nil {
		return nil, &Error{Code: CodeValidationFailed, Message: "check entity-ID systems", Cause: err}
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		out = append(out, r.identity())
	}
	return out, nil
}
